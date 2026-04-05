package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/docker"
)

func init() {
	rootCmd.AddCommand(servicesCmd)
	servicesCmd.AddCommand(servicesUpCmd)
	servicesCmd.AddCommand(servicesDownCmd)
}

var servicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Manage shared Docker services",
}

var servicesUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start shared services (MySQL, Redis, Typesense, etc.)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting shared services...")
		if err := docker.Up(); err != nil {
			return err
		}
		fmt.Println("✓ Services running")
		return nil
	},
}

var servicesDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop shared services",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Stopping shared services...")
		if err := docker.Down(); err != nil {
			return err
		}
		fmt.Println("✓ Services stopped")
		return nil
	},
}
