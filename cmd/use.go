package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(useCmd)
	useCmd.AddCommand(usePhpCmd)
	useCmd.AddCommand(useNodeCmd)
	useCmd.AddCommand(useDbDriverCmd)
}

var useCmd = &cobra.Command{
	Use:   "use",
	Short: "Set project configuration values",
}

var usePhpCmd = &cobra.Command{
	Use:   "php <version>",
	Short: "Set the PHP version for this project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigField("php", args[0])
	},
}

var useNodeCmd = &cobra.Command{
	Use:   "node <version>",
	Short: "Set the Node version for this project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigField("node", args[0])
	},
}

var useDbDriverCmd = &cobra.Command{
	Use:   "db <driver>",
	Short: "Set the database driver (mysql or postgres)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		driver := args[0]
		if driver != "mysql" && driver != "postgres" {
			return fmt.Errorf("unsupported driver %q — use mysql or postgres", driver)
		}
		return setConfigField("db_driver", driver)
	},
}

// setConfigField reads the existing .dev.yaml (or creates one), sets a field, and writes it back.
func setConfigField(key, value string) error {
	p, err := project.Detect()
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(p.Dir, config.ConfigFile)

	// Read existing YAML as a map to preserve unknown fields
	data := make(map[string]interface{})
	existing, err := os.ReadFile(cfgPath)
	if err == nil {
		if err := yaml.Unmarshal(existing, &data); err != nil {
			return fmt.Errorf("parsing %s: %w", cfgPath, err)
		}
	}

	data[key] = value

	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", cfgPath, err)
	}

	fmt.Printf("✓ Set %s = %s in %s\n", key, value, cfgPath)
	return nil
}
