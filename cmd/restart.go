package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/phpimage"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(restartCmd) }

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}
		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		imgCfg := phpimage.ImageConfig{
			PHPVersion: p.Config.PHP,
			Extensions: p.Config.Extensions,
		}
		built, err := phpimage.EnsureBuilt(imgCfg)
		if err != nil {
			return err
		}
		lc := newLifecycle(global, p.Config)
		if err := lc.Stop(p); err != nil {
			return err
		}
		php := []docker.PHPVersion{
			{
				Version:    p.Config.PHP,
				Extensions: p.Config.Extensions,
				Ports:      p.Config.Ports,
			},
		}
		return lc.Start(p, php, built)
	},
}
