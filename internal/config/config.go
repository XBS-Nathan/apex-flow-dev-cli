package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPHP            = "8.2"
	DefaultNode           = "22"
	DefaultPackageManager = "npm"
	NovaDir               = ".nova"
	ConfigFile            = ".nova.yaml"
)

// GlobalDir returns ~/.nova, creating it and its subdirectories if needed.
// Subdirectories are pre-created before Docker can claim them as root-owned
// volumes, preventing permission errors on first run.
func GlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Join(home, NovaDir)

	subdirs := []string{
		"",
		"caddy/sites",
		"caddy/data",
	}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot create %s: %v\n", filepath.Join(dir, sub), err)
			os.Exit(1)
		}
	}

	return dir
}

// SnapshotDir returns ~/.nova/snapshots, creating it if needed.
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

// ProjectConfig represents a .nova.yaml file in a project root.
type ProjectConfig struct {
	Type         string                       `yaml:"type"`
	Domain         string                       `yaml:"domain"`
	PHP            string                       `yaml:"php"`
	Node           string                       `yaml:"node"`
	PackageManager string                       `yaml:"package_manager"`
	DBDriver     string                       `yaml:"db_driver"`
	DB           string                       `yaml:"db"`
	DBVersion    string                       `yaml:"db_version"`
	RedisVersion string                       `yaml:"redis_version"`
	Ports        []string                     `yaml:"ports"`
	NodeCommand  string                       `yaml:"node_command"`
	Extensions   []string                     `yaml:"extensions"`
	MySQL        MySQLConfig                  `yaml:"mysql"`
	Postgres     PostgresConfig               `yaml:"postgres"`
	Hooks          Hooks                        `yaml:"hooks"`
	SharedServices map[string]ServiceDefinition `yaml:"shared_services"`
	Services       map[string]ServiceDefinition `yaml:"services"`
	PhpIni         map[string]string            `yaml:"php_ini"`
	MysqlCnf       map[string]string            `yaml:"mysql_cnf"`
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

// Load reads .nova.yaml from the given project directory, returning defaults if not found.
func Load(projectDir string) (*ProjectConfig, error) {
	cfg := &ProjectConfig{}

	path := filepath.Join(projectDir, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fillDefaults(cfg, projectDir)
			return cfg, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	fillDefaults(cfg, projectDir)
	return cfg, nil
}

// fillDefaults sets zero-value fields to their defaults.
func fillDefaults(cfg *ProjectConfig, projectDir string) {
	if cfg.Type == "" {
		cfg.Type = detectType(projectDir)
	}
	if cfg.Domain == "" {
		cfg.Domain = filepath.Base(projectDir) + ".test"
	}
	if cfg.PHP == "" {
		cfg.PHP = DefaultPHP
	}
	if cfg.Node == "" {
		cfg.Node = DefaultNode
	}
	if cfg.PackageManager == "" {
		cfg.PackageManager = DefaultPackageManager
	}
	if cfg.DB == "" {
		cfg.DB = dbNameFromDir(projectDir)
	}
	if cfg.DBDriver == "" {
		cfg.DBDriver = "mysql"
	}
	if cfg.DBVersion == "" {
		if cfg.DBDriver == "postgres" {
			cfg.DBVersion = DefaultPostgresVersion
		} else {
			cfg.DBVersion = DefaultMySQLVersion
		}
	}
	if cfg.RedisVersion == "" {
		cfg.RedisVersion = DefaultRedisVersion
	}
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = defaultExtensionsForType(cfg.Type)
	}
	if len(cfg.Hooks.PostStart) == 0 && len(cfg.Hooks.PostStop) == 0 {
		cfg.Hooks = defaultHooksForType(cfg.Type)
	}
	fillMySQLDefaults(&cfg.MySQL)
	fillPostgresDefaults(&cfg.Postgres)
}

func fillMySQLDefaults(m *MySQLConfig) {
	if m.User == "" {
		m.User = "root"
	}
	if m.Pass == "" {
		m.Pass = "root"
	}
	if m.Host == "" {
		m.Host = "127.0.0.1"
	}
	if m.Port == "" {
		m.Port = "3306"
	}
}

func fillPostgresDefaults(p *PostgresConfig) {
	if p.User == "" {
		p.User = "postgres"
	}
	if p.Pass == "" {
		p.Pass = "postgres"
	}
	if p.Host == "" {
		p.Host = "127.0.0.1"
	}
	if p.Port == "" {
		p.Port = "5432"
	}
}

// defaultExtensionsForType returns PHP extensions commonly needed by a project type.
func defaultExtensionsForType(projectType string) []string {
	switch projectType {
	case TypeLaravel:
		return []string{"gd", "zip", "intl", "exif"}
	default:
		return nil
	}
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

// CollectedVersions holds unique service versions found across all projects.
type CollectedVersions struct {
	MySQL          []string
	Postgres       []string
	Redis          []string
	SharedServices map[string]ServiceDefinition
}

// CollectVersions scans projectsDir for .nova.yaml files and returns all
// unique service versions needed. The current project's versions are always
// included.
func CollectVersions(projectsDir string, current *ProjectConfig) CollectedVersions {
	mysqlSet := make(map[string]bool)
	pgSet := make(map[string]bool)
	redisSet := make(map[string]bool)
	sharedSvcs := make(map[string]ServiceDefinition)

	// Scan all project directories
	entries, _ := os.ReadDir(projectsDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfg, err := Load(filepath.Join(projectsDir, e.Name()))
		if err != nil {
			continue
		}
		addVersionToSet(cfg, mysqlSet, pgSet, redisSet)
		collectSharedServices(cfg, sharedSvcs)
	}

	// Always include current project
	addVersionToSet(current, mysqlSet, pgSet, redisSet)
	collectSharedServices(current, sharedSvcs)

	return CollectedVersions{
		MySQL:          setToSorted(mysqlSet),
		Postgres:       setToSorted(pgSet),
		Redis:          setToSorted(redisSet),
		SharedServices: sharedSvcs,
	}
}

// collectSharedServices merges a project's shared_services into the map.
// First definition wins — subsequent projects with the same service name
// are assumed to want the same shared instance.
func collectSharedServices(cfg *ProjectConfig, dest map[string]ServiceDefinition) {
	for name, svc := range cfg.SharedServices {
		if _, exists := dest[name]; !exists {
			dest[name] = svc
		}
	}
}

func addVersionToSet(
	cfg *ProjectConfig,
	mysqlSet, pgSet, redisSet map[string]bool,
) {
	if cfg.DBDriver == "mysql" {
		mysqlSet[cfg.DBVersion] = true
	} else if cfg.DBDriver == "postgres" {
		pgSet[cfg.DBVersion] = true
	}
	redisSet[cfg.RedisVersion] = true
}

func setToSorted(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// SanitizeName lowercases a string and keeps only [a-z0-9_].
// If replaceHyphens is true, hyphens and dots become underscores;
// otherwise they are stripped.
func SanitizeName(name string, replaceHyphens bool) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if replaceHyphens && (c == '-' || c == '.') {
			result = append(result, '_')
		} else if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		}
	}
	return string(result)
}

// dbNameFromDir derives a database name from the directory name.
// e.g., /home/nathan/Projects/xlinx-1 -> xlinx_1
func dbNameFromDir(dir string) string {
	return SanitizeName(filepath.Base(dir), true)
}
