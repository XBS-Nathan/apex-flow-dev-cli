package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/XBS-Nathan/nova/internal/config"
)

// Project represents a linked dev project.
type Project struct {
	Name   string
	Dir    string
	Config *config.ProjectConfig
}

// projectMarkers are files that indicate a project root.
var projectMarkers = []string{
	config.ConfigFile,
	"composer.json",
	"package.json",
	".git",
}

// Detect finds the project from the current working directory.
// It walks up looking for .dev.yaml or common project root markers.
func Detect() (*Project, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("detect project: %w", err)
	}

	// Walk up to find project root
	for {
		for _, marker := range projectMarkers {
			if hasFile(dir, marker) {
				cfg, err := config.Load(dir)
				if err != nil {
					return nil, fmt.Errorf("detect project: load config: %w", err)
				}
				return &Project{
					Name:   filepath.Base(dir),
					Dir:    dir,
					Config: cfg,
				}, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil, fmt.Errorf(
		"no project found (looked for %s or common project markers)",
		config.ConfigFile,
	)
}

func hasFile(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// PHPBin returns the path to the PHP binary for this project's version.
func (p *Project) PHPBin() string {
	return fmt.Sprintf("php%s", p.Config.PHP)
}

// SiteDomain returns the .test domain for this project.
func (p *Project) SiteDomain() string {
	return p.Config.Domain
}
