# PHP ini & MySQL cnf Overrides — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow per-project and global overrides for php.ini and my.cnf settings, with dev-optimized defaults, delivered at runtime via mounted config files.

**Architecture:** New `internal/config/defaults.go` holds dev-optimized default maps and a merge function. Config structs gain `PhpIni`/`MysqlCnf` fields. On `dev start`, merged settings are written to `~/.dev/php/{version}/conf.d/dev-overrides.ini` and `~/.dev/mysql/conf.d/dev-overrides.cnf`. Docker compose mounts the MySQL conf.d directory.

**Tech Stack:** Go, YAML config, Docker volume mounts

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/config/defaults.go` | Dev-optimized default maps for php.ini and my.cnf, merge function, INI/CNF writers |
| Create | `internal/config/defaults_test.go` | Tests for merge logic and file writers |
| Modify | `internal/config/config.go:49-67` | Add `PhpIni` and `MysqlCnf` fields to `ProjectConfig` |
| Modify | `internal/config/config_test.go` | Test that `PhpIni`/`MysqlCnf` parse from YAML |
| Modify | `internal/config/global.go:30-34` | Add `PhpIni` and `MysqlCnf` fields to `GlobalConfig` |
| Modify | `internal/config/global_test.go` | Test that global `PhpIni`/`MysqlCnf` parse from YAML |
| Modify | `internal/docker/docker.go:226-238` | Add MySQL conf.d volume mount |
| Modify | `internal/docker/docker_test.go` | Test MySQL conf.d mount appears in generated compose |
| Modify | `cmd/start.go:24-54` | Call settings writers before starting services |
| Modify | `cmd/services.go:26-54` | Call MySQL cnf writer before starting services |

---

### Task 1: Dev-optimized defaults and merge function

**Files:**
- Create: `internal/config/defaults.go`
- Create: `internal/config/defaults_test.go`

- [ ] **Step 1: Write failing tests for MergeSettings**

In `internal/config/defaults_test.go`:

```go
package config

import (
	"testing"
)

func TestMergeSettings_DefaultsOnly(t *testing.T) {
	t.Parallel()
	defaults := map[string]string{"memory_limit": "512M", "display_errors": "On"}

	got := MergeSettings(defaults, nil, nil)

	if got["memory_limit"] != "512M" {
		t.Errorf("memory_limit = %q, want %q", got["memory_limit"], "512M")
	}
	if got["display_errors"] != "On" {
		t.Errorf("display_errors = %q, want %q", got["display_errors"], "On")
	}
}

func TestMergeSettings_GlobalOverridesDefaults(t *testing.T) {
	t.Parallel()
	defaults := map[string]string{"memory_limit": "512M"}
	global := map[string]string{"memory_limit": "1G"}

	got := MergeSettings(defaults, global, nil)

	if got["memory_limit"] != "1G" {
		t.Errorf("memory_limit = %q, want %q", got["memory_limit"], "1G")
	}
}

func TestMergeSettings_ProjectOverridesGlobal(t *testing.T) {
	t.Parallel()
	defaults := map[string]string{"memory_limit": "512M"}
	global := map[string]string{"memory_limit": "1G"}
	project := map[string]string{"memory_limit": "2G"}

	got := MergeSettings(defaults, global, project)

	if got["memory_limit"] != "2G" {
		t.Errorf("memory_limit = %q, want %q", got["memory_limit"], "2G")
	}
}

func TestMergeSettings_AddsNewKeys(t *testing.T) {
	t.Parallel()
	defaults := map[string]string{"memory_limit": "512M"}
	global := map[string]string{"upload_max_filesize": "100M"}
	project := map[string]string{"max_execution_time": "600"}

	got := MergeSettings(defaults, global, project)

	if got["memory_limit"] != "512M" {
		t.Errorf("memory_limit = %q, want %q", got["memory_limit"], "512M")
	}
	if got["upload_max_filesize"] != "100M" {
		t.Errorf("upload_max_filesize = %q, want %q", got["upload_max_filesize"], "100M")
	}
	if got["max_execution_time"] != "600" {
		t.Errorf("max_execution_time = %q, want %q", got["max_execution_time"], "600")
	}
}

func TestMergeSettings_NilInputs(t *testing.T) {
	t.Parallel()
	got := MergeSettings(nil, nil, nil)

	if got == nil {
		t.Error("expected empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestMergeSettings -v`
Expected: FAIL — `MergeSettings` not defined

- [ ] **Step 3: Implement MergeSettings and default maps**

In `internal/config/defaults.go`:

```go
package config

// DefaultPhpIni holds dev-optimized PHP ini settings.
// These are written to a runtime override file, layered on top of the
// base php.ini baked into the Docker image.
var DefaultPhpIni = map[string]string{
	"error_reporting":              "E_ALL",
	"display_errors":               "On",
	"display_startup_errors":       "On",
	"log_errors":                   "On",
	"memory_limit":                 "512M",
	"max_execution_time":           "300",
	"max_input_time":               "300",
	"upload_max_filesize":          "256M",
	"post_max_size":                "256M",
	"max_input_vars":               "5000",
	"opcache.enable":               "1",
	"opcache.revalidate_freq":      "0",
	"opcache.validate_timestamps":  "1",
	"opcache.max_accelerated_files": "20000",
	"opcache.memory_consumption":   "256",
	"opcache.interned_strings_buffer": "32",
	"realpath_cache_size":          "4096k",
	"realpath_cache_ttl":           "600",
	"session.gc_maxlifetime":       "86400",
	"date.timezone":                "UTC",
	"zend.assertions":              "1",
}

// DefaultMysqlCnf holds dev-optimized MySQL server settings.
var DefaultMysqlCnf = map[string]string{
	"skip-log-bin":                    "",
	"innodb_flush_log_at_trx_commit":  "0",
	"innodb_flush_method":             "O_DIRECT",
	"innodb_buffer_pool_size":         "512M",
	"innodb_log_file_size":            "128M",
	"innodb_doublewrite":              "0",
	"innodb_io_capacity":              "2000",
	"innodb_io_capacity_max":          "4000",
	"max_connections":                 "200",
	"table_open_cache":                "4000",
	"performance_schema":              "OFF",
	"slow_query_log":                  "1",
	"long_query_time":                 "2",
	"host_cache_size":                 "0",
	"skip-name-resolve":               "",
}

// ProtectedMysqlCnf holds keys that are always forced and cannot be overridden.
var ProtectedMysqlCnf = map[string]string{
	"ssl": "0",
}

// MergeSettings merges settings maps in order: defaults → global → project.
// Later layers override earlier ones. Returns a new map.
func MergeSettings(defaults, global, project map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range global {
		merged[k] = v
	}
	for k, v := range project {
		merged[k] = v
	}
	return merged
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestMergeSettings -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/defaults.go internal/config/defaults_test.go
git commit -m "feat: add dev-optimized defaults and MergeSettings function"
```

---

### Task 2: INI and CNF file writers

**Files:**
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/defaults_test.go`

- [ ] **Step 1: Write failing tests for WritePhpIni and WriteMysqlCnf**

Append to `internal/config/defaults_test.go`:

```go
func TestWritePhpIni(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settings := map[string]string{
		"memory_limit":       "1G",
		"display_errors":     "On",
		"opcache.revalidate_freq": "0",
	}

	err := WritePhpIni(dir, "8.2", settings)
	if err != nil {
		t.Fatalf("WritePhpIni() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "php", "8.2", "conf.d", "dev-overrides.ini"))
	if err != nil {
		t.Fatalf("reading ini file: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "memory_limit = 1G") {
		t.Errorf("missing memory_limit setting in:\n%s", s)
	}
	if !strings.Contains(s, "display_errors = On") {
		t.Errorf("missing display_errors setting in:\n%s", s)
	}
	if !strings.Contains(s, "; Generated by dev") {
		t.Errorf("missing generated header in:\n%s", s)
	}
}

func TestWriteMysqlCnf(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settings := map[string]string{
		"innodb_buffer_pool_size": "1G",
		"max_connections":         "500",
		"skip-log-bin":            "",
		"ssl":                     "0",
	}

	err := WriteMysqlCnf(dir, settings)
	if err != nil {
		t.Fatalf("WriteMysqlCnf() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "mysql", "conf.d", "dev-overrides.cnf"))
	if err != nil {
		t.Fatalf("reading cnf file: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "[mysqld]") {
		t.Errorf("missing [mysqld] section in:\n%s", s)
	}
	if !strings.Contains(s, "innodb_buffer_pool_size = 1G") {
		t.Errorf("missing innodb_buffer_pool_size in:\n%s", s)
	}
	if !strings.Contains(s, "skip-log-bin\n") {
		t.Errorf("flag-style skip-log-bin should have no value in:\n%s", s)
	}
	if !strings.Contains(s, "ssl = 0") {
		t.Errorf("missing ssl setting in:\n%s", s)
	}
}

func TestWriteMysqlCnf_ProtectedKeysApplied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settings := MergeSettings(DefaultMysqlCnf, nil, nil)
	// Apply protected keys
	for k, v := range ProtectedMysqlCnf {
		settings[k] = v
	}

	err := WriteMysqlCnf(dir, settings)
	if err != nil {
		t.Fatalf("WriteMysqlCnf() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "mysql", "conf.d", "dev-overrides.cnf"))
	if err != nil {
		t.Fatalf("reading cnf file: %v", err)
	}

	if !strings.Contains(string(content), "ssl = 0") {
		t.Errorf("protected ssl key missing in:\n%s", string(content))
	}
}
```

Also add imports at the top of the test file:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run "TestWrite" -v`
Expected: FAIL — `WritePhpIni` and `WriteMysqlCnf` not defined

- [ ] **Step 3: Implement WritePhpIni and WriteMysqlCnf**

Append to `internal/config/defaults.go`:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WritePhpIni writes a dev-overrides.ini file to devDir/php/{version}/conf.d/.
func WritePhpIni(devDir, phpVersion string, settings map[string]string) error {
	dir := filepath.Join(devDir, "php", phpVersion, "conf.d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating php conf.d dir: %w", err)
	}

	content := formatIni(settings)
	path := filepath.Join(dir, "dev-overrides.ini")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// WriteMysqlCnf writes a dev-overrides.cnf file to devDir/mysql/conf.d/.
func WriteMysqlCnf(devDir string, settings map[string]string) error {
	dir := filepath.Join(devDir, "mysql", "conf.d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating mysql conf.d dir: %w", err)
	}

	content := formatCnf(settings)
	path := filepath.Join(dir, "dev-overrides.cnf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func formatIni(settings map[string]string) string {
	var b strings.Builder
	b.WriteString("; Generated by dev — do not edit manually\n")

	keys := sortedKeys(settings)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, settings[k])
	}
	return b.String()
}

func formatCnf(settings map[string]string) string {
	var b strings.Builder
	b.WriteString("; Generated by dev — do not edit manually\n")
	b.WriteString("[mysqld]\n")

	keys := sortedKeys(settings)
	for _, k := range keys {
		v := settings[k]
		if v == "" {
			fmt.Fprintf(&b, "%s\n", k)
		} else {
			fmt.Fprintf(&b, "%s = %s\n", k, v)
		}
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

Note: the `import` block should be at the top of `defaults.go`, combining with the existing `package config` declaration.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run "TestWrite|TestMerge" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/defaults.go internal/config/defaults_test.go
git commit -m "feat: add WritePhpIni and WriteMysqlCnf file writers"
```

---

### Task 3: Add PhpIni and MysqlCnf fields to config structs

**Files:**
- Modify: `internal/config/config.go:49-67`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/global.go:30-34`
- Modify: `internal/config/global_test.go`

- [ ] **Step 1: Write failing tests for PhpIni/MysqlCnf parsing**

Append to `internal/config/config_test.go`:

```go
func TestLoad_ParsesPhpIni(t *testing.T) {
	dir := t.TempDir()
	yaml := `
php_ini:
  memory_limit: 1G
  upload_max_filesize: 500M
`
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PhpIni["memory_limit"] != "1G" {
		t.Errorf("PhpIni[memory_limit] = %q, want %q", cfg.PhpIni["memory_limit"], "1G")
	}
	if cfg.PhpIni["upload_max_filesize"] != "500M" {
		t.Errorf("PhpIni[upload_max_filesize] = %q, want %q", cfg.PhpIni["upload_max_filesize"], "500M")
	}
}

func TestLoad_ParsesMysqlCnf(t *testing.T) {
	dir := t.TempDir()
	yaml := `
mysql_cnf:
  max_connections: "500"
  innodb_buffer_pool_size: 1G
`
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MysqlCnf["max_connections"] != "500" {
		t.Errorf("MysqlCnf[max_connections] = %q, want %q", cfg.MysqlCnf["max_connections"], "500")
	}
	if cfg.MysqlCnf["innodb_buffer_pool_size"] != "1G" {
		t.Errorf("MysqlCnf[innodb_buffer_pool_size] = %q, want %q", cfg.MysqlCnf["innodb_buffer_pool_size"], "1G")
	}
}
```

Append to `internal/config/global_test.go`:

```go
func TestLoadGlobal_ParsesPhpIni(t *testing.T) {
	dir := t.TempDir()
	content := `
php_ini:
  memory_limit: 1G
  display_errors: "Off"
`
	os.WriteFile(filepath.Join(dir, GlobalConfigFile), []byte(content), 0644)

	cfg, err := loadGlobal(dir)
	if err != nil {
		t.Fatalf("loadGlobal() error = %v", err)
	}

	if cfg.PhpIni["memory_limit"] != "1G" {
		t.Errorf("PhpIni[memory_limit] = %q, want %q", cfg.PhpIni["memory_limit"], "1G")
	}
	if cfg.PhpIni["display_errors"] != "Off" {
		t.Errorf("PhpIni[display_errors] = %q, want %q", cfg.PhpIni["display_errors"], "Off")
	}
}

func TestLoadGlobal_ParsesMysqlCnf(t *testing.T) {
	dir := t.TempDir()
	content := `
mysql_cnf:
  innodb_buffer_pool_size: 256M
`
	os.WriteFile(filepath.Join(dir, GlobalConfigFile), []byte(content), 0644)

	cfg, err := loadGlobal(dir)
	if err != nil {
		t.Fatalf("loadGlobal() error = %v", err)
	}

	if cfg.MysqlCnf["innodb_buffer_pool_size"] != "256M" {
		t.Errorf("MysqlCnf[innodb_buffer_pool_size] = %q, want %q", cfg.MysqlCnf["innodb_buffer_pool_size"], "256M")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run "TestLoad_ParsesPhpIni|TestLoad_ParsesMysqlCnf|TestLoadGlobal_ParsesPhpIni|TestLoadGlobal_ParsesMysqlCnf" -v`
Expected: FAIL — fields don't exist on structs

- [ ] **Step 3: Add fields to ProjectConfig**

In `internal/config/config.go`, add two fields to the `ProjectConfig` struct after `Services`:

```go
PhpIni         map[string]string            `yaml:"php_ini"`
MysqlCnf       map[string]string            `yaml:"mysql_cnf"`
```

- [ ] **Step 4: Add fields to GlobalConfig**

In `internal/config/global.go`, add two fields to `GlobalConfig` after `Versions`:

```go
PhpIni      map[string]string `yaml:"php_ini"`
MysqlCnf    map[string]string `yaml:"mysql_cnf"`
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -run "TestLoad_ParsesPhpIni|TestLoad_ParsesMysqlCnf|TestLoadGlobal_ParsesPhpIni|TestLoadGlobal_ParsesMysqlCnf" -v`
Expected: PASS

- [ ] **Step 6: Run full config test suite**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/config/global.go internal/config/global_test.go
git commit -m "feat: add PhpIni and MysqlCnf fields to project and global config"
```

---

### Task 4: Mount MySQL conf.d in Docker compose

**Files:**
- Modify: `internal/docker/docker.go:226-238`
- Modify: `internal/docker/docker_test.go`

- [ ] **Step 1: Write failing test for MySQL conf.d mount**

Append to `internal/docker/docker_test.go`:

```go
func TestGenerateCompose_MountsMysqlConfD(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2"))
	if !strings.Contains(got, "/mysql/conf.d:/etc/mysql/conf.d") {
		t.Errorf("missing mysql conf.d mount in:\n%s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/docker/ -run TestGenerateCompose_MountsMysqlConfD -v`
Expected: FAIL

- [ ] **Step 3: Add MySQL conf.d volume mount**

In `internal/docker/docker.go`, in the MySQL service section of `generateCompose`, after the existing `volumes:` line that mounts the data volume (around line 230), add the conf.d mount:

Find this block:
```go
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/var/lib/mysql\n", volName)
```

Replace with:
```go
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/var/lib/mysql\n", volName)
		fmt.Fprintf(&b, "      - %s/mysql/conf.d:/etc/mysql/conf.d\n", globalDir)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/docker/ -run TestGenerateCompose_MountsMysqlConfD -v`
Expected: PASS

- [ ] **Step 5: Run full docker test suite**

Run: `go test ./internal/docker/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/docker/docker.go internal/docker/docker_test.go
git commit -m "feat: mount MySQL conf.d directory in Docker compose"
```

---

### Task 5: Write settings on dev start

**Files:**
- Modify: `cmd/start.go:24-54`

- [ ] **Step 1: Add settings writing to startCmd**

In `cmd/start.go`, in the `RunE` function, after `global, err := config.LoadGlobal()` and before `imgCfg := phpimage.ImageConfig{`, add the settings writing logic:

```go
		// Write runtime PHP ini overrides
		phpIni := config.MergeSettings(config.DefaultPhpIni, global.PhpIni, p.Config.PhpIni)
		if err := config.WritePhpIni(config.GlobalDir(), p.Config.PHP, phpIni); err != nil {
			return fmt.Errorf("writing php.ini overrides: %w", err)
		}

		// Write runtime MySQL cnf overrides
		mysqlCnf := config.MergeSettings(config.DefaultMysqlCnf, global.MysqlCnf, p.Config.MysqlCnf)
		for k, v := range config.ProtectedMysqlCnf {
			mysqlCnf[k] = v
		}
		if err := config.WriteMysqlCnf(config.GlobalDir(), mysqlCnf); err != nil {
			return fmt.Errorf("writing my.cnf overrides: %w", err)
		}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add cmd/start.go
git commit -m "feat: write php.ini and my.cnf overrides on dev start"
```

---

### Task 6: Write MySQL cnf on dev services up

**Files:**
- Modify: `cmd/services.go:26-54`

- [ ] **Step 1: Add MySQL cnf writing to servicesUpCmd**

In `cmd/services.go`, in the `servicesUpCmd` `RunE` function, after `collected := config.CollectVersions(...)` and before `fmt.Println("Starting shared services...")`, add:

```go
		// Write runtime MySQL cnf overrides
		mysqlCnf := config.MergeSettings(config.DefaultMysqlCnf, global.MysqlCnf, nil)
		for k, v := range config.ProtectedMysqlCnf {
			mysqlCnf[k] = v
		}
		if err := config.WriteMysqlCnf(config.GlobalDir(), mysqlCnf); err != nil {
			return fmt.Errorf("writing my.cnf overrides: %w", err)
		}
```

Note: `dev services up` doesn't have a project context, so only defaults + global are merged (no per-project overrides). Per-project MySQL overrides take effect on `dev start` which rewrites the file.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add cmd/services.go
git commit -m "feat: write my.cnf overrides on dev services up"
```

---

### Task 7: Run full test suite and vet

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `go test ./... -count=1`
Expected: All PASS

- [ ] **Step 2: Run vet**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 3: Run tests with race detector**

Run: `go test ./... -race -count=1`
Expected: All PASS
