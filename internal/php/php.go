package php

import (
	"fmt"
	"os"
	"os/exec"
)

// Run executes a command using the specified PHP version.
func Run(phpVersion string, args ...string) error {
	bin := fmt.Sprintf("php%s", phpVersion)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// FPMSocket returns the PHP-FPM socket path for a version.
func FPMSocket(version string) string {
	return fmt.Sprintf("/run/php/php%s-fpm.sock", version)
}

// FPMServiceName returns the systemd service name for a PHP-FPM version.
func FPMServiceName(version string) string {
	return fmt.Sprintf("php%s-fpm", version)
}
