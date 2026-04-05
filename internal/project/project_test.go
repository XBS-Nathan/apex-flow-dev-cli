package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

func TestDetect_FindsComposerJson(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	p, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if p.Dir != dir {
		t.Errorf("Dir = %q, want %q", p.Dir, dir)
	}
}

func TestDetect_FindsDevYaml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, config.ConfigFile), []byte(`php: "8.1"`), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	p, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if p.Config.PHP != "8.1" {
		t.Errorf("PHP = %q, want %q", p.Config.PHP, "8.1")
	}
}

func TestDetect_FindsPackageJson(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	p, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if p.Dir != dir {
		t.Errorf("Dir = %q, want %q", p.Dir, dir)
	}
}

func TestDetect_FindsGitDir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	p, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if p.Dir != dir {
		t.Errorf("Dir = %q, want %q", p.Dir, dir)
	}
}

func TestDetect_WalksUpDirectoryTree(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0644)

	subdir := filepath.Join(root, "app", "Http")
	os.MkdirAll(subdir, 0755)

	orig, _ := os.Getwd()
	os.Chdir(subdir)
	defer os.Chdir(orig)

	p, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if p.Dir != root {
		t.Errorf("Dir = %q, want %q (should walk up)", p.Dir, root)
	}
}

func TestDetect_NoProjectFound(t *testing.T) {
	dir := t.TempDir()

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	_, err := Detect()
	if err == nil {
		t.Error("expected error when no project markers found, got nil")
	}
}

func TestPHPBin(t *testing.T) {
	p := &Project{Config: &config.ProjectConfig{PHP: "8.1"}}
	if got := p.PHPBin(); got != "php8.1" {
		t.Errorf("PHPBin() = %q, want %q", got, "php8.1")
	}
}

func TestSiteDomain(t *testing.T) {
	p := &Project{Name: "xlinx-1"}
	if got := p.SiteDomain(); got != "xlinx-1.test" {
		t.Errorf("SiteDomain() = %q, want %q", got, "xlinx-1.test")
	}
}
