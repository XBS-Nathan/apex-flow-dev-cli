package caddy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

const caddyDir = "caddy"

// SiteConfigPath returns the path to a site's Caddy config file.
func SiteConfigPath(siteName string) string {
	dir := filepath.Join(config.GlobalDir(), caddyDir, "sites")
	_ = os.MkdirAll(dir, 0755) // errors surface when caller writes
	return filepath.Join(dir, siteName+".caddy")
}

// CaddyfilePath returns the path to the main Caddyfile.
func CaddyfilePath() string {
	dir := filepath.Join(config.GlobalDir(), caddyDir)
	_ = os.MkdirAll(dir, 0755) // errors surface when caller writes
	return filepath.Join(dir, "Caddyfile")
}

// Link creates a Caddy site config for a project.
func Link(siteName, projectDir, fpmSocket string) error {
	docroot := filepath.Join(projectDir, "public")

	siteConfig := fmt.Sprintf(`%s.test {
	root * %s
	php_fastcgi unix/%s
	file_server
	encode gzip

	log {
		output file %s/logs/%s.log
	}
}
`, siteName, docroot, fpmSocket, config.GlobalDir(), siteName)

	if err := os.WriteFile(SiteConfigPath(siteName), []byte(siteConfig), 0644); err != nil {
		return fmt.Errorf("writing site config: %w", err)
	}

	if err := writeMainCaddyfile(); err != nil {
		return err
	}

	return Reload()
}

// Unlink removes a Caddy site config.
func Unlink(siteName string) error {
	_ = os.Remove(SiteConfigPath(siteName)) // may already be absent

	if err := writeMainCaddyfile(); err != nil {
		return err
	}

	return Reload()
}

// writeMainCaddyfile generates the main Caddyfile that imports all site configs.
func writeMainCaddyfile() error {
	content := fmt.Sprintf("import %s/caddy/sites/*.caddy\n", config.GlobalDir())
	return os.WriteFile(CaddyfilePath(), []byte(content), 0644)
}

// Reload tells Caddy to reload its configuration.
func Reload() error {
	cmd := exec.Command("caddy", "reload", "--config", CaddyfilePath())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy reload: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// IsRunning checks if Caddy is currently running.
func IsRunning() bool {
	cmd := exec.Command("pgrep", "-x", "caddy")
	return cmd.Run() == nil
}

// Start starts Caddy if not already running.
func Start() error {
	if IsRunning() {
		return nil
	}

	if err := writeMainCaddyfile(); err != nil {
		return err
	}

	cmd := exec.Command("caddy", "start", "--config", CaddyfilePath())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy start: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// Stop stops Caddy.
func Stop() error {
	if !IsRunning() {
		return nil
	}
	cmd := exec.Command("caddy", "stop")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("caddy stop: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
