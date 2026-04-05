# Docker-Based Stack Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor `dev` CLI from host-native PHP/Caddy to a fully Docker-based stack where the only host dependency is Docker.

**Architecture:** All services (Caddy, PHP-FPM, MySQL, Redis, etc.) run as Docker containers managed by a single generated `docker-compose.yml`. The CLI writes Caddy configs to a mounted volume, builds PHP images with project extensions, and manages `/etc/hosts` entries. The `db` package is unchanged.

**Tech Stack:** Go 1.25+, Docker Compose, Caddy 2 (Alpine), PHP-FPM (Alpine), testcontainers-go (tests)

**Spec:** `docs/superpowers/specs/2026-04-05-docker-refactor-design.md`

---

## File Structure

### New Files
- `internal/config/global.go` — global `~/.dev/config.yaml` loading (projects_dir, php_versions)
- `internal/config/global_test.go` — tests for global config
- `internal/hosts/hosts.go` — `/etc/hosts` management + WSL2 detection
- `internal/hosts/hosts_test.go` — tests for hosts management
- `internal/phpimage/phpimage.go` — Dockerfile generation, extension union, image building
- `internal/phpimage/phpimage_test.go` — tests for Dockerfile generation
- `cmd/trust.go` — `dev trust` command
- `cmd/build.go` — `dev build` command

### Modified Files
- `internal/config/config.go` — add `Extensions` field to `ProjectConfig`
- `internal/config/config_test.go` — test extensions field
- `internal/caddy/caddy.go` — rewrite: configs to mounted volume, reload via docker exec
- `internal/docker/docker.go` — rewrite: dynamic compose generation, `Exec()` method
- `internal/docker/service.go` — update `Service` adapter for new interface
- `internal/caddy/service.go` — update `Service` adapter for new interface
- `internal/lifecycle/lifecycle.go` — updated interfaces, new `HostsService`, hooks via docker exec
- `internal/lifecycle/lifecycle_test.go` — updated tests for new interfaces
- `cmd/start.go` — wire new lifecycle with hosts + phpimage
- `cmd/stop.go` — remove killProjectProcesses
- `cmd/restart.go` — wire new lifecycle
- `cmd/down.go` — simplified
- `cmd/artisan.go` — use docker exec
- `cmd/composer.go` — use docker exec
- `cmd/xdebug.go` — write ini + docker exec reload
- `cmd/info.go` — add extensions info

### Removed Files
- `internal/php/php.go` — replaced by containerized PHP
- `internal/php/php_test.go` — no longer needed

---

## Task 1: Add `Extensions` field to ProjectConfig

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for extensions parsing**

In `internal/config/config_test.go`, add:

```go
func TestLoad_ParsesExtensions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
php: "8.2"
extensions:
  - imagick
  - swoole
`), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Extensions) != 2 {
		t.Fatalf("Extensions = %v, want 2 items", cfg.Extensions)
	}
	if cfg.Extensions[0] != "imagick" || cfg.Extensions[1] != "swoole" {
		t.Errorf("Extensions = %v, want [imagick swoole]", cfg.Extensions)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_ParsesExtensions -v`
Expected: FAIL — `cfg.Extensions` field does not exist

- [ ] **Step 3: Add Extensions field to ProjectConfig**

In `internal/config/config.go`, add the field to the struct:

```go
type ProjectConfig struct {
	Type       string                       `yaml:"type"`
	PHP        string                       `yaml:"php"`
	Node       string                       `yaml:"node"`
	DBDriver   string                       `yaml:"db_driver"`
	DB         string                       `yaml:"db"`
	Extensions []string                     `yaml:"extensions"`
	MySQL      MySQLConfig                  `yaml:"mysql"`
	Postgres   PostgresConfig               `yaml:"postgres"`
	Hooks      Hooks                        `yaml:"hooks"`
	Services   map[string]ServiceDefinition `yaml:"services"`
}
```

No default-filling needed — empty slice is the correct default.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_ParsesExtensions -v`
Expected: PASS

- [ ] **Step 5: Run all tests to verify no regressions**

Run: `go test ./... -count=1`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add extensions field to ProjectConfig"
```

---

## Task 2: Add global config (`~/.dev/config.yaml`)

**Files:**
- Create: `internal/config/global.go`
- Create: `internal/config/global_test.go`

- [ ] **Step 1: Write failing tests for global config**

Create `internal/config/global_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobal_DefaultsWhenNoFile(t *testing.T) {
	dir := t.TempDir()

	cfg, err := loadGlobal(dir)
	if err != nil {
		t.Fatalf("loadGlobal() error = %v", err)
	}

	home, _ := os.UserHomeDir()
	wantDir := filepath.Join(home, "Projects")
	if cfg.ProjectsDir != wantDir {
		t.Errorf("ProjectsDir = %q, want %q", cfg.ProjectsDir, wantDir)
	}
	if len(cfg.PHPVersions) != 0 {
		t.Errorf("PHPVersions = %v, want empty", cfg.PHPVersions)
	}
}

func TestLoadGlobal_ParsesConfigFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`
projects_dir: /home/dev/code
php_versions:
  - "8.2"
  - "8.3"
`), 0644)

	cfg, err := loadGlobal(dir)
	if err != nil {
		t.Fatalf("loadGlobal() error = %v", err)
	}

	if cfg.ProjectsDir != "/home/dev/code" {
		t.Errorf("ProjectsDir = %q, want %q", cfg.ProjectsDir, "/home/dev/code")
	}
	if len(cfg.PHPVersions) != 2 {
		t.Errorf("PHPVersions = %v, want 2 items", cfg.PHPVersions)
	}
}

func TestLoadGlobal_ExpandsTilde(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`
projects_dir: ~/MyProjects
`), 0644)

	cfg, err := loadGlobal(dir)
	if err != nil {
		t.Fatalf("loadGlobal() error = %v", err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "MyProjects")
	if cfg.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q", cfg.ProjectsDir, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoadGlobal -v`
Expected: FAIL — `loadGlobal` does not exist

- [ ] **Step 3: Implement global config**

Create `internal/config/global.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const GlobalConfigFile = "config.yaml"

// GlobalConfig holds settings from ~/.dev/config.yaml.
type GlobalConfig struct {
	ProjectsDir string   `yaml:"projects_dir"`
	PHPVersions []string `yaml:"php_versions"`
}

// LoadGlobal reads the global config from ~/.dev/config.yaml.
func LoadGlobal() (*GlobalConfig, error) {
	return loadGlobal(GlobalDir())
}

// loadGlobal is the testable core that reads from a given directory.
func loadGlobal(devDir string) (*GlobalConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	cfg := &GlobalConfig{
		ProjectsDir: filepath.Join(home, "Projects"),
	}

	path := filepath.Join(devDir, GlobalConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Expand ~ in projects_dir
	if strings.HasPrefix(cfg.ProjectsDir, "~/") {
		cfg.ProjectsDir = filepath.Join(home, cfg.ProjectsDir[2:])
	}

	if cfg.ProjectsDir == "" {
		cfg.ProjectsDir = filepath.Join(home, "Projects")
	}

	return cfg, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestLoadGlobal -v`
Expected: All 3 PASS

- [ ] **Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/config/global.go internal/config/global_test.go
git commit -m "feat: add global config loading from ~/.dev/config.yaml"
```

---

## Task 3: Create `internal/hosts` package

**Files:**
- Create: `internal/hosts/hosts.go`
- Create: `internal/hosts/hosts_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/hosts/hosts_test.go`:

```go
package hosts

import (
	"os"
	"strings"
	"testing"
)

func TestEnsure_AddsEntry(t *testing.T) {
	f := t.TempDir() + "/hosts"
	os.WriteFile(f, []byte("127.0.0.1 localhost\n"), 0644)

	err := ensureEntry(f, "myproject.test")
	if err != nil {
		t.Fatalf("ensureEntry() error = %v", err)
	}

	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "127.0.0.1 myproject.test") {
		t.Errorf("hosts file missing entry:\n%s", data)
	}
}

func TestEnsure_SkipsExistingEntry(t *testing.T) {
	f := t.TempDir() + "/hosts"
	os.WriteFile(f, []byte("127.0.0.1 localhost\n127.0.0.1 myproject.test\n"), 0644)

	err := ensureEntry(f, "myproject.test")
	if err != nil {
		t.Fatalf("ensureEntry() error = %v", err)
	}

	data, _ := os.ReadFile(f)
	count := strings.Count(string(data), "myproject.test")
	if count != 1 {
		t.Errorf("entry appears %d times, want 1:\n%s", count, data)
	}
}

func TestIsWSL2(t *testing.T) {
	// Just verify it doesn't panic — actual result depends on environment
	_ = isWSL2()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hosts/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement hosts package**

Create `internal/hosts/hosts.go`:

```go
package hosts

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const hostsFile = "/etc/hosts"

// Ensure adds a 127.0.0.1 entry for the domain to /etc/hosts if missing.
// Uses sudo for the write. On WSL2, also writes to the Windows hosts file.
func Ensure(domain string) error {
	if err := ensureWithSudo(hostsFile, domain); err != nil {
		return err
	}

	if isWSL2() {
		winHosts := "/mnt/c/Windows/System32/drivers/etc/hosts"
		if err := ensureWithSudo(winHosts, domain); err != nil {
			return fmt.Errorf("windows hosts: %w", err)
		}
	}

	return nil
}

// ensureWithSudo adds the entry via sudo tee -a.
func ensureWithSudo(path, domain string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	entry := fmt.Sprintf("127.0.0.1 %s", domain)
	if strings.Contains(string(data), entry) {
		return nil
	}

	cmd := exec.Command("sudo", "sh", "-c",
		fmt.Sprintf("echo '%s' >> %s", entry, path))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("adding %s to %s: %w", domain, path, err)
	}

	return nil
}

// ensureEntry is the testable core that writes directly (no sudo).
func ensureEntry(path, domain string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	entry := fmt.Sprintf("127.0.0.1 %s", domain)
	if strings.Contains(string(data), entry) {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", entry); err != nil {
		return fmt.Errorf("writing to %s: %w", path, err)
	}

	return nil
}

// isWSL2 detects if running under WSL2.
func isWSL2() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hosts/ -v`
Expected: All 3 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hosts/hosts.go internal/hosts/hosts_test.go
git commit -m "feat: add hosts package for /etc/hosts management"
```

---

## Task 4: Create `internal/phpimage` package

**Files:**
- Create: `internal/phpimage/phpimage.go`
- Create: `internal/phpimage/phpimage_test.go`

- [ ] **Step 1: Write failing tests for Dockerfile generation**

Create `internal/phpimage/phpimage_test.go`:

```go
package phpimage

import (
	"strings"
	"testing"
)

func TestGenerateDockerfile_BaseExtensionsOnly(t *testing.T) {
	got := generateDockerfile("8.2", nil)

	if !strings.Contains(got, "FROM php:8.2-fpm-alpine") {
		t.Error("missing base image")
	}
	if !strings.Contains(got, "pdo_mysql") {
		t.Error("missing base extension pdo_mysql")
	}
	if strings.Contains(got, "pecl install imagick") {
		t.Error("should not contain extra extensions")
	}
}

func TestGenerateDockerfile_WithExtraExtensions(t *testing.T) {
	got := generateDockerfile("8.3", []string{"imagick", "swoole"})

	if !strings.Contains(got, "FROM php:8.3-fpm-alpine") {
		t.Error("missing base image")
	}
	if !strings.Contains(got, "pecl install imagick swoole") {
		t.Errorf("missing extra extensions in:\n%s", got)
	}
}

func TestUnionExtensions(t *testing.T) {
	projects := [][]string{
		{"imagick", "swoole"},
		{"swoole", "mongodb"},
		nil,
	}

	got := unionExtensions(projects...)
	want := []string{"imagick", "mongodb", "swoole"}

	if len(got) != len(want) {
		t.Fatalf("unionExtensions() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unionExtensions()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtensionHash(t *testing.T) {
	h1 := extensionHash([]string{"imagick", "swoole"})
	h2 := extensionHash([]string{"swoole", "imagick"})
	h3 := extensionHash([]string{"imagick"})

	if h1 != h2 {
		t.Error("hash should be order-independent")
	}
	if h1 == h3 {
		t.Error("different extensions should produce different hashes")
	}
	if len(h1) != 8 {
		t.Errorf("hash length = %d, want 8", len(h1))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/phpimage/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement phpimage package**

Create `internal/phpimage/phpimage.go`:

```go
package phpimage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

const baseExtensions = "pdo_mysql pdo_pgsql opcache pcntl bcmath"

// EnsureBuilt builds the PHP-FPM image for a version if the extension
// set has changed since the last build.
func EnsureBuilt(version string, extensions []string) error {
	tag := ImageTag(version, extensions)

	// Check if image already exists
	cmd := exec.Command("docker", "image", "inspect", tag)
	if cmd.Run() == nil {
		return nil // already built
	}

	dir, err := writeDockerfile(version, extensions)
	if err != nil {
		return err
	}

	fmt.Printf("  → Building PHP %s image...\n", version)
	build := exec.Command("docker", "build", "-t", tag, dir)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building php %s image: %w", version, err)
	}

	return nil
}

// ImageTag returns the Docker image tag for a PHP version + extensions.
func ImageTag(version string, extensions []string) string {
	hash := extensionHash(extensions)
	return fmt.Sprintf("dev-php:%s-%s", version, hash)
}

// writeDockerfile writes the Dockerfile and php.ini to a temp directory.
func writeDockerfile(version string, extensions []string) (string, error) {
	dir := filepath.Join(config.GlobalDir(), "dockerfiles", "php", version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating dockerfile dir: %w", err)
	}

	content := generateDockerfile(version, extensions)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing Dockerfile: %w", err)
	}

	phpIni := "[PHP]\nmemory_limit = 512M\nupload_max_filesize = 100M\npost_max_size = 100M\n"
	if err := os.WriteFile(filepath.Join(dir, "php.ini"), []byte(phpIni), 0644); err != nil {
		return "", fmt.Errorf("writing php.ini: %w", err)
	}

	return dir, nil
}

// generateDockerfile produces the Dockerfile content for a PHP version.
func generateDockerfile(version string, extensions []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "FROM php:%s-fpm-alpine\n\n", version)
	fmt.Fprintf(&b, "RUN apk add --no-cache linux-headers $PHPIZE_DEPS \\\n")
	fmt.Fprintf(&b, "    && docker-php-ext-install %s \\\n", baseExtensions)
	fmt.Fprintf(&b, "    && pecl install redis xdebug \\\n")
	fmt.Fprintf(&b, "    && docker-php-ext-enable redis \\\n")

	if len(extensions) > 0 {
		fmt.Fprintf(&b, "    && pecl install %s \\\n", strings.Join(extensions, " "))
		fmt.Fprintf(&b, "    && docker-php-ext-enable %s \\\n", strings.Join(extensions, " "))
	}

	fmt.Fprintf(&b, "    && apk del $PHPIZE_DEPS\n\n")
	fmt.Fprintf(&b, "COPY php.ini /usr/local/etc/php/php.ini\n\n")
	fmt.Fprintf(&b, "WORKDIR /srv\n")

	return b.String()
}

// unionExtensions merges multiple extension lists, deduplicated and sorted.
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

// extensionHash returns a short hash of the sorted extension list.
func extensionHash(extensions []string) string {
	sorted := make([]string, len(extensions))
	copy(sorted, extensions)
	sort.Strings(sorted)

	h := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return fmt.Sprintf("%x", h[:4])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/phpimage/ -v`
Expected: All 4 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/phpimage/phpimage.go internal/phpimage/phpimage_test.go
git commit -m "feat: add phpimage package for Dockerfile generation and builds"
```

---

## Task 5: Rewrite `internal/docker` for compose generation

**Files:**
- Rewrite: `internal/docker/docker.go`
- Modify: `internal/docker/service.go`
- Create: `internal/docker/docker_test.go`

- [ ] **Step 1: Write failing tests for compose generation**

Create `internal/docker/docker_test.go`:

```go
package docker

import (
	"strings"
	"testing"
)

func TestGenerateCompose_IncludesCaddy(t *testing.T) {
	got := generateCompose("/home/user/Projects", []string{"8.2"})

	if !strings.Contains(got, "caddy:") {
		t.Error("missing caddy service")
	}
	if !strings.Contains(got, "caddy:2-alpine") {
		t.Error("missing caddy image")
	}
}

func TestGenerateCompose_IncludesPHPVersions(t *testing.T) {
	got := generateCompose("/home/user/Projects", []string{"8.2", "8.3"})

	if !strings.Contains(got, "php82:") {
		t.Error("missing php82 service")
	}
	if !strings.Contains(got, "php83:") {
		t.Error("missing php83 service")
	}
}

func TestGenerateCompose_MountsProjectsDir(t *testing.T) {
	got := generateCompose("/home/user/Code", []string{"8.2"})

	if !strings.Contains(got, "/home/user/Code:/srv") {
		t.Error("missing projects dir mount")
	}
}

func TestGenerateCompose_IncludesDBServices(t *testing.T) {
	got := generateCompose("/home/user/Projects", []string{"8.2"})

	if !strings.Contains(got, "mysql:") {
		t.Error("missing mysql service")
	}
	if !strings.Contains(got, "redis:") {
		t.Error("missing redis service")
	}
}

func TestPHPServiceName(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"8.2", "php82"},
		{"8.3", "php83"},
		{"7.4", "php74"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := PHPServiceName(tt.version)
			if got != tt.want {
				t.Errorf("PHPServiceName(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/docker/ -v`
Expected: FAIL — `generateCompose` and `PHPServiceName` do not exist

- [ ] **Step 3: Rewrite docker.go**

Replace `internal/docker/docker.go` with:

```go
package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/phpimage"
)

// ComposeFile returns the path to the generated docker-compose.yml.
func ComposeFile() string {
	return filepath.Join(config.GlobalDir(), "docker-compose.yml")
}

// Up generates the compose file and starts all services.
func Up(projectsDir string, phpVersions []string) error {
	content := generateCompose(projectsDir, phpVersions)
	if err := os.WriteFile(ComposeFile(), []byte(content), 0644); err != nil {
		return fmt.Errorf("writing compose file: %w", err)
	}
	return compose("up", "-d")
}

// Down stops all services.
func Down() error {
	return compose("down")
}

// Exec runs a command inside a container.
func Exec(service string, workdir string, args ...string) error {
	execArgs := []string{"compose", "-f", ComposeFile(),
		"exec", "-w", workdir, service}
	execArgs = append(execArgs, args...)
	cmd := exec.Command("docker", execArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker exec %s: %w", service, err)
	}
	return nil
}

// PHPServiceName converts a PHP version to a compose service name.
func PHPServiceName(version string) string {
	return "php" + strings.ReplaceAll(version, ".", "")
}

func compose(args ...string) error {
	fullArgs := append([]string{"compose", "-f", ComposeFile()}, args...)
	cmd := exec.Command("docker", fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w", args[0], err)
	}
	return nil
}

// generateCompose produces the docker-compose.yml content.
func generateCompose(projectsDir string, phpVersions []string) string {
	var b strings.Builder

	b.WriteString("services:\n")

	// Caddy
	b.WriteString("  caddy:\n")
	b.WriteString("    image: caddy:2-alpine\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"80:80\"\n")
	b.WriteString("      - \"443:443\"\n")
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s/caddy:/etc/caddy\n", config.GlobalDir())
	fmt.Fprintf(&b, "      - %s/caddy/data:/data\n", config.GlobalDir())
	fmt.Fprintf(&b, "      - %s:/srv\n", projectsDir)
	b.WriteString("    networks: [dev]\n\n")

	// PHP services
	for _, ver := range phpVersions {
		name := PHPServiceName(ver)
		tag := phpimage.ImageTag(ver, nil)
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", tag)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/srv\n", projectsDir)
		fmt.Fprintf(&b, "      - %s/php/%s/conf.d:/usr/local/etc/php/conf.d\n",
			config.GlobalDir(), ver)
		b.WriteString("    networks: [dev]\n\n")
	}

	// MySQL
	b.WriteString("  mysql:\n")
	b.WriteString("    image: mysql:8.0\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"3306:3306\"\n")
	b.WriteString("    environment:\n")
	b.WriteString("      MYSQL_ROOT_PASSWORD: root\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - mysql_data:/var/lib/mysql\n")
	b.WriteString("    networks: [dev]\n\n")

	// Redis
	b.WriteString("  redis:\n")
	b.WriteString("    image: redis:8\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"6379:6379\"\n")
	b.WriteString("    command: [\"redis-server\", \"--appendonly\", \"yes\"]\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - redis_data:/data\n")
	b.WriteString("    networks: [dev]\n\n")

	// Typesense
	b.WriteString("  typesense:\n")
	b.WriteString("    image: typesense/typesense:26.0\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"8108:8108\"\n")
	b.WriteString("    environment:\n")
	b.WriteString("      TYPESENSE_API_KEY: dev\n")
	b.WriteString("    command: \"--data-dir /data --enable-cors\"\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - typesense_data:/data\n")
	b.WriteString("    networks: [dev]\n\n")

	// Postgres
	b.WriteString("  postgres:\n")
	b.WriteString("    image: postgres:15\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"5432:5432\"\n")
	b.WriteString("    environment:\n")
	b.WriteString("      POSTGRES_USER: postgres\n")
	b.WriteString("      POSTGRES_PASSWORD: postgres\n")
	b.WriteString("      POSTGRES_DB: postgres\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - postgres_data:/var/lib/postgresql/data\n")
	b.WriteString("    networks: [dev]\n\n")

	// Volumes
	b.WriteString("volumes:\n")
	b.WriteString("  mysql_data:\n")
	b.WriteString("  redis_data:\n")
	b.WriteString("  typesense_data:\n")
	b.WriteString("  postgres_data:\n\n")

	// Networks
	b.WriteString("networks:\n")
	b.WriteString("  dev:\n")

	return b.String()
}
```

- [ ] **Step 4: Update `internal/docker/service.go`**

Replace with:

```go
package docker

// Service wraps the docker package functions into a struct
// that satisfies lifecycle.DockerService.
type Service struct {
	ProjectsDir string
}

func (s Service) Up(phpVersions []string) error {
	return Up(s.ProjectsDir, phpVersions)
}

func (s Service) Down() error { return Down() }

func (s Service) Exec(service string, workdir string, args ...string) error {
	return Exec(service, workdir, args...)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/docker/ -v`
Expected: All 5 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/docker/docker.go internal/docker/service.go internal/docker/docker_test.go
git commit -m "feat: rewrite docker package for dynamic compose generation"
```

---

## Task 6: Rewrite `internal/caddy` for Docker-based reload

**Files:**
- Rewrite: `internal/caddy/caddy.go`
- Modify: `internal/caddy/service.go`
- Create: `internal/caddy/caddy_test.go`

- [ ] **Step 1: Write failing tests for site config generation**

Create `internal/caddy/caddy_test.go`:

```go
package caddy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSiteConfig(t *testing.T) {
	got := generateSiteConfig("myproject", "/srv/myproject/public", "php82")

	if !strings.Contains(got, "myproject.test {") {
		t.Error("missing site domain")
	}
	if !strings.Contains(got, "/srv/myproject/public") {
		t.Error("missing docroot")
	}
	if !strings.Contains(got, "php_fastcgi php82:9000") {
		t.Error("missing php_fastcgi directive")
	}
}

func TestGenerateMainCaddyfile(t *testing.T) {
	got := generateMainCaddyfile()

	if !strings.Contains(got, "local_certs") {
		t.Error("missing local_certs directive")
	}
	if !strings.Contains(got, "import /etc/caddy/sites/*.caddy") {
		t.Error("missing import directive")
	}
}

func TestWriteSiteConfig(t *testing.T) {
	dir := t.TempDir()
	sitesDir := filepath.Join(dir, "sites")
	os.MkdirAll(sitesDir, 0755)

	err := writeSiteConfig(dir, "myproject", "/srv/myproject/public", "php82")
	if err != nil {
		t.Fatalf("writeSiteConfig() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(sitesDir, "myproject.caddy"))
	if err != nil {
		t.Fatalf("reading site config: %v", err)
	}
	if !strings.Contains(string(data), "myproject.test") {
		t.Error("site config missing domain")
	}
}

func TestRemoveSiteConfig(t *testing.T) {
	dir := t.TempDir()
	sitesDir := filepath.Join(dir, "sites")
	os.MkdirAll(sitesDir, 0755)

	path := filepath.Join(sitesDir, "myproject.caddy")
	os.WriteFile(path, []byte("test"), 0644)

	removeSiteConfig(dir, "myproject")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("site config should be removed")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/caddy/ -v`
Expected: FAIL — functions don't exist yet

- [ ] **Step 3: Rewrite caddy.go**

Replace `internal/caddy/caddy.go` with:

```go
package caddy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

// Link creates a Caddy site config and reloads.
func Link(siteName, docroot, phpService string) error {
	caddyDir := filepath.Join(config.GlobalDir(), "caddy")
	if err := writeSiteConfig(caddyDir, siteName, docroot, phpService); err != nil {
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

func writeSiteConfig(caddyDir, siteName, docroot, phpService string) error {
	sitesDir := filepath.Join(caddyDir, "sites")
	if err := os.MkdirAll(sitesDir, 0755); err != nil {
		return fmt.Errorf("creating sites dir: %w", err)
	}

	content := generateSiteConfig(siteName, docroot, phpService)
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

func generateSiteConfig(siteName, docroot, phpService string) string {
	return fmt.Sprintf(`%s.test {
	root * %s
	php_fastcgi %s:9000
	file_server
	encode gzip
}
`, siteName, docroot, phpService)
}

func generateMainCaddyfile() string {
	return "{\n\tlocal_certs\n}\n\nimport /etc/caddy/sites/*.caddy\n"
}
```

- [ ] **Step 4: Update `internal/caddy/service.go`**

Replace with:

```go
package caddy

// Service wraps the caddy package functions into a struct
// that satisfies lifecycle.CaddyService.
type Service struct{}

func (Service) Start() error                                           { return nil } // caddy starts via docker compose up
func (Service) Stop() error                                            { return nil } // caddy stops via docker compose down
func (Service) Link(siteName, docroot, phpService string) error        { return Link(siteName, docroot, phpService) }
func (Service) Unlink(siteName string) error                           { return Unlink(siteName) }
func (Service) Reload() error                                          { return Reload() }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/caddy/ -v`
Expected: All 4 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/caddy/caddy.go internal/caddy/service.go internal/caddy/caddy_test.go
git commit -m "feat: rewrite caddy package for Docker-based config and reload"
```

---

## Task 7: Rewrite `internal/lifecycle` for new interfaces

**Files:**
- Rewrite: `internal/lifecycle/lifecycle.go`
- Rewrite: `internal/lifecycle/lifecycle_test.go`

- [ ] **Step 1: Write failing tests for new lifecycle**

Replace `internal/lifecycle/lifecycle_test.go` with tests that use the new interfaces (DockerService with `Up(phpVersions)`, `Exec()`, new `HostsService`). Key test cases:

- `TestStart_CallsServicesInOrder` — Docker.Up, Caddy.Link, Hosts.Ensure, DB create
- `TestStart_StopsOnDockerError` — Docker.Up fails, nothing else called
- `TestStop_CallsUnlink` — hooks run via Docker.Exec, then Caddy.Unlink
- `TestDown_StopsDocker` — Docker.Down called

The mock structs change:

```go
type mockDocker struct {
	upCalled     bool
	upVersions   []string
	downCalled   bool
	execCalls    [][]string
	upErr, downErr, execErr error
}

func (m *mockDocker) Up(versions []string) error {
	m.upCalled = true
	m.upVersions = versions
	return m.upErr
}
func (m *mockDocker) Down() error { m.downCalled = true; return m.downErr }
func (m *mockDocker) Exec(service, workdir string, args ...string) error {
	m.execCalls = append(m.execCalls, append([]string{service, workdir}, args...))
	return m.execErr
}

type mockHosts struct {
	ensureCalled bool
	ensureDomain string
	ensureErr    error
}

func (m *mockHosts) Ensure(domain string) error {
	m.ensureCalled = true
	m.ensureDomain = domain
	return m.ensureErr
}
```

See `internal/lifecycle/lifecycle_test.go` in Task 7 Step 3 below for the full test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/lifecycle/ -v`
Expected: FAIL — interfaces don't match

- [ ] **Step 3: Rewrite lifecycle.go**

Replace `internal/lifecycle/lifecycle.go` with:

```go
package lifecycle

import (
	"fmt"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/db"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

// DockerService manages Docker containers.
type DockerService interface {
	Up(phpVersions []string) error
	Down() error
	Exec(service string, workdir string, args ...string) error
}

// CaddyService manages the Caddy reverse proxy.
type CaddyService interface {
	Start() error
	Stop() error
	Link(siteName, docroot, phpService string) error
	Unlink(siteName string) error
	Reload() error
}

// HostsService manages /etc/hosts entries.
type HostsService interface {
	Ensure(domain string) error
}

// Lifecycle orchestrates starting and stopping a dev environment.
type Lifecycle struct {
	Docker     DockerService
	Caddy      CaddyService
	Hosts      HostsService
	PHPService func(version string) string // converts "8.2" to "php82"
	Docroot    func(p *project.Project) string
	Output     func(format string, a ...any)
}

func (l *Lifecycle) printf(format string, a ...any) {
	if l.Output != nil {
		l.Output(format, a...)
	} else {
		fmt.Printf(format, a...)
	}
}

// Start brings up the full dev environment for a project.
func (l *Lifecycle) Start(p *project.Project, phpVersions []string) error {
	l.printf("Starting %s...\n", p.Name)

	phpSvc := l.PHPService(p.Config.PHP)
	docroot := l.Docroot(p)

	l.printf("  → Starting services...\n")
	if err := l.Docker.Up(phpVersions); err != nil {
		return fmt.Errorf("starting services: %w", err)
	}

	l.printf("  → Linking site...\n")
	if err := l.Caddy.Link(p.Name, docroot, phpSvc); err != nil {
		return fmt.Errorf("linking site: %w", err)
	}

	l.printf("  → Adding hosts entry...\n")
	if err := l.Hosts.Ensure(p.SiteDomain()); err != nil {
		return fmt.Errorf("adding hosts entry: %w", err)
	}

	l.printf("  → Creating database %s (%s)...\n", p.Config.DB, p.Config.DBDriver)
	store, err := db.NewStore(p.Config.DBConfig())
	if err != nil {
		return err
	}
	if err := store.CreateIfNotExists(p.Config.DB); err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	for _, hook := range p.Config.Hooks.PostStart {
		l.printf("  → Running: %s\n", hook)
		if err := l.Docker.Exec(phpSvc, docroot, "bash", "-c", hook); err != nil {
			l.printf("  ! hook failed: %s\n", err)
		}
	}

	l.printf("\n✓ %s is running at https://%s\n", p.Name, p.SiteDomain())
	l.printf("  PHP:      %s\n", p.Config.PHP)
	l.printf("  Database: %s\n", p.Config.DB)

	return nil
}

// Stop tears down the dev environment for a project.
func (l *Lifecycle) Stop(p *project.Project) error {
	l.printf("Stopping %s...\n", p.Name)

	phpSvc := l.PHPService(p.Config.PHP)
	docroot := l.Docroot(p)

	for _, hook := range p.Config.Hooks.PostStop {
		l.printf("  → Running: %s\n", hook)
		if err := l.Docker.Exec(phpSvc, docroot, "bash", "-c", hook); err != nil {
			l.printf("  ! hook failed: %s\n", err)
		}
	}

	l.printf("  → Unlinking site...\n")
	if err := l.Caddy.Unlink(p.Name); err != nil {
		return fmt.Errorf("unlinking site: %w", err)
	}

	l.printf("✓ %s stopped\n", p.Name)
	return nil
}

// Down stops all shared services.
func (l *Lifecycle) Down() error {
	l.printf("Stopping everything...\n")

	l.printf("  → Stopping services...\n")
	if err := l.Docker.Down(); err != nil {
		return fmt.Errorf("stopping services: %w", err)
	}

	l.printf("✓ All services stopped\n")
	return nil
}
```

Write matching `lifecycle_test.go` with full mocks and tests for Start, Stop, Down, error propagation (same patterns as current tests but with new interfaces).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lifecycle/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/lifecycle/lifecycle.go internal/lifecycle/lifecycle_test.go
git commit -m "feat: rewrite lifecycle for Docker-based interfaces"
```

---

## Task 8: Rewrite command files

**Files:**
- Modify: `cmd/start.go`, `cmd/stop.go`, `cmd/restart.go`, `cmd/down.go`
- Modify: `cmd/artisan.go`, `cmd/composer.go`, `cmd/xdebug.go`
- Create: `cmd/trust.go`, `cmd/build.go`
- Remove: `internal/php/php.go`, `internal/php/php_test.go`

- [ ] **Step 1: Rewrite `cmd/start.go`**

```go
package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/caddy"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/docker"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/hosts"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/lifecycle"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/phpimage"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}

		if err := phpimage.EnsureBuilt(p.Config.PHP, p.Config.Extensions); err != nil {
			return err
		}

		phpVersions := append(global.PHPVersions, p.Config.PHP)

		lc := &lifecycle.Lifecycle{
			Docker:     docker.Service{ProjectsDir: global.ProjectsDir},
			Caddy:      caddy.Service{},
			Hosts:      hosts.Service{},
			PHPService: docker.PHPServiceName,
			Docroot: func(proj *project.Project) string {
				rel, _ := filepath.Rel(global.ProjectsDir, proj.Dir)
				return filepath.Join("/srv", rel, "public")
			},
		}
		return lc.Start(p, phpVersions)
	},
}
```

- [ ] **Step 2: Rewrite `cmd/stop.go`**

Same pattern — thin wrapper calling `lc.Stop(p)`. No more `killProjectProcesses`.

- [ ] **Step 3: Rewrite `cmd/restart.go`**

Calls `lc.Stop(p)` then `lc.Start(p, phpVersions)`.

- [ ] **Step 4: Rewrite `cmd/down.go`**

Calls `lc.Down()`.

- [ ] **Step 5: Rewrite `cmd/artisan.go`**

Use `docker.Exec(phpService, workdir, "php", "artisan", args...)`.

- [ ] **Step 6: Rewrite `cmd/composer.go`**

Use `docker.Exec(phpService, workdir, "composer", args...)`.

- [ ] **Step 7: Rewrite `cmd/xdebug.go`**

Write/remove `~/.dev/php/<version>/conf.d/xdebug.ini`, then `docker compose exec <phpService> kill -USR2 1`.

- [ ] **Step 8: Create `cmd/trust.go`**

New command that extracts Caddy's root CA from the container data volume and installs it in the host trust store. On WSL2, also installs in Windows.

- [ ] **Step 9: Create `cmd/build.go`**

New command that force-rebuilds PHP images for all configured versions.

- [ ] **Step 10: Remove `internal/php/`**

```bash
rm internal/php/php.go internal/php/php_test.go
rmdir internal/php
```

- [ ] **Step 11: Build and verify**

Run: `go build ./... && go vet ./...`
Expected: Clean build

- [ ] **Step 12: Run all tests**

Run: `go test ./... -count=1`
Expected: All pass

- [ ] **Step 13: Commit**

```bash
git add -A
git commit -m "feat: rewrite commands for Docker-based stack"
```

---

## Task 9: Create `hosts.Service` adapter and wire into lifecycle

**Files:**
- Create: `internal/hosts/service.go`

- [ ] **Step 1: Create hosts Service adapter**

Create `internal/hosts/service.go`:

```go
package hosts

// Service wraps the hosts package functions into a struct
// that satisfies lifecycle.HostsService.
type Service struct{}

func (Service) Ensure(domain string) error { return Ensure(domain) }
```

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: Clean build

- [ ] **Step 3: Commit**

```bash
git add internal/hosts/service.go
git commit -m "feat: add hosts.Service adapter for lifecycle interface"
```

---

## Task 10: Update README and CLAUDE.md

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update README**

- Update install section: just Docker + `go build` + `dev trust`
- Remove Ondrej PPA, Homebrew PHP, dnsmasq, mkcert references
- Add `dev trust` and `dev build` to commands table
- Update "How it works" diagram for containerized stack
- Update "Building from source" Go version to 1.25+

- [ ] **Step 2: Update CLAUDE.md**

- Update architecture section for new packages (`hosts`, `phpimage`)
- Remove `internal/php` from package list
- Update interfaces section
- Add note about dynamic compose generation

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: update README and CLAUDE.md for Docker-based stack"
```

---

## Task 11: Final verification

- [ ] **Step 1: Full build**

Run: `go build ./...`

- [ ] **Step 2: Vet**

Run: `go vet ./...`

- [ ] **Step 3: All unit tests**

Run: `go test ./... -count=1 -race`

- [ ] **Step 4: Fuzz tests**

Run: `go test ./internal/config/ -fuzz=FuzzDbNameFromDir -fuzztime=5s`
Run: `go test ./internal/db/ -fuzz=FuzzSanitizeDBName -fuzztime=5s`

- [ ] **Step 5: Manual smoke test**

```bash
go build -o dev .
./dev trust
cd ~/Projects/some-laravel-project
./dev start
# Verify: https://project-name.test loads
./dev stop
./dev down
```
