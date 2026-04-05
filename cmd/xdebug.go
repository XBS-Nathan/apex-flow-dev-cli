package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/php"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(xdebugCmd)
}

var xdebugCmd = &cobra.Command{
	Use:   "xdebug [on|off]",
	Short: "Toggle Xdebug for the project's PHP version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		action := args[0]
		version := p.Config.PHP
		fpmService := php.FPMServiceName(version)

		switch action {
		case "on":
			fmt.Printf("Enabling Xdebug for PHP %s...\n", version)
			if err := toggleXdebugModule(version, true); err != nil {
				return err
			}
		case "off":
			fmt.Printf("Disabling Xdebug for PHP %s...\n", version)
			if err := toggleXdebugModule(version, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("usage: dev xdebug [on|off]")
		}

		// Restart PHP-FPM
		fmt.Printf("  → Restarting %s...\n", fpmService)
		if err := exec.Command("sudo", "systemctl", "restart", fpmService).Run(); err != nil {
			return fmt.Errorf("restarting PHP-FPM: %w", err)
		}

		fmt.Printf("✓ Xdebug %s for PHP %s\n", action, version)
		return nil
	},
}

func toggleXdebugModule(phpVersion string, enable bool) error {
	tool := "phpdismod"
	if enable {
		tool = "phpenmod"
	}

	cmd := exec.Command("sudo", tool, "-v", phpVersion, "xdebug")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s: %w", tool, string(output), err)
	}
	return nil
}
