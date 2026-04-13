package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/XBS-Nathan/nova/internal/config"
)

func init() { rootCmd.AddCommand(initCmd) }

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a .nova/config.yaml for the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		path := filepath.Join(cwd, config.ConfigFile)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists — edit it directly or delete it first", config.ConfigFile)
		}

		// Create .nova/ directory
		novaDir := filepath.Join(cwd, ".nova")
		if err := os.MkdirAll(novaDir, 0755); err != nil {
			return fmt.Errorf("creating .nova directory: %w", err)
		}

		projectName := filepath.Base(cwd)

		// Auto-detect project type
		detectedType := "other"
		if _, err := os.Stat(filepath.Join(cwd, "artisan")); err == nil {
			detectedType = config.TypeLaravel
		}

		fmt.Println()
		pterm.DefaultSection.Printfln("nova init — %s", projectName)

		// --- General ---
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgDefault)).
			WithTextStyle(pterm.NewStyle(pterm.Bold)).Println("General")

		projectType, err := pterm.DefaultInteractiveSelect.
			WithOptions([]string{"laravel", "other"}).
			WithDefaultOption(detectedType).
			WithFilter(true).
			Show("Project type")
		if err != nil {
			return err
		}

		defaultDomain := projectName + ".test"
		domain, err := pterm.DefaultInteractiveTextInput.
			WithDefaultValue(defaultDomain).
			Show("Domain")
		if err != nil {
			return err
		}
		if !strings.HasSuffix(domain, ".test") {
			domain += ".test"
		}

		// --- Language ---
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgDefault)).
			WithTextStyle(pterm.NewStyle(pterm.Bold)).Println("Language")

		php, err := pterm.DefaultInteractiveSelect.
			WithOptions(config.PHPVersions).
			WithDefaultOption(config.DefaultPHP).
			WithFilter(true).
			Show("PHP version")
		if err != nil {
			return err
		}

		node, err := pterm.DefaultInteractiveSelect.
			WithOptions(config.NodeVersions).
			WithDefaultOption(config.DefaultNode).
			WithFilter(true).
			Show("Node version")
		if err != nil {
			return err
		}

		packageManager, err := pterm.DefaultInteractiveSelect.
			WithOptions([]string{"npm", "yarn", "pnpm"}).
			WithDefaultOption(config.DefaultPackageManager).
			WithFilter(true).
			Show("Package manager")
		if err != nil {
			return err
		}

		nodeCommand, err := pterm.DefaultInteractiveTextInput.
			WithDefaultValue("").
			Show("Node dev command (e.g. yarn run hot, leave empty to skip)")
		if err != nil {
			return err
		}

		var ports []string
		if nodeCommand != "" {
			portStr, err := pterm.DefaultInteractiveTextInput.
				WithDefaultValue("").
				Show("Dev server port (e.g. 8080, leave empty to skip)")
			if err != nil {
				return err
			}
			if portStr != "" {
				ports = []string{portStr}
			}
		}

		// --- Database ---
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgDefault)).
			WithTextStyle(pterm.NewStyle(pterm.Bold)).Println("Database")

		dbDriver, err := pterm.DefaultInteractiveSelect.
			WithOptions([]string{"mysql", "postgres"}).
			WithDefaultOption("mysql").
			WithFilter(true).
			Show("Driver")
		if err != nil {
			return err
		}

		dbVersionOptions := config.MySQLVersions
		defaultDBVer := config.DefaultMySQLVersion
		if dbDriver == "postgres" {
			dbVersionOptions = config.PostgresVersions
			defaultDBVer = config.DefaultPostgresVersion
		}

		dbVersion, err := pterm.DefaultInteractiveSelect.
			WithOptions(dbVersionOptions).
			WithDefaultOption(defaultDBVer).
			WithFilter(true).
			Show("Version")
		if err != nil {
			return err
		}

		defaultDBName := config.SanitizeName(projectName, true)
		dbName, err := pterm.DefaultInteractiveTextInput.
			WithDefaultValue(defaultDBName).
			Show("Database name")
		if err != nil {
			return err
		}

		// --- Services ---
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgDefault)).
			WithTextStyle(pterm.NewStyle(pterm.Bold)).Println("Services")

		redisVersion, err := pterm.DefaultInteractiveSelect.
			WithOptions(config.RedisVersions).
			WithDefaultOption(config.DefaultRedisVersion).
			WithFilter(true).
			Show("Redis version")
		if err != nil {
			return err
		}

		// Discover shared services from other projects
		var selectedShared map[string]config.ServiceDefinition
		global, err := config.LoadGlobal()
		if err == nil {
			collected := config.CollectVersions(global.ProjectsDir, &config.ProjectConfig{})
			if len(collected.SharedServices) > 0 {
				names := make([]string, 0, len(collected.SharedServices))
				for name := range collected.SharedServices {
					names = append(names, name)
				}
				sort.Strings(names)

				picked, err := pterm.DefaultInteractiveMultiselect.
					WithOptions(names).
					WithFilter(true).
					Show("Shared services (space to select, enter to confirm)")
				if err != nil {
					return err
				}

				if len(picked) > 0 {
					selectedShared = make(map[string]config.ServiceDefinition, len(picked))
					for _, name := range picked {
						selectedShared[name] = collected.SharedServices[name]
					}
				}
			}
		}

		// Build config
		cfg := &initConfig{
			Domain:         domain,
			PHP:            php,
			Node:           node,
			PackageManager: packageManager,
			NodeCommand:    nodeCommand,
			Ports:          ports,
			DBDriver:       dbDriver,
			DBVersion:      dbVersion,
			DB:             dbName,
			RedisVersion:   redisVersion,
			SharedServices: selectedShared,
		}
		if projectType != "other" {
			cfg.Type = projectType
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", config.ConfigFile, err)
		}

		// Create .gitignore to exclude generated files but keep config
		gitignore := "*\n!config.yaml\n!.gitignore\n"
		if err := os.WriteFile(filepath.Join(novaDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
			return fmt.Errorf("writing .nova/.gitignore: %w", err)
		}

		// Summary
		fmt.Println()
		pterm.DefaultSection.Println("Summary")
		fmt.Printf("  Type:     %s\n", projectType)
		fmt.Printf("  Domain:   %s\n", domain)
		fmt.Printf("  PHP:      %s\n", php)
		nodeInfo := fmt.Sprintf("%s (%s)", node, packageManager)
		if nodeCommand != "" {
			nodeInfo += fmt.Sprintf(" — %s", nodeCommand)
			if len(ports) > 0 {
				nodeInfo += fmt.Sprintf(" on port %s", ports[0])
			}
		}
		fmt.Printf("  Node:     %s\n", nodeInfo)
		fmt.Printf("  Database: %s %s (%s)\n", dbDriver, dbVersion, dbName)
		fmt.Printf("  Redis:    %s\n", redisVersion)
		if len(selectedShared) > 0 {
			shared := make([]string, 0, len(selectedShared))
			for name := range selectedShared {
				shared = append(shared, name)
			}
			sort.Strings(shared)
			fmt.Printf("  Shared:   %s\n", strings.Join(shared, ", "))
		}
		fmt.Println()
		pterm.Success.Printfln("Created %s", config.ConfigFile)
		pterm.Info.Printfln("Run %s to get going.", pterm.LightCyan("nova start"))
		fmt.Println()

		return nil
	},
}

// initConfig controls which fields are written to .nova/config.yaml.
type initConfig struct {
	Type           string                              `yaml:"type,omitempty"`
	Domain         string                              `yaml:"domain"`
	PHP            string                              `yaml:"php"`
	Node           string                              `yaml:"node"`
	PackageManager string                              `yaml:"package_manager"`
	NodeCommand    string                              `yaml:"node_command,omitempty"`
	Ports          []string                            `yaml:"ports,omitempty"`
	DBDriver       string                              `yaml:"db_driver"`
	DBVersion      string                              `yaml:"db_version"`
	DB             string                              `yaml:"db"`
	RedisVersion   string                              `yaml:"redis_version"`
	SharedServices map[string]config.ServiceDefinition `yaml:"shared_services,omitempty"`
}
