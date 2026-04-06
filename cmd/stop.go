package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(stopCmd) }

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}
		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		return newLifecycle(global, p.Config).Stop(p)
	},
}
