package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/caddy"
	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/hosts"
	"github.com/XBS-Nathan/nova/internal/lifecycle"
	"github.com/XBS-Nathan/nova/internal/phpimage"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(startCmd) }

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}

		// Write runtime PHP ini overrides
		phpIni := config.MergeSettings(config.DefaultPhpIni, global.PhpIni, p.Config.PhpIni)
		if err := config.WritePhpIni(config.GlobalDir(), p.Config.PHP, phpIni); err != nil {
			return fmt.Errorf("writing php.ini overrides: %w", err)
		}

		// Write runtime MySQL cnf overrides
		mysqlCnf := config.MergeSettings(config.DefaultMysqlCnf, global.MysqlCnf, p.Config.MysqlCnf)
		for k, v := range config.ProtectedMysqlCnf {
			mysqlCnf[k] = v
		}
		if err := config.WriteMysqlCnf(config.GlobalDir(), mysqlCnf); err != nil {
			return fmt.Errorf("writing my.cnf overrides: %w", err)
		}

		imgCfg := phpimage.ImageConfig{
			PHPVersion: p.Config.PHP,
			Extensions: p.Config.Extensions,
		}
		built, err := phpimage.EnsureBuilt(imgCfg)
		if err != nil {
			return err
		}

		php := []docker.PHPVersion{
			{
				Version:    p.Config.PHP,
				Extensions: p.Config.Extensions,
				Ports:      p.Config.Ports,
			},
		}

		lc := newLifecycle(global, p.Config)
		return lc.Start(p, php, built)
	},
}

// nodeServiceForProject builds a ServiceDefinition for the project's
// Node container if node_command is configured. Returns nil if not needed.
func nodeServiceForProject(
	p *project.Project,
	global *config.GlobalConfig,
) *config.ServiceDefinition {
	if p.Config.NodeCommand == "" {
		return nil
	}

	rel, err := filepath.Rel(global.ProjectsDir, p.Dir)
	if err != nil {
		rel = p.Name
	}
	workdir := filepath.Join("/srv", rel)

	// Enable corepack for pnpm/yarn, then run the configured command.
	// node_modules is already on the host via volume mount.
	pm := p.Config.PackageManager
	var setupCmds []string
	switch pm {
	case "pnpm":
		setupCmds = append(setupCmds, "corepack enable pnpm")
	case "yarn":
		setupCmds = append(setupCmds, "corepack enable yarn")
	}

	parts := append([]string{"cd " + workdir}, setupCmds...)
	parts = append(parts, p.Config.NodeCommand)
	cmd := strings.Join(parts, " && ")

	return &config.ServiceDefinition{
		Image: fmt.Sprintf("node:%s-alpine", p.Config.Node),
		Command: fmt.Sprintf("sh -c '%s'", strings.ReplaceAll(cmd, "'", "'\\''")),
		Volumes: []string{
			fmt.Sprintf("%s:/srv", global.ProjectsDir),
		},
		Environment: map[string]string{
			"NODE_ENV": "development",
			"NOVA":     "true",
		},
	}
}

// workerServicesForProject builds ServiceDefinitions for each configured worker.
// Workers run in the project's PHP image with auto-restart.
func workerServicesForProject(
	p *project.Project,
	global *config.GlobalConfig,
) map[string]config.ServiceDefinition {
	if len(p.Config.Workers) == 0 {
		return nil
	}

	rel, err := filepath.Rel(global.ProjectsDir, p.Dir)
	if err != nil {
		rel = p.Name
	}
	workdir := filepath.Join("/srv", rel)

	image := phpimage.ImageTag(phpimage.ImageConfig{
		PHPVersion: p.Config.PHP,
		Extensions: p.Config.Extensions,
	})

	services := make(map[string]config.ServiceDefinition, len(p.Config.Workers))
	for name, command := range p.Config.Workers {
		svcName := fmt.Sprintf("%s-%s", name, p.Name)
		cmd := fmt.Sprintf("cd %s && %s", workdir, command)
		services[svcName] = config.ServiceDefinition{
			Image:   image,
			Command: fmt.Sprintf("sh -c '%s'", strings.ReplaceAll(cmd, "'", "'\\''")),
			Volumes: []string{
				fmt.Sprintf("%s:/srv", global.ProjectsDir),
			},
			Environment: map[string]string{
				"NOVA": "true",
			},
		}
	}
	return services
}

func newLifecycle(
	global *config.GlobalConfig,
	projectCfg *config.ProjectConfig,
) *lifecycle.Lifecycle {
	collected := config.CollectVersions(global.ProjectsDir, projectCfg)

	dbServiceName := dbServiceForProject(projectCfg, global)

	return &lifecycle.Lifecycle{
		Docker: docker.Service{
			ProjectsDir:    global.ProjectsDir,
			Collected:      collected,
			MailpitVersion: global.Versions.Mailpit,
		},
		Caddy:         caddy.Service{},
		Hosts:         hosts.Service{},
		PHPService:    docker.PHPServiceName,
		DBServiceName:   dbServiceName,
		ServiceVersions: global.Versions,
		Docroot: func(p *project.Project) string {
			rel, err := filepath.Rel(global.ProjectsDir, p.Dir)
			if err != nil {
				rel = p.Name
			}
			return filepath.Join("/srv", rel, "public")
		},
		NodeServiceBuilder: func(p *project.Project) *config.ServiceDefinition {
			return nodeServiceForProject(p, global)
		},
		WorkersBuilder: func(p *project.Project) map[string]config.ServiceDefinition {
			return workerServicesForProject(p, global)
		},
	}
}


