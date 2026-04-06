package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(xdebugCmd) }

var xdebugCmd = &cobra.Command{
	Use:   "xdebug [on|off]",
	Short: "Toggle Xdebug for the project's PHP version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		version := p.Config.PHP
		iniDir := filepath.Join(config.GlobalDir(), "php", version, "conf.d")
		iniPath := filepath.Join(iniDir, "xdebug.ini")
		svc := docker.PHPServiceName(version)

		switch args[0] {
		case "on":
			fmt.Printf("Enabling Xdebug for PHP %s...\n", version)
			if err := os.MkdirAll(iniDir, 0755); err != nil {
				return fmt.Errorf("creating conf.d dir: %w", err)
			}
			ini := "zend_extension=xdebug\nxdebug.mode=debug\nxdebug.client_host=host.docker.internal\nxdebug.start_with_request=yes\n"
			if err := os.WriteFile(iniPath, []byte(ini), 0644); err != nil {
				return fmt.Errorf("writing xdebug.ini: %w", err)
			}
		case "off":
			fmt.Printf("Disabling Xdebug for PHP %s...\n", version)
			_ = os.Remove(iniPath) // may not exist
		default:
			return fmt.Errorf("usage: dev xdebug [on|off]")
		}

		// Graceful PHP-FPM reload (no container restart)
		fmt.Printf("  → Reloading PHP-FPM...\n")
		if err := docker.Exec(svc, "/srv", "kill", "-USR2", "1"); err != nil {
			return fmt.Errorf("reloading PHP-FPM: %w", err)
		}

		fmt.Printf("✓ Xdebug %s for PHP %s\n", args[0], version)
		return nil
	},
}
