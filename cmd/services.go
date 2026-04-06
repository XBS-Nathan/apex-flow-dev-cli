package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
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
	Short: "Start shared services (MySQL, Redis, etc.)",
	RunE: func(cmd *cobra.Command, args []string) error {
		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}

		// Use a default project config to collect versions from all projects
		defaultCfg := &config.ProjectConfig{
			DBDriver:     "mysql",
			DBVersion:    global.Versions.MySQL,
			RedisVersion: global.Versions.Redis,
		}
		collected := config.CollectVersions(global.ProjectsDir, defaultCfg)

		// Write runtime MySQL cnf overrides
		mysqlCnf := config.MergeSettings(config.DefaultMysqlCnf, global.MysqlCnf, nil)
		for k, v := range config.ProtectedMysqlCnf {
			mysqlCnf[k] = v
		}
		if err := config.WriteMysqlCnf(config.GlobalDir(), mysqlCnf); err != nil {
			return fmt.Errorf("writing my.cnf overrides: %w", err)
		}

		fmt.Println("Starting shared services...")
		opts := docker.ComposeOptions{
			ProjectsDir:      global.ProjectsDir,
			PHP:              []docker.PHPVersion{{Version: config.DefaultPHP}},
			MySQLVersions:    collected.MySQL,
			PostgresVersions: collected.Postgres,
			RedisVersions:    collected.Redis,
			MailpitVersion:   global.Versions.Mailpit,
		}
		if err := docker.Up(opts); err != nil {
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
