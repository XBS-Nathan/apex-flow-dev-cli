package cmd

import (
	"fmt"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() {
	rootCmd.AddCommand(slowCmd)
	slowCmd.AddCommand(slowOnCmd)
	slowCmd.AddCommand(slowOffCmd)

	slowOnCmd.Flags().String("cpu", "0.5", "CPU limit (e.g. 0.25, 0.5, 1)")
	slowOnCmd.Flags().String("memory", "256m", "Memory limit (e.g. 128m, 256m, 512m)")
}

var slowCmd = &cobra.Command{
	Use:   "slow",
	Short: "Simulate a slow server with resource constraints",
	Long: `Apply CPU and memory limits to PHP containers to simulate
a slow or resource-constrained production server.

  nova slow on                        # default: 0.5 CPU, 256MB RAM
  nova slow on --cpu 0.25 --memory 128m  # custom limits
  nova slow off                       # remove constraints`,
	RunE: func(cmd *cobra.Command, args []string) error {
		throttle := config.LoadThrottle()
		if throttle != nil {
			pterm.Info.Printfln("Throttle active: CPU %s, Memory %s", throttle.CPUs, throttle.Memory)
		} else {
			pterm.Info.Println("Throttle is off")
		}
		return nil
	},
}

var slowOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Enable resource constraints",
	RunE: func(cmd *cobra.Command, args []string) error {
		cpu, _ := cmd.Flags().GetString("cpu")
		memory, _ := cmd.Flags().GetString("memory")

		throttle := &config.ThrottleConfig{
			CPUs:   cpu,
			Memory: memory,
		}
		if err := config.SaveThrottle(throttle); err != nil {
			return err
		}

		pterm.Success.Printfln("Throttle enabled: CPU %s, Memory %s", cpu, memory)

		// Regenerate compose and recreate PHP containers
		return restartServices()
	},
}

var slowOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Remove resource constraints",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.ClearThrottle(); err != nil {
			return err
		}

		pterm.Success.Println("Throttle disabled")

		// Regenerate compose and recreate PHP containers
		return restartServices()
	},
}

// restartServices regenerates the compose file and recreates containers.
func restartServices() error {
	if !docker.IsUp() {
		pterm.Info.Println("Services not running — throttle will apply on next start")
		return nil
	}

	p, err := project.Detect()
	if err != nil {
		return fmt.Errorf("detecting project: %w", err)
	}

	global, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	php := []docker.PHPVersion{{
		Version:    p.Config.PHP,
		Extensions: p.Config.Extensions,
		Ports:      p.Config.Ports,
	}}

	svc := docker.Service{
		ProjectsDir:    global.ProjectsDir,
		Collected:      config.CollectVersions(global.ProjectsDir, p.Config),
		MailpitVersion: global.Versions.Mailpit,
	}
	return svc.Up(php, true)
}
