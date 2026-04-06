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

// baseBuildDeps are Alpine packages needed to compile the base extensions.
var baseBuildDeps = []string{"postgresql-dev"}

// baseRuntimeDeps are Alpine packages needed at runtime by base extensions.
var baseRuntimeDeps = []string{"bash", "libpq", "mysql-client", "postgresql-client"}

// nativeExtDeps maps extensions to their Alpine build-time (-dev) packages.
var nativeExtDeps = map[string][]string{
	"gd":   {"libpng-dev", "libjpeg-turbo-dev", "freetype-dev"},
	"zip":  {"libzip-dev"},
	"intl": {"icu-dev"},
	"exif": {},
}

// nativeExtRuntime maps extensions to their Alpine runtime packages
// (must remain after build deps are removed).
var nativeExtRuntime = map[string][]string{
	"gd":   {"libpng", "libjpeg-turbo", "freetype"},
	"zip":  {"libzip"},
	"intl": {"icu-libs"},
	"exif": {},
}

// ImageConfig holds everything needed to build a PHP image.
type ImageConfig struct {
	PHPVersion string
	Extensions []string
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
	return fmt.Sprintf("dev-php:%s-%s", cfg.PHPVersion, hash)
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

	return dir, nil
}

func generateDockerfile(cfg ImageConfig) string {
	var native, pecl []string
	var buildDeps, runtimeDeps []string

	for _, ext := range cfg.Extensions {
		if deps, ok := nativeExtDeps[ext]; ok {
			native = append(native, ext)
			buildDeps = append(buildDeps, deps...)
			runtimeDeps = append(runtimeDeps, nativeExtRuntime[ext]...)
		} else {
			pecl = append(pecl, ext)
		}
	}

	var b strings.Builder

	fmt.Fprintf(&b, "FROM php:%s-fpm-alpine\n\n", cfg.PHPVersion)

	// Install runtime libs permanently (survive the del step)
	allRuntimeDeps := append(baseRuntimeDeps, runtimeDeps...)
	fmt.Fprintf(&b, "RUN apk add --no-cache %s\n\n",
		strings.Join(allRuntimeDeps, " "))

	// Install build deps, compile extensions, then remove build deps
	allBuildDeps := append(baseBuildDeps, buildDeps...)
	fmt.Fprintf(&b, "RUN apk add --no-cache --virtual .build-deps linux-headers $PHPIZE_DEPS %s",
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

	fmt.Fprintf(&b, "    && apk del .build-deps\n\n")

	// Custom conf directory for host-mounted overrides (e.g., xdebug.ini)
	fmt.Fprintf(&b, "RUN mkdir -p /usr/local/etc/php/conf.custom\n")
	fmt.Fprintf(&b, "ENV PHP_INI_SCAN_DIR=/usr/local/etc/php/conf.d:/usr/local/etc/php/conf.custom\n\n")

	// Allow FPM to run as any UID by removing the user/group directives.
	// The actual UID is set via docker compose user: directive at runtime.
	fmt.Fprintf(&b, "RUN sed -i '/^user = /d; /^group = /d' /usr/local/etc/php-fpm.d/www.conf\n\n")

	fmt.Fprintf(&b, "COPY php.ini /usr/local/etc/php/php.ini\n")
	fmt.Fprintf(&b, "COPY my.cnf /etc/my.cnf.d/dev.cnf\n\n")
	fmt.Fprintf(&b, "WORKDIR /srv\n")

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
	sorted := make([]string, len(cfg.Extensions))
	copy(sorted, cfg.Extensions)
	sort.Strings(sorted)

	h := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return fmt.Sprintf("%x", h[:4])
}
