package caddy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XBS-Nathan/nova/internal/config"
)

// PortProxy maps a host-facing port to the backend service that handles it.
type PortProxy struct {
	Port    string // e.g. "8080"
	Backend string // e.g. "node-xlinx"
}

// Link creates a Caddy site config and reloads.
func Link(siteName, docroot, phpService string, portProxies []PortProxy) error {
	caddyDir := filepath.Join(config.GlobalDir(), "caddy")
	if err := writeSiteConfig(caddyDir, siteName, docroot, phpService, portProxies); err != nil {
		return err
	}
	if err := writeMainCaddyfile(caddyDir); err != nil {
		return err
	}
	return Reload()
}

// Unlink removes a site config and reloads.
func Unlink(siteName string) error {
	caddyDir := filepath.Join(config.GlobalDir(), "caddy")
	removeSiteConfig(caddyDir, siteName)
	return Reload()
}

// Reload tells the Caddy container to reload its config.
func Reload() error {
	cmd := exec.Command("docker", "compose", "-f",
		filepath.Join(config.GlobalDir(), "docker-compose.yml"),
		"exec", "caddy", "caddy", "reload",
		"--config", "/etc/caddy/Caddyfile")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy reload: %s: %w",
			strings.TrimSpace(string(output)), err)
	}
	return nil
}

// SiteConfigPath returns the path for a site's config file.
func SiteConfigPath(siteName string) string {
	return filepath.Join(config.GlobalDir(), "caddy", "sites", siteName+".caddy")
}

func writeSiteConfig(caddyDir, siteName, docroot, phpService string, portProxies []PortProxy) error {
	sitesDir := filepath.Join(caddyDir, "sites")
	if err := os.MkdirAll(sitesDir, 0755); err != nil {
		return fmt.Errorf("creating sites dir: %w", err)
	}

	content := generateSiteConfig(siteName, docroot, phpService, portProxies)
	path := filepath.Join(sitesDir, siteName+".caddy")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing site config: %w", err)
	}
	return nil
}

func removeSiteConfig(caddyDir, siteName string) {
	path := filepath.Join(caddyDir, "sites", siteName+".caddy")
	_ = os.Remove(path) // may already be absent
}

func writeMainCaddyfile(caddyDir string) error {
	content := generateMainCaddyfile()
	path := filepath.Join(caddyDir, "Caddyfile")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing Caddyfile: %w", err)
	}
	return nil
}

func generateSiteConfig(siteName, docroot, phpService string, portProxies []PortProxy) string {
	var b strings.Builder

	// Main site block
	fmt.Fprintf(&b, "%s.test {\n", siteName)
	fmt.Fprintf(&b, "\troot * %s\n", docroot)
	fmt.Fprintf(&b, "\tphp_fastcgi %s:9000\n", phpService)
	b.WriteString("\tfile_server\n")
	b.WriteString("\tencode gzip\n")
	b.WriteString("}\n")

	// Extra port blocks — SSL-terminated reverse proxy to backend services
	for _, pp := range portProxies {
		fmt.Fprintf(&b, "\n%s.test:%s {\n", siteName, pp.Port)
		fmt.Fprintf(&b, "\treverse_proxy %s:%s\n", pp.Backend, pp.Port)
		b.WriteString("}\n")
	}

	return b.String()
}

func generateMainCaddyfile() string {
	return "{\n\tlocal_certs\n}\n\nimport /etc/caddy/sites/*.caddy\n"
}
