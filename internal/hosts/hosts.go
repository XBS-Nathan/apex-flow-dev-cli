package hosts

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ensureEntry reads path and appends "127.0.0.1 <domain>" if not already present.
// This is the testable core — it writes directly without sudo.
func ensureEntry(path, domain string) error {
	entry := fmt.Sprintf("127.0.0.1 %s", domain)

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}

	if strings.Contains(string(data), entry) {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open hosts file for append: %w", err)
	}
	defer f.Close()

	line := entry + "\n"
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("append to hosts file: %w", err)
	}

	return nil
}

// ensureWithSudo reads path to check if entry exists, then uses sudo sh -c to append if needed.
func ensureWithSudo(path, domain string) error {
	entry := fmt.Sprintf("127.0.0.1 %s", domain)

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}

	if strings.Contains(string(data), entry) {
		return nil
	}

	line := entry + "\n"
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo %q >> %s", line, path))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo append to %s: %w", path, err)
	}

	return nil
}

// isWSL2 reports whether the current environment is WSL2 by inspecting /proc/version.
func isWSL2() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// Ensure ensures that "127.0.0.1 <domain>" exists in /etc/hosts.
// On WSL2 it also writes to the Windows hosts file.
func Ensure(domain string) error {
	const linuxHosts = "/etc/hosts"

	if err := ensureWithSudo(linuxHosts, domain); err != nil {
		return err
	}

	if isWSL2() {
		const windowsHosts = "/mnt/c/Windows/System32/drivers/etc/hosts"
		if err := ensureWithSudo(windowsHosts, domain); err != nil {
			return err
		}
	}

	return nil
}

// Service is an adapter that satisfies the lifecycle.HostsService interface.
type Service struct{}

// Ensure delegates to the package-level Ensure function.
func (s Service) Ensure(domain string) error { return Ensure(domain) }
