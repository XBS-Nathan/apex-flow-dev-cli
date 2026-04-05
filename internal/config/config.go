package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPHP  = "8.2"
	DefaultNode = "22"
	DevDir      = ".dev"
	ConfigFile  = ".dev.yaml"
)

// GlobalDir returns ~/.dev, creating it if needed.
func GlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Join(home, DevDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create %s: %v\n", dir, err)
		os.Exit(1)
	}
	return dir
}

// SnapshotDir returns ~/.dev/snapshots, creating it if needed.
func SnapshotDir() string {
	dir := filepath.Join(GlobalDir(), "snapshots")
	_ = os.MkdirAll(dir, 0755) // errors surface when caller writes
	return dir
}

// ProjectType determines which framework-specific commands are available.
const (
	TypeLaravel = "laravel"
	TypeGeneric = "generic"
)

// ProjectConfig represents a .dev.yaml file in a project root.
type ProjectConfig struct {
	Type     string                       `yaml:"type"`
	PHP      string                       `yaml:"php"`
	Node     string                       `yaml:"node"`
	DBDriver string                       `yaml:"db_driver"`
	DB       string                       `yaml:"db"`
	MySQL    MySQLConfig                  `yaml:"mysql"`
	Postgres PostgresConfig               `yaml:"postgres"`
	Hooks    Hooks                        `yaml:"hooks"`
	Services map[string]ServiceDefinition `yaml:"services"`
}

// DBConfig returns a unified database config for the db package.
func (c *ProjectConfig) DBConfig() DBConfig {
	return DBConfig{
		Driver:   c.DBDriver,
		Name:     c.DB,
		MySQL:    c.MySQL,
		Postgres: c.Postgres,
	}
}

// DBConfig holds the driver choice and connection settings for both backends.
type DBConfig struct {
	Driver   string
	Name     string
	MySQL    MySQLConfig
	Postgres PostgresConfig
}

// MySQLConfig holds MySQL connection settings.
type MySQLConfig struct {
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

// PostgresConfig holds PostgreSQL connection settings.
type PostgresConfig struct {
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

type Hooks struct {
	PostStart []string `yaml:"post-start"`
	PostStop  []string `yaml:"post-stop"`
}

type ServiceDefinition struct {
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports"`
	Environment map[string]string `yaml:"environment"`
	Volumes     []string          `yaml:"volumes"`
	Command     string            `yaml:"command"`
}

// Load reads .dev.yaml from the given project directory, returning defaults if not found.
func Load(projectDir string) (*ProjectConfig, error) {
	cfg := &ProjectConfig{
		PHP:      DefaultPHP,
		Node:     DefaultNode,
		DBDriver: "mysql",
		MySQL: MySQLConfig{
			User: "root",
			Pass: "root",
			Host: "127.0.0.1",
			Port: "3306",
		},
		Postgres: PostgresConfig{
			User: "postgres",
			Pass: "postgres",
			Host: "127.0.0.1",
			Port: "5432",
		},
	}

	path := filepath.Join(projectDir, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.Type = detectType(projectDir)
			cfg.DB = dbNameFromDir(projectDir)
			cfg.Hooks = defaultHooksForType(cfg.Type)
			return cfg, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if cfg.Type == "" {
		cfg.Type = detectType(projectDir)
	}
	if cfg.PHP == "" {
		cfg.PHP = DefaultPHP
	}
	if cfg.Node == "" {
		cfg.Node = DefaultNode
	}
	if cfg.DB == "" {
		cfg.DB = dbNameFromDir(projectDir)
	}
	if len(cfg.Hooks.PostStart) == 0 && len(cfg.Hooks.PostStop) == 0 {
		cfg.Hooks = defaultHooksForType(cfg.Type)
	}
	if cfg.DBDriver == "" {
		cfg.DBDriver = "mysql"
	}
	if cfg.MySQL.User == "" {
		cfg.MySQL.User = "root"
	}
	if cfg.MySQL.Pass == "" {
		cfg.MySQL.Pass = "root"
	}
	if cfg.MySQL.Host == "" {
		cfg.MySQL.Host = "127.0.0.1"
	}
	if cfg.MySQL.Port == "" {
		cfg.MySQL.Port = "3306"
	}
	if cfg.Postgres.User == "" {
		cfg.Postgres.User = "postgres"
	}
	if cfg.Postgres.Pass == "" {
		cfg.Postgres.Pass = "postgres"
	}
	if cfg.Postgres.Host == "" {
		cfg.Postgres.Host = "127.0.0.1"
	}
	if cfg.Postgres.Port == "" {
		cfg.Postgres.Port = "5432"
	}

	return cfg, nil
}

// detectType auto-detects the project type from files present in the directory.
func detectType(projectDir string) string {
	// Check for Laravel's artisan file
	if _, err := os.Stat(filepath.Join(projectDir, "artisan")); err == nil {
		return TypeLaravel
	}
	return TypeGeneric
}

// defaultHooksForType returns sensible default hooks for a project type.
func defaultHooksForType(projectType string) Hooks {
	switch projectType {
	case TypeLaravel:
		return Hooks{
			PostStart: []string{
				"php artisan horizon &",
				"yarn run hot &",
			},
		}
	default:
		return Hooks{}
	}
}

// dbNameFromDir derives a database name from the directory name.
// e.g., /home/nathan/Projects/xlinx-1 -> xlinx_1
func dbNameFromDir(dir string) string {
	name := filepath.Base(dir)
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' || c == '.' {
			result = append(result, '_')
		} else if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		}
	}
	return string(result)
}
