package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/caddy"
	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
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

		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}

		linked := pterm.Green("●")
		if _, err := os.Stat(caddy.SiteConfigPath(p.Name)); err != nil {
			linked = pterm.Red("●")
		}

		servicesUp := pterm.Green("●")
		if !docker.IsUp() {
			servicesUp = pterm.Red("●")
		}

		dbService := dbServiceForProject(p.Config, global)
		collected := config.CollectVersions(global.ProjectsDir, p.Config)
		redisService := docker.ServiceName("redis", p.Config.RedisVersion, len(collected.Redis))

		dbUser := p.Config.MySQL.User
		dbPass := p.Config.MySQL.Pass
		dbPort := p.Config.MySQL.Port
		if p.Config.DBDriver == "postgres" {
			dbUser = p.Config.Postgres.User
			dbPass = p.Config.Postgres.Pass
			dbPort = p.Config.Postgres.Port
		}

		fmt.Println()

		// Node info line
		nodeInfo := p.Config.Node + " (" + p.Config.PackageManager + ")"
		if p.Config.NodeCommand != "" {
			nodeInfo += "  " + pterm.Gray("cmd") + " " + pterm.White(p.Config.NodeCommand)
			if len(p.Config.Ports) > 0 {
				nodeInfo += "  " + pterm.Gray("port") + " " + pterm.White(strings.Join(p.Config.Ports, ", "))
			}
		}

		// Title box
		pterm.DefaultBox.
			WithTitle(pterm.LightCyan(p.Name)).
			WithTitleTopCenter().
			WithBoxStyle(pterm.NewStyle(pterm.FgGray)).
			Printfln(
				"%s  %s   %s  %s   %s  %s\n%s  %s   %s  %s",
				pterm.Gray("URL"), pterm.LightCyan("https://"+p.SiteDomain()),
				pterm.Gray("Linked"), linked,
				pterm.Gray("Services"), servicesUp,
				pterm.Gray("PHP"), pterm.White(p.Config.PHP),
				pterm.Gray("Node"), nodeInfo,
			)

		fmt.Println()

		// Connection details as a compact table
		caddyPorts := "80, 443"
		if len(p.Config.Ports) > 0 {
			caddyPorts += ", " + strings.Join(p.Config.Ports, ", ")
		}
		tableData := pterm.TableData{
			{"Service", "Host", "Port", "User", "Password"},
			{"Caddy", "caddy", caddyPorts, "-", "-"},
			{p.Config.DBDriver, dbService, dbPort, dbUser, dbPass},
			{"Redis", redisService, "6379", "-", "-"},
			{"Mailpit (SMTP)", "mailpit", "1025", "-", "-"},
			{"Mailpit (UI)", "http://localhost:8025", "8025", "-", "-"},
		}

		// Shared services (e.g. typesense, meilisearch)
		sharedNames := make([]string, 0, len(collected.SharedServices))
		for name := range collected.SharedServices {
			sharedNames = append(sharedNames, name)
		}
		sort.Strings(sharedNames)
		for _, name := range sharedNames {
			svc := collected.SharedServices[name]
			ports := strings.Join(svc.Ports, ", ")
			if ports == "" {
				ports = "-"
			}
			tableData = append(tableData, []string{name, name, ports, "-", "-"})
		}

		// Per-project services
		projectSvcNames := make([]string, 0, len(p.Config.Services))
		for name := range p.Config.Services {
			projectSvcNames = append(projectSvcNames, name)
		}
		sort.Strings(projectSvcNames)
		for _, name := range projectSvcNames {
			svc := p.Config.Services[name]
			ports := strings.Join(svc.Ports, ", ")
			if ports == "" {
				ports = "-"
			}
			tableData = append(tableData, []string{name, name, ports, "-", "-"})
		}

		pterm.DefaultTable.
			WithHasHeader().
			WithBoxed().
			WithData(tableData).
			Render()

		fmt.Println()
		return nil
	},
}
