package cmd

import (
	"fmt"
	"os"
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
			Runtime:    p.Config.Runtime,
		}
		built, err := phpimage.EnsureBuilt(imgCfg)
		if err != nil {
			return err
		}

		php, frankenphp, err := runtimePayload(p, global)
		if err != nil {
			return err
		}

		lc := newLifecycle(global, p.Config)
		return lc.Start(p, php, frankenphp, built)
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

// runtimePayload returns the (php, frankenphp) slices to pass to lifecycle.Start.
// It includes services for every linked project (any project that has a
// Caddy site file under <globalDir>/caddy/sites) plus the current project,
// so that `nova start` on one project does not tear down PHP services
// belonging to other already-running projects via --remove-orphans.
//
// Linked FPM projects sharing a PHP version share a single php<XX> service;
// the current project's extensions/ports win when there's a conflict.
// FrankenPHP projects each get their own per-project service.
func runtimePayload(
	p *project.Project,
	global *config.GlobalConfig,
) ([]docker.PHPVersion, []docker.FrankenPHPProject, error) {
	type entry struct {
		name string
		dir  string
		cfg  *config.ProjectConfig
	}

	// Current project is always included and always wins on conflicts.
	entries := []entry{{name: p.Name, dir: p.Dir, cfg: p.Config}}
	seen := map[string]bool{p.Name: true}

	sitesDir := filepath.Join(config.GlobalDir(), "caddy", "sites")
	if dirEntries, err := os.ReadDir(sitesDir); err == nil {
		for _, e := range dirEntries {
			name := strings.TrimSuffix(e.Name(), ".caddy")
			if name == e.Name() || seen[name] {
				continue
			}
			seen[name] = true
			otherDir := filepath.Join(global.ProjectsDir, name)
			otherCfg, err := config.Load(otherDir)
			if err != nil {
				continue
			}
			entries = append(entries, entry{name: name, dir: otherDir, cfg: otherCfg})
		}
	}

	var php []docker.PHPVersion
	var franken []docker.FrankenPHPProject
	fpmIdxByVersion := map[string]int{}

	for i, e := range entries {
		if e.cfg.Runtime == config.RuntimeFrankenPHP {
			rel, err := filepath.Rel(global.ProjectsDir, e.dir)
			if err != nil {
				return nil, nil, fmt.Errorf("resolving %s workdir: %w", e.name, err)
			}
			franken = append(franken, docker.FrankenPHPProject{
				Name:       e.name,
				PHPVersion: e.cfg.PHP,
				Extensions: e.cfg.Extensions,
				Octane:     e.cfg.Octane,
				Workdir:    filepath.Join("/srv", rel),
				Ports:      e.cfg.Ports,
			})
			continue
		}
		if idx, ok := fpmIdxByVersion[e.cfg.PHP]; ok {
			// Current project (i == 0) overrides any linked project's
			// extensions/ports for the same PHP version.
			if i == 0 {
				php[idx] = docker.PHPVersion{
					Version:    e.cfg.PHP,
					Extensions: e.cfg.Extensions,
					Ports:      e.cfg.Ports,
				}
			}
			continue
		}
		fpmIdxByVersion[e.cfg.PHP] = len(php)
		php = append(php, docker.PHPVersion{
			Version:    e.cfg.PHP,
			Extensions: e.cfg.Extensions,
			Ports:      e.cfg.Ports,
		})
	}

	return php, franken, nil
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
		Runtime:    p.Config.Runtime,
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


