package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/caddy"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/docker"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/lifecycle"
)

func init() {
	rootCmd.AddCommand(downCmd)
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all projects and shared services",
	RunE: func(cmd *cobra.Command, args []string) error {
		lc := &lifecycle.Lifecycle{
			Docker: docker.Service{},
			Caddy:  caddy.Service{},
		}
		return lc.Down()
	},
}
