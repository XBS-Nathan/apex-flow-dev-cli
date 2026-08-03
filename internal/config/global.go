package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	GlobalConfigFile = "config.yaml"

	DefaultMySQLVersion    = "8.0"
	DefaultRedisVersion    = "latest"
	DefaultPostgresVersion = "16"
	DefaultMailpitVersion  = "latest"
)

// ServiceVersions holds Docker image versions for built-in shared services.
type ServiceVersions struct {
	MySQL    string `yaml:"mysql"`
	Redis    string `yaml:"redis"`
	Postgres string `yaml:"postgres"`
	Mailpit  string `yaml:"mailpit"`
}

// GlobalConfig holds user-level configuration from ~/.nova/config.yaml.
type GlobalConfig struct {
	ProjectsDir string            `yaml:"projects_dir"`
	PHPVersions []string          `yaml:"php_versions"`
	Versions    ServiceVersions   `yaml:"versions"`
	PhpIni      map[string]string `yaml:"php_ini"`
	MysqlCnf    map[string]string `yaml:"mysql_cnf"`

	// DNS enables the shared wildcard .test DNS responder so other
	// machines (e.g. over Tailscale) can resolve Nova sites.
	DNS bool `yaml:"dns"`
	// DNSBind pins the responder's bind IP; when empty the Tailscale
	// IPv4 address is auto-detected.
	DNSBind string `yaml:"dns_bind"`
}

// loadGlobal reads devDir/config.yaml and returns a GlobalConfig with defaults applied.
// It is intentionally unexported so tests can inject a temp directory.
func loadGlobal(devDir string) (*GlobalConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determining home directory: %w", err)
	}

	cfg := &GlobalConfig{}

	path := filepath.Join(devDir, GlobalConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.ProjectsDir = filepath.Join(home, "Projects")
			fillServiceVersionDefaults(&cfg.Versions)
			return cfg, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Expand ~/... in projects_dir.
	if strings.HasPrefix(cfg.ProjectsDir, "~/") {
		cfg.ProjectsDir = filepath.Join(home, cfg.ProjectsDir[2:])
	}

	// Default projects_dir when empty.
	if cfg.ProjectsDir == "" {
		cfg.ProjectsDir = filepath.Join(home, "Projects")
	}

	fillServiceVersionDefaults(&cfg.Versions)

	return cfg, nil
}

func fillServiceVersionDefaults(v *ServiceVersions) {
	if v.MySQL == "" {
		v.MySQL = DefaultMySQLVersion
	}
	if v.Redis == "" {
		v.Redis = DefaultRedisVersion
	}
	if v.Mailpit == "" {
		v.Mailpit = DefaultMailpitVersion
	}
}

// LoadGlobal reads ~/.nova/config.yaml and returns a GlobalConfig with defaults applied.
func LoadGlobal() (*GlobalConfig, error) {
	return loadGlobal(GlobalDir())
}
