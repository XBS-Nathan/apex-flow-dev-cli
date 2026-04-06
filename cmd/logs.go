package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/docker"
)

func init() { rootCmd.AddCommand(logsCmd) }

var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Stream container logs",
	Long: `Stream real-time logs from Docker containers.

Without arguments, shows logs from all containers.
Specify a service name to filter:

  dev logs          # all containers
  dev logs php82    # PHP 8.2 container
  dev logs mysql    # MySQL
  dev logs redis    # Redis
  dev logs caddy    # Caddy
  dev logs mailpit  # Mailpit`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := ""
		if len(args) > 0 {
			service = args[0]
		}
		return docker.Logs(service)
	},
}
