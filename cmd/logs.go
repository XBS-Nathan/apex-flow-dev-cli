package cmd

import (
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(logsCmd) }

var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Stream container logs",
	Long: `Stream real-time logs from Docker containers.

Without arguments, shows logs from all shared containers.
Specify a service name to filter:

  nova logs          # all shared containers
  nova logs php82    # PHP 8.2 container
  nova logs mysql    # MySQL
  nova logs redis    # Redis
  nova logs caddy    # Caddy
  nova logs mailpit  # Mailpit
  nova logs node     # Node dev server (project service)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := ""
		if len(args) > 0 {
			service = args[0]
		}

		// Check if the service is a project service (e.g. node container)
		if service != "" {
			if p, err := project.Detect(); err == nil {
				projectServices := docker.ProjectServices(p.Name, p.Dir)
				for _, ps := range projectServices {
					if ps == service || ps == docker.NodeServiceName(p.Name) && service == "node" {
						return docker.ProjectLogs(p.Name, p.Dir, ps)
					}
				}
			}
		}

		return docker.Logs(service)
	},
}
