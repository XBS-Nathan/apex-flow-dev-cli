package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/XBS-Nathan/nova/internal/config"
)

// Store defines the operations every database adapter must support.
type Store interface {
	CreateIfNotExists(dbName string) error
	Drop(dbName string) error
	Snapshot(dbName, snapshotDir string) error
	Restore(dbName, snapshotDir string) error
}

// NewStore returns a Store for the given config's driver.
// serviceName is the docker compose service name (e.g. "mysql80", "postgres16").
func NewStore(dbc config.DBConfig, serviceName string) (Store, error) {
	switch dbc.Driver {
	case "postgres":
		return &PostgresStore{Config: dbc.Postgres, Service: serviceName}, nil
	case "mysql", "":
		return &MySQLStore{Config: dbc.MySQL, Service: serviceName}, nil
	default:
		return nil, fmt.Errorf("unsupported db_driver: %q", dbc.Driver)
	}
}

// SnapshotDir builds the global snapshot directory path, creating it if needed.
func SnapshotDir(dbName, label string) string {
	return snapshotDir(config.SnapshotDir(), dbName, label)
}

// LocalSnapshotDir builds a project-local snapshot directory under .nova/snapshots/.
func LocalSnapshotDir(projectDir, dbName, label string) string {
	baseDir := filepath.Join(projectDir, ".nova", "snapshots")
	return snapshotDir(baseDir, dbName, label)
}

// snapshotDir is the testable core that builds a snapshot directory under baseDir.
func snapshotDir(baseDir, dbName, label string) string {
	if label == "" {
		label = time.Now().Format("20060102_150405")
	}
	dir := filepath.Join(baseDir, dbName, fmt.Sprintf("%s_%s", dbName, label))
	os.MkdirAll(dir, 0755)
	return dir
}

// ListSnapshots returns available snapshots from the global directory.
func ListSnapshots(dbName string) ([]string, error) {
	return listSnapshots(config.SnapshotDir(), dbName)
}

// ListLocalSnapshots returns available snapshots from the project .nova/snapshots/ directory.
func ListLocalSnapshots(projectDir, dbName string) ([]string, error) {
	baseDir := filepath.Join(projectDir, ".nova", "snapshots")
	return listSnapshots(baseDir, dbName)
}

// listSnapshots is the testable core that lists snapshots under baseDir.
// Snapshots can be directories (mydumper/pg_dump -Fd) or files (.sql, .sql.gz).
func listSnapshots(baseDir, dbName string) ([]string, error) {
	dir := filepath.Join(baseDir, dbName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list snapshots for %s: %w", dbName, err)
	}

	var snapshots []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			snapshots = append(snapshots, filepath.Join(dir, name))
		} else if strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".sql.gz") || strings.HasSuffix(name, ".sql.zst") || strings.HasSuffix(name, ".pgc") {
			snapshots = append(snapshots, filepath.Join(dir, name))
		}
	}
	return snapshots, nil
}

// IsFileSnapshot returns true if the snapshot path is a file rather than a directory.
func IsFileSnapshot(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dockerExec runs a command inside a docker compose service container.
func dockerExec(service string, args ...string) *exec.Cmd {
	composeFile := filepath.Join(config.GlobalDir(), "docker-compose.yml")
	fullArgs := append(
		[]string{"compose", "-f", composeFile, "exec", "-T", service},
		args...,
	)
	return exec.Command("docker", fullArgs...)
}

// sanitizeDBName strips anything that isn't [a-z0-9_] to prevent malformed SQL.
func sanitizeDBName(name string) string {
	return config.SanitizeName(name, false)
}
