package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/docker"
)

func init() { rootCmd.AddCommand(pullCmd) }

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Re-download the latest Docker images for shared services",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Pulling latest images...")
		if err := docker.Pull(); err != nil {
			return err
		}
		fmt.Println("✓ Images updated")
		return nil
	},
}
