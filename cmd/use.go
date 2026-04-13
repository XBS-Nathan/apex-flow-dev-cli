package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/phpimage"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() {
	rootCmd.AddCommand(useCmd)
	useCmd.AddCommand(usePhpCmd)
	useCmd.AddCommand(useNodeCmd)
	useCmd.AddCommand(useDbDriverCmd)
	useCmd.AddCommand(useServiceCmd)
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
		version := args[0]
		if err := setConfigField("php", version); err != nil {
			return err
		}
		return ensurePHPRunning(version)
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

var useServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Add a shared service from another project",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}

		collected := config.CollectVersions(global.ProjectsDir, p.Config)
		if len(collected.SharedServices) == 0 {
			return fmt.Errorf("no shared services found in any project under %s", global.ProjectsDir)
		}

		// Filter out services already in this project
		available := make(map[string]config.ServiceDefinition)
		for name, svc := range collected.SharedServices {
			if _, exists := p.Config.SharedServices[name]; !exists {
				available[name] = svc
			}
		}
		if len(available) == 0 {
			fmt.Println("✓ All available shared services are already configured")
			return nil
		}

		names := make([]string, 0, len(available))
		for name := range available {
			names = append(names, name)
		}
		sort.Strings(names)

		picked, err := pterm.DefaultInteractiveMultiselect.
			WithOptions(names).
			WithFilter(true).
			Show("Select shared services to add (space to select, enter to confirm)")
		if err != nil {
			return err
		}
		if len(picked) == 0 {
			return nil
		}

		// Read existing config as map to preserve all fields
		cfgPath := filepath.Join(p.Dir, config.ConfigFile)
		data := make(map[string]interface{})
		existing, err := os.ReadFile(cfgPath)
		if err == nil {
			if err := yaml.Unmarshal(existing, &data); err != nil {
				return fmt.Errorf("parsing %s: %w", cfgPath, err)
			}
		}

		// Merge selected services into shared_services
		sharedRaw, _ := data["shared_services"].(map[string]interface{})
		if sharedRaw == nil {
			sharedRaw = make(map[string]interface{})
		}
		for _, name := range picked {
			svc := available[name]
			// Convert ServiceDefinition to a map for YAML marshaling
			svcBytes, _ := yaml.Marshal(svc)
			var svcMap interface{}
			yaml.Unmarshal(svcBytes, &svcMap)
			sharedRaw[name] = svcMap
		}
		data["shared_services"] = sharedRaw

		out, err := yaml.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(cfgPath), err)
		}

		if err := os.WriteFile(cfgPath, out, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", cfgPath, err)
		}

		for _, name := range picked {
			fmt.Printf("✓ Added shared service %s\n", name)
		}
		fmt.Println("  Run nova restart to apply changes.")
		return nil
	},
}

// setConfigField reads the existing .nova/config.yaml (or creates one), sets a field, and writes it back.
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

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(cfgPath), err)
	}

	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", cfgPath, err)
	}

	fmt.Printf("✓ Set %s = %s in %s\n", key, value, cfgPath)
	return nil
}

// ensurePHPRunning builds the PHP image and brings up services if Docker is already running.
func ensurePHPRunning(version string) error {
	if !docker.IsUp() {
		return nil
	}

	p, err := project.Detect()
	if err != nil {
		return err
	}

	global, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	// Pre-create conf.d directory before Docker can create it as root
	confDir := filepath.Join(config.GlobalDir(), "php", version, "conf.d")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("creating php conf.d dir: %w", err)
	}

	imgCfg := phpimage.ImageConfig{
		PHPVersion: version,
		Extensions: p.Config.Extensions,
	}
	if _, err := phpimage.EnsureBuilt(imgCfg); err != nil {
		return fmt.Errorf("building PHP %s image: %w", version, err)
	}

	php := []docker.PHPVersion{{
		Version:    version,
		Extensions: p.Config.Extensions,
		Ports:      p.Config.Ports,
	}}

	svc := docker.Service{
		ProjectsDir:    global.ProjectsDir,
		Collected:      config.CollectVersions(global.ProjectsDir, p.Config),
		MailpitVersion: global.Versions.Mailpit,
	}
	if err := svc.Up(php, false); err != nil {
		return fmt.Errorf("starting PHP %s service: %w", version, err)
	}

	fmt.Printf("✓ PHP %s service is running\n", version)
	return nil
}
