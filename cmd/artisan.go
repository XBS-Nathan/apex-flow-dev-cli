package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(artisanCmd) }

var artisanCmd = &cobra.Command{
	Use:                "artisan [args...]",
	Short:              "Run php artisan with the project's PHP version",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}
		if p.Config.Type != config.TypeLaravel {
			return fmt.Errorf(
				"artisan is only available for Laravel projects (current type: %s)",
				p.Config.Type,
			)
		}
		return runInContainer(append([]string{"php", "artisan"}, args...)...)
	},
}

func containerWorkdir(projectsDir, projectDir string) (string, error) {
	rel, err := filepath.Rel(projectsDir, projectDir)
	if err != nil {
		return "", fmt.Errorf(
			"project dir %s is not under projects dir %s: %w",
			projectDir, projectsDir, err,
		)
	}
	return filepath.Join("/srv", rel), nil
}

// runInContainer detects the project, resolves the container workdir,
// and executes a command in the project's PHP container.
func runInContainer(args ...string) error {
	p, err := project.Detect()
	if err != nil {
		return err
	}
	global, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	workdir, err := containerWorkdir(global.ProjectsDir, p.Dir)
	if err != nil {
		return err
	}
	svc := docker.PHPServiceName(p.Config.PHP)
	return docker.Exec(svc, workdir, args...)
}
