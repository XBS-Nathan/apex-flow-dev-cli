package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/XBS-Nathan/nova/internal/config"
)

func init() { rootCmd.AddCommand(initCmd) }

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a .dev.yaml config for the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		path := filepath.Join(cwd, config.ConfigFile)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists — edit it directly or delete it first", config.ConfigFile)
		}

		projectName := filepath.Base(cwd)
		scanner := bufio.NewScanner(os.Stdin)
		prompt := func(label, defaultVal string) string {
			fmt.Printf("  \033[36m?\033[0m %s \033[2m(%s)\033[0m: ", label, defaultVal)
			if scanner.Scan() {
				if v := strings.TrimSpace(scanner.Text()); v != "" {
					return v
				}
			}
			return defaultVal
		}

		fmt.Println()
		fmt.Printf("  \033[1mdev init\033[0m — %s\n", projectName)
		fmt.Println()

		// Auto-detect project type
		detectedType := config.TypeGeneric
		if _, err := os.Stat(filepath.Join(cwd, "artisan")); err == nil {
			detectedType = config.TypeLaravel
		}

		if detectedType != config.TypeGeneric {
			fmt.Printf("  \033[32m✓\033[0m Detected %s project\n", detectedType)
			fmt.Println()
		}

		// --- General ---
		fmt.Println("  \033[1mGeneral\033[0m")
		projectType := prompt("Project type", detectedType)
		domain := prompt("Domain", projectName+".test")
		if !strings.HasSuffix(domain, ".test") {
			domain += ".test"
		}
		fmt.Println()

		// --- Language ---
		fmt.Println("  \033[1mLanguage\033[0m")
		php := prompt("PHP version", config.DefaultPHP)
		node := prompt("Node version", config.DefaultNode)
		packageManager := prompt("Package manager (npm/yarn/pnpm)", config.DefaultPackageManager)
		fmt.Println()

		// --- Database ---
		fmt.Println("  \033[1mDatabase\033[0m")
		dbDriver := prompt("Driver", "mysql")
		defaultDBVer := config.DefaultMySQLVersion
		if dbDriver == "postgres" {
			defaultDBVer = config.DefaultPostgresVersion
		}
		dbVersion := prompt("Version", defaultDBVer)
		dbName := prompt("Name", config.SanitizeName(projectName, true))
		fmt.Println()

		// --- Services ---
		fmt.Println("  \033[1mServices\033[0m")
		redisVersion := prompt("Redis version", config.DefaultRedisVersion)
		fmt.Println()

		// Build config
		cfg := &initConfig{
			Domain:         domain,
			PHP:            php,
			Node:           node,
			PackageManager: packageManager,
			DBDriver:       dbDriver,
			DBVersion:    dbVersion,
			DB:           dbName,
			RedisVersion: redisVersion,
		}
		if projectType != config.TypeGeneric {
			cfg.Type = projectType
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", config.ConfigFile, err)
		}

		// Summary
		fmt.Println("  \033[1mSummary\033[0m")
		fmt.Printf("  \033[2m├─\033[0m Type:     %s\n", projectType)
		fmt.Printf("  \033[2m├─\033[0m Domain:   %s\n", domain)
		fmt.Printf("  \033[2m├─\033[0m PHP:      %s\n", php)
		fmt.Printf("  \033[2m├─\033[0m Node:     %s (%s)\n", node, packageManager)
		fmt.Printf("  \033[2m├─\033[0m Database: %s %s (%s)\n", dbDriver, dbVersion, dbName)
		fmt.Printf("  \033[2m└─\033[0m Redis:    %s\n", redisVersion)
		fmt.Println()
		fmt.Printf("  \033[32m✓\033[0m Created %s\n", config.ConfigFile)
		fmt.Printf("  Run \033[1mdev start\033[0m to get going.\n")
		fmt.Println()

		return nil
	},
}

// initConfig controls which fields are written to .dev.yaml.
type initConfig struct {
	Type           string `yaml:"type,omitempty"`
	Domain         string `yaml:"domain"`
	PHP            string `yaml:"php"`
	Node           string `yaml:"node"`
	PackageManager string `yaml:"package_manager"`
	DBDriver       string `yaml:"db_driver"`
	DBVersion    string `yaml:"db_version"`
	DB           string `yaml:"db"`
	RedisVersion string `yaml:"redis_version"`
}
