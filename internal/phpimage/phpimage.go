package phpimage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/XBS-Nathan/nova/internal/config"
)

const baseExtensions = "pdo_mysql pdo_pgsql opcache pcntl bcmath"

// baseBuildDeps are Debian packages needed to compile the base extensions.
var baseBuildDeps = []string{"libpq-dev"}

// baseRuntimeDeps are Debian packages needed at runtime by base extensions.
// (`bash` and `linux-headers` are part of the Debian base image so they do
// not need explicit installation, unlike on Alpine.)
var baseRuntimeDeps = []string{"libpq5", "default-mysql-client", "postgresql-client"}

// nativeExtDeps maps extensions to their Debian build-time (-dev) packages.
var nativeExtDeps = map[string][]string{
	"gd":      {"libpng-dev", "libjpeg-dev", "libfreetype6-dev"},
	"zip":     {"libzip-dev"},
	"intl":    {"libicu-dev"},
	"exif":    {},
	"soap":    {"libxml2-dev"},
	"sockets": {},
}

// nativeExtRuntime maps extensions to their Debian runtime packages
// (must remain after build deps are removed).
var nativeExtRuntime = map[string][]string{
	"gd":      {"libpng16-16", "libjpeg62-turbo", "libfreetype6"},
	"zip":     {"libzip4"},
	"intl":    {"libicu72"},
	"exif":    {},
	"soap":    {"libxml2"},
	"sockets": {},
}

// ImageConfig holds everything needed to build a PHP image.
type ImageConfig struct {
	PHPVersion string
	Extensions []string
	Runtime    string // "fpm" (default) or "frankenphp"
}

// runtime returns the runtime, defaulting to "fpm" when unset.
func (c ImageConfig) runtime() string {
	if c.Runtime == "" {
		return "fpm"
	}
	return c.Runtime
}

// EnsureBuilt builds the PHP-FPM image if it doesn't already exist.
// Returns true if a new image was built.
func EnsureBuilt(cfg ImageConfig) (bool, error) {
	tag := ImageTag(cfg)

	cmd := exec.Command("docker", "image", "inspect", tag)
	if cmd.Run() == nil {
		return false, nil // already built
	}

	return true, buildImage(cfg, true)
}

// ForceBuild removes the existing image and rebuilds from scratch.
func ForceBuild(cfg ImageConfig) error {
	tag := ImageTag(cfg)
	// Remove existing image if present
	_ = exec.Command("docker", "rmi", "-f", tag).Run()
	return buildImage(cfg, true)
}

func buildImage(cfg ImageConfig, noCache bool) error {
	tag := ImageTag(cfg)

	dir, err := writeDockerfile(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("  → Building PHP %s image...\n", cfg.PHPVersion)
	args := []string{"build", "-t", tag}
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, dir)
	build := exec.Command("docker", args...)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building php %s image: %w", cfg.PHPVersion, err)
	}

	return nil
}

// ImageTag returns the Docker image tag for an image config.
func ImageTag(cfg ImageConfig) string {
	hash := imageHash(cfg)
	return fmt.Sprintf("nova-%s:%s-%s", cfg.runtime(), cfg.PHPVersion, hash)
}

func writeDockerfile(cfg ImageConfig) (string, error) {
	dir := filepath.Join(config.GlobalDir(), "dockerfiles", "php", cfg.PHPVersion)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating dockerfile dir: %w", err)
	}

	content := generateDockerfile(cfg)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing Dockerfile: %w", err)
	}

	phpIni := "[PHP]\nmemory_limit = 512M\nupload_max_filesize = 100M\npost_max_size = 100M\n"
	if err := os.WriteFile(filepath.Join(dir, "php.ini"), []byte(phpIni), 0644); err != nil {
		return "", fmt.Errorf("writing php.ini: %w", err)
	}

	// Disable SSL requirement for MariaDB/MySQL client (dev environment)
	myCnf := "[client]\nssl = 0\n"
	if err := os.WriteFile(filepath.Join(dir, "my.cnf"), []byte(myCnf), 0644); err != nil {
		return "", fmt.Errorf("writing my.cnf: %w", err)
	}

	if cfg.runtime() == "frankenphp" {
		caddyfile := `{
    frankenphp
    auto_https off
    admin off
}
:8000 {
    root * /srv/{$NOVA_APP}/public
    php_server
}
`
		if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddyfile), 0644); err != nil {
			return "", fmt.Errorf("writing Caddyfile: %w", err)
		}
	}

	return dir, nil
}

func generateDockerfile(cfg ImageConfig) string {
	var native, pecl []string
	var buildDeps, runtimeDeps []string

	// Sort extensions for deterministic Dockerfile output
	sorted := make([]string, len(cfg.Extensions))
	copy(sorted, cfg.Extensions)
	sort.Strings(sorted)

	for _, ext := range sorted {
		if deps, ok := nativeExtDeps[ext]; ok {
			native = append(native, ext)
			buildDeps = append(buildDeps, deps...)
			runtimeDeps = append(runtimeDeps, nativeExtRuntime[ext]...)
		} else {
			pecl = append(pecl, ext)
		}
	}

	var b strings.Builder

	switch cfg.runtime() {
	case "frankenphp":
		fmt.Fprintf(&b, "FROM dunglas/frankenphp:1-php%s-bookworm\n\n", cfg.PHPVersion)
	default:
		fmt.Fprintf(&b, "FROM php:%s-fpm-bookworm\n\n", cfg.PHPVersion)
	}

	// Install runtime libs permanently in their own apt update; keeping them
	// out of the build-deps install means they survive the apt-get purge step
	// and stay available to the running container.
	allRuntimeDeps := append(baseRuntimeDeps, runtimeDeps...)
	fmt.Fprintf(&b, "RUN apt-get update \\\n")
	fmt.Fprintf(&b, "    && apt-get install -y --no-install-recommends %s \\\n",
		strings.Join(allRuntimeDeps, " "))
	fmt.Fprintf(&b, "    && rm -rf /var/lib/apt/lists/*\n\n")

	// Install build deps, compile extensions, then purge build deps + apt cache.
	allBuildDeps := append(baseBuildDeps, buildDeps...)
	fmt.Fprintf(&b, "RUN apt-get update \\\n")
	fmt.Fprintf(&b, "    && apt-get install -y --no-install-recommends %s",
		strings.Join(allBuildDeps, " "))
	fmt.Fprintf(&b, " \\\n")

	allNative := baseExtensions
	if len(native) > 0 {
		allNative += " " + strings.Join(native, " ")
	}

	if hasGD(native) {
		fmt.Fprintf(&b, "    && docker-php-ext-configure gd --with-freetype --with-jpeg \\\n")
	}

	fmt.Fprintf(&b, "    && docker-php-ext-install %s \\\n", allNative)
	fmt.Fprintf(&b, "    && pecl install redis xdebug \\\n")
	fmt.Fprintf(&b, "    && docker-php-ext-enable redis \\\n")

	if len(pecl) > 0 {
		fmt.Fprintf(&b, "    && pecl install %s \\\n", strings.Join(pecl, " "))
		fmt.Fprintf(&b, "    && docker-php-ext-enable %s \\\n", strings.Join(pecl, " "))
	}

	fmt.Fprintf(&b, "    && apt-get purge -y --auto-remove %s \\\n",
		strings.Join(allBuildDeps, " "))
	fmt.Fprintf(&b, "    && rm -rf /var/lib/apt/lists/*\n\n")

	// Custom conf directory for host-mounted overrides (e.g., xdebug.ini)
	fmt.Fprintf(&b, "RUN mkdir -p /usr/local/etc/php/conf.custom\n")
	fmt.Fprintf(&b, "ENV PHP_INI_SCAN_DIR=/usr/local/etc/php/conf.d:/usr/local/etc/php/conf.custom\n\n")

	if cfg.runtime() == "fpm" {
		// Allow FPM to run as any UID by removing the user/group directives.
		// The actual UID is set via docker compose user: directive at runtime.
		fmt.Fprintf(&b, "RUN sed -i '/^user = /d; /^group = /d' /usr/local/etc/php-fpm.d/www.conf\n\n")
	}

	fmt.Fprintf(&b, "COPY --from=composer:latest /usr/bin/composer /usr/bin/composer\n")
	fmt.Fprintf(&b, "COPY php.ini /usr/local/etc/php/php.ini\n")
	fmt.Fprintf(&b, "COPY my.cnf /etc/my.cnf.d/dev.cnf\n")

	if cfg.runtime() == "frankenphp" {
		fmt.Fprintf(&b, "COPY Caddyfile /etc/caddy/Caddyfile\n")
	}

	fmt.Fprintf(&b, "\nWORKDIR /srv\n")

	return b.String()
}


func hasGD(exts []string) bool {
	for _, e := range exts {
		if e == "gd" {
			return true
		}
	}
	return false
}

func unionExtensions(lists ...[]string) []string {
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, ext := range list {
			seen[ext] = true
		}
	}

	result := make([]string, 0, len(seen))
	for ext := range seen {
		result = append(result, ext)
	}
	sort.Strings(result)
	return result
}

func imageHash(cfg ImageConfig) string {
	// Hash the full Dockerfile content so the tag changes
	// whenever the image definition changes (extensions, base
	// packages, composer, etc.)
	content := generateDockerfile(cfg)
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:4])
}
