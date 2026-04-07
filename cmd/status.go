package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of services and linked projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println()

		services, err := composeServiceStatus()
		if err != nil {
			pterm.Warning.Println("Services are not running")
			return nil
		}

		// Services table
		pterm.DefaultSection.Println("Services")
		for _, svc := range services {
			indicator := pterm.Green("●")
			state := "running"
			if svc.status != "running" {
				indicator = pterm.Red("●")
				state = svc.status
			}
			fmt.Printf("  %s %s %s\n", indicator, pterm.LightCyan(svc.name), pterm.Gray(state))
		}

		// Linked projects
		fmt.Println()
		pterm.DefaultSection.Println("Projects")
		projects := linkedProjects()
		if len(projects) == 0 {
			fmt.Println("  No linked projects")
		} else {
			for _, p := range projects {
				fmt.Printf("  %s %s %s\n",
					pterm.Green("●"),
					pterm.White(p.name),
					pterm.Gray("https://"+p.name+".test"),
				)
			}
		}

		fmt.Println()
		return nil
	},
}

type serviceStatus struct {
	name   string
	status string
}

// composeServiceStatus runs docker compose ps to get service states.
func composeServiceStatus() ([]serviceStatus, error) {
	composeFile := docker.ComposeFile()
	cmd := exec.Command("docker", "compose", "-f", composeFile,
		"ps", "--format", "{{.Service}}\t{{.State}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting service status: %w", err)
	}

	var services []serviceStatus
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		services = append(services, serviceStatus{
			name:   parts[0],
			status: parts[1],
		})
	}
	return services, nil
}

type linkedProject struct {
	name string
}

// linkedProjects reads .caddy files from the sites directory.
func linkedProjects() []linkedProject {
	sitesDir := filepath.Join(config.GlobalDir(), "caddy", "sites")
	entries, err := os.ReadDir(sitesDir)
	if err != nil {
		return nil
	}

	var projects []linkedProject
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".caddy") {
			projects = append(projects, linkedProject{
				name: strings.TrimSuffix(name, ".caddy"),
			})
		}
	}
	return projects
}
