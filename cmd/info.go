package cmd

import (
	"fmt"
	"os"

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
				pterm.Gray("Node"), pterm.White(p.Config.Node+" ("+p.Config.PackageManager+")"),
			)

		fmt.Println()

		// Connection details as a compact table
		tableData := pterm.TableData{
			{"Service", "Host", "Port", "User", "Password"},
			{p.Config.DBDriver, dbService, dbPort, dbUser, dbPass},
			{"Redis", redisService, "6379", "-", "-"},
			{"Mailpit (SMTP)", "mailpit", "1025", "-", "-"},
			{"Mailpit (UI)", "http://localhost:8025", "8025", "-", "-"},
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
