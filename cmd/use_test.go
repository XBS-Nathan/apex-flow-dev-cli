package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/XBS-Nathan/nova/internal/config"
)

// chdirTest changes to dir and registers cleanup to restore the original.
func chdirTest(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestSetConfigField_CreatesNovaDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0644)

	chdirTest(t, dir)

	if err := setConfigField("php", "8.4"); err != nil {
		t.Fatalf("setConfigField() error = %v", err)
	}

	cfgPath := filepath.Join(dir, config.ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if got := string(data); got != "php: \"8.4\"\n" {
		t.Errorf("config content = %q, want %q", got, "php: \"8.4\"\n")
	}
}

func TestSetConfigField_PreservesExistingFields(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0644)

	novaDir := filepath.Join(dir, ".nova")
	os.MkdirAll(novaDir, 0755)
	os.WriteFile(filepath.Join(dir, config.ConfigFile), []byte("php: \"8.3\"\ndb_driver: postgres\n"), 0644)

	chdirTest(t, dir)

	if err := setConfigField("php", "8.4"); err != nil {
		t.Fatalf("setConfigField() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	content := string(data)
	if !contains(content, "php: \"8.4\"") {
		t.Errorf("expected php: \"8.4\" in config, got:\n%s", content)
	}
	if !contains(content, "db_driver: postgres") {
		t.Errorf("expected db_driver: postgres preserved in config, got:\n%s", content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
