package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/hosts"
)

func init() { rootCmd.AddCommand(trustCmd) }

var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust the Caddy local CA certificate",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !docker.IsUp() {
			return fmt.Errorf("services not running — run 'dev start' first to generate the CA certificate")
		}

		// Copy cert from the Caddy container to the host
		certPath := filepath.Join(config.GlobalDir(), "caddy-root-ca.crt")
		cpCmd := exec.Command("docker", "compose", "-f", docker.ComposeFile(),
			"cp", "caddy:/data/caddy/pki/authorities/local/root.crt", certPath)
		cpCmd.Stderr = os.Stderr
		if err := cpCmd.Run(); err != nil {
			return fmt.Errorf("extracting CA cert from Caddy container: %w", err)
		}

		fmt.Println("Installing Caddy local CA certificate...")

		switch runtime.GOOS {
		case "linux":
			if err := installCertLinux(certPath); err != nil {
				return err
			}
			if hosts.IsWSL2() {
				if err := installCertWindows(certPath); err != nil {
					return fmt.Errorf("windows cert trust: %w", err)
				}
				if err := enableFirefoxEnterpriseRoots(); err != nil {
					fmt.Printf("  ! Firefox: %s\n", err)
					fmt.Println("  You may need to manually import the cert in Firefox")
				}
			}
		case "darwin":
			if err := installCertMacOS(certPath); err != nil {
				return err
			}
		}

		fmt.Println("✓ Caddy CA certificate trusted")
		return nil
	},
}

func installCertLinux(certPath string) error {
	dest := "/usr/local/share/ca-certificates/dev-caddy.crt"
	cp := exec.Command("sudo", "cp", certPath, dest)
	cp.Stdout = os.Stdout
	cp.Stderr = os.Stderr
	if err := cp.Run(); err != nil {
		return fmt.Errorf("copying cert: %w", err)
	}
	update := exec.Command("sudo", "update-ca-certificates")
	update.Stdout = os.Stdout
	update.Stderr = os.Stderr
	if err := update.Run(); err != nil {
		return fmt.Errorf("updating ca certificates: %w", err)
	}
	return nil
}

func installCertMacOS(certPath string) error {
	cmd := exec.Command("sudo", "security", "add-trusted-cert",
		"-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		certPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trusting cert on macOS: %w", err)
	}
	return nil
}

func installCertWindows(certPath string) error {
	// Convert WSL path to Windows path
	winPath, err := exec.Command("wslpath", "-w", certPath).Output()
	if err != nil {
		return fmt.Errorf("converting path for Windows: %w", err)
	}

	winCertPath := strings.TrimSpace(string(winPath))

	psCommand := fmt.Sprintf(
		"Import-Certificate -FilePath '%s' -CertStoreLocation Cert:\\LocalMachine\\Root",
		winCertPath,
	)

	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf(
			"Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile','-Command','%s'",
			strings.ReplaceAll(psCommand, "'", "''"),
		),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trusting cert on Windows: %w", err)
	}
	return nil
}

// enableFirefoxEnterpriseRoots creates a policies.json that tells Firefox
// to trust certificates from the Windows system store. Works for both
// Firefox and Firefox Developer Edition.
func enableFirefoxEnterpriseRoots() error {
	policy := `{"policies":{"Certificates":{"ImportEnterpriseRoots":true}}}`

	// Common Firefox install paths on Windows (accessed via /mnt/c)
	firefoxDirs := []string{
		"/mnt/c/Program Files/Mozilla Firefox",
		"/mnt/c/Program Files/Firefox Developer Edition",
		"/mnt/c/Program Files (x86)/Mozilla Firefox",
	}

	installed := false
	for _, dir := range firefoxDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}

		distDir := filepath.Join(dir, "distribution")
		policyPath := filepath.Join(distDir, "policies.json")

		// Check if policy already exists with the right content
		if data, err := os.ReadFile(policyPath); err == nil {
			if strings.Contains(string(data), "ImportEnterpriseRoots") {
				fmt.Printf("  ✓ Firefox policy already set: %s\n", policyPath)
				installed = true
				continue
			}
		}

		// Need elevated PowerShell to write to Program Files
		winDir, err := exec.Command("wslpath", "-w", dir).Output()
		if err != nil {
			continue
		}
		winDistDir := strings.TrimSpace(string(winDir)) + `\distribution`
		winPolicyPath := winDistDir + `\policies.json`

		psCmd := fmt.Sprintf(
			"New-Item -ItemType Directory -Force -Path '%s'; Set-Content -Path '%s' -Value '%s'",
			winDistDir, winPolicyPath, policy,
		)

		cmd := exec.Command("powershell.exe",
			"-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf(
				"Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile','-Command','%s'",
				strings.ReplaceAll(psCmd, "'", "''"),
			),
		)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("  ! Could not write Firefox policy: %s\n", err)
			continue
		}

		fmt.Printf("  ✓ Firefox enterprise roots enabled: %s\n", dir)
		installed = true
	}

	if !installed {
		return fmt.Errorf("no Firefox installation found")
	}

	fmt.Println("  Restart Firefox for the change to take effect")
	return nil
}
