package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
)

func init() { rootCmd.AddCommand(downCmd) }

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all projects and shared services",
	RunE: func(cmd *cobra.Command, args []string) error {
		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		return newLifecycle(global, &config.ProjectConfig{}).Down()
	},
}
