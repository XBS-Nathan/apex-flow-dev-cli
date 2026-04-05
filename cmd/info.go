package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/caddy"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/docker"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show project information",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		linked := "no"
		if _, err := os.Stat(caddy.SiteConfigPath(p.Name)); err == nil {
			linked = "yes"
		}

		servicesUp := "no"
		if docker.IsUp() {
			servicesUp = "yes"
		}

		caddyUp := "no"
		if caddy.IsRunning() {
			caddyUp = "yes"
		}

		fmt.Printf("Project:    %s\n", p.Name)
		fmt.Printf("Type:       %s\n", p.Config.Type)
		fmt.Printf("Directory:  %s\n", p.Dir)
		fmt.Printf("URL:        http://%s\n", p.SiteDomain())
		fmt.Printf("PHP:        %s\n", p.Config.PHP)
		fmt.Printf("Node:       %s\n", p.Config.Node)
		fmt.Printf("DB Driver:  %s\n", p.Config.DBDriver)
		fmt.Printf("Database:   %s\n", p.Config.DB)
		fmt.Printf("Linked:     %s\n", linked)
		fmt.Printf("Caddy:      %s\n", caddyUp)
		fmt.Printf("Services:   %s\n", servicesUp)

		return nil
	},
}
