package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/caddy"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/docker"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/lifecycle"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/php"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		lc := &lifecycle.Lifecycle{
			Docker: docker.Service{},
			Caddy:  caddy.Service{},
		}
		return lc.Start(p, php.FPMSocket(p.Config.PHP))
	},
}
