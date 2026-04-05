package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDbNameFromDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"simple name",            "/home/user/projects/myapp",       "myapp"},
		{"hyphens to underscores", "/home/user/projects/xlinx-1",    "xlinx_1"},
		{"dots to underscores",    "/home/user/projects/my.app",     "my_app"},
		{"uppercase to lower",     "/home/user/projects/MyApp",      "myapp"},
		{"mixed special chars",    "/home/user/projects/My-App.v2",  "my_app_v2"},
		{"strips backticks",       "/home/user/projects/my" + "`" + "app", "myapp"},
		{"strips spaces",          "/home/user/projects/my app",     "myapp"},
		{"preserves underscores",  "/home/user/projects/my_app",     "my_app"},
		{"numbers preserved",      "/home/user/projects/app123",     "app123"},
		{"all special chars",      "/home/user/projects/!@#$%",      ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dbNameFromDir(tt.dir)
			if got != tt.want {
				t.Errorf("dbNameFromDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestDetectType(t *testing.T) {
	t.Run("detects laravel when artisan file exists", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php"), 0644)

		got := detectType(dir)
		if got != TypeLaravel {
			t.Errorf("detectType() = %q, want %q", got, TypeLaravel)
		}
	})

	t.Run("returns generic when no artisan file", func(t *testing.T) {
		dir := t.TempDir()

		got := detectType(dir)
		if got != TypeGeneric {
			t.Errorf("detectType() = %q, want %q", got, TypeGeneric)
		}
	})
}

func TestDefaultHooksForType(t *testing.T) {
	t.Run("laravel has default hooks", func(t *testing.T) {
		hooks := defaultHooksForType(TypeLaravel)
		if len(hooks.PostStart) == 0 {
			t.Error("expected PostStart hooks for laravel, got none")
		}
	})

	t.Run("generic has no hooks", func(t *testing.T) {
		hooks := defaultHooksForType(TypeGeneric)
		if len(hooks.PostStart) != 0 {
			t.Errorf("expected no PostStart hooks for generic, got %d", len(hooks.PostStart))
		}
	})
}

func TestLoad_NoConfigFile(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PHP != DefaultPHP {
		t.Errorf("PHP = %q, want %q", cfg.PHP, DefaultPHP)
	}
	if cfg.Node != DefaultNode {
		t.Errorf("Node = %q, want %q", cfg.Node, DefaultNode)
	}
	if cfg.DBDriver != "mysql" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "mysql")
	}
	if cfg.Type != TypeGeneric {
		t.Errorf("Type = %q, want %q", cfg.Type, TypeGeneric)
	}
	if cfg.DB == "" {
		t.Error("DB should be derived from directory name, got empty")
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	dir := t.TempDir()
	yaml := `
php: "8.1"
node: "20"
type: generic
db: custom_db
db_driver: postgres
hooks:
  post-start:
    - "my-worker &"
  post-stop:
    - "cleanup.sh"
`
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PHP != "8.1" {
		t.Errorf("PHP = %q, want %q", cfg.PHP, "8.1")
	}
	if cfg.Node != "20" {
		t.Errorf("Node = %q, want %q", cfg.Node, "20")
	}
	if cfg.Type != TypeGeneric {
		t.Errorf("Type = %q, want %q", cfg.Type, TypeGeneric)
	}
	if cfg.DB != "custom_db" {
		t.Errorf("DB = %q, want %q", cfg.DB, "custom_db")
	}
	if cfg.DBDriver != "postgres" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "postgres")
	}
	if len(cfg.Hooks.PostStart) != 1 || cfg.Hooks.PostStart[0] != "my-worker &" {
		t.Errorf("PostStart = %v, want [\"my-worker &\"]", cfg.Hooks.PostStart)
	}
	if len(cfg.Hooks.PostStop) != 1 || cfg.Hooks.PostStop[0] != "cleanup.sh" {
		t.Errorf("PostStop = %v, want [\"cleanup.sh\"]", cfg.Hooks.PostStop)
	}
}

func TestLoad_PartialConfig_FillsDefaults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`php: "8.1"`), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PHP != "8.1" {
		t.Errorf("PHP = %q, want %q", cfg.PHP, "8.1")
	}
	if cfg.Node != DefaultNode {
		t.Errorf("Node = %q, want default %q", cfg.Node, DefaultNode)
	}
	if cfg.DBDriver != "mysql" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "mysql")
	}
	if cfg.MySQL.User != "root" {
		t.Errorf("MySQL.User = %q, want %q", cfg.MySQL.User, "root")
	}
	if cfg.Postgres.User != "postgres" {
		t.Errorf("Postgres.User = %q, want %q", cfg.Postgres.User, "postgres")
	}
}

func TestLoad_LaravelAutoDetect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php"), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Type != TypeLaravel {
		t.Errorf("Type = %q, want %q", cfg.Type, TypeLaravel)
	}
	if len(cfg.Hooks.PostStart) == 0 {
		t.Error("expected default Laravel hooks, got none")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte("{{invalid yaml"), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestLoad_PostStopPreserved(t *testing.T) {
	dir := t.TempDir()
	yaml := `
hooks:
  post-stop:
    - "cleanup.sh"
`
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Hooks.PostStop) != 1 || cfg.Hooks.PostStop[0] != "cleanup.sh" {
		t.Errorf("PostStop = %v, want [\"cleanup.sh\"]", cfg.Hooks.PostStop)
	}
}
