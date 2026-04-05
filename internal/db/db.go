package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

// Store defines the operations every database adapter must support.
type Store interface {
	CreateIfNotExists(dbName string) error
	Drop(dbName string) error
	Snapshot(dbName, snapshotDir string) error
	Restore(dbName, snapshotDir string) error
}

// NewStore returns a Store for the given config's driver.
func NewStore(dbc config.DBConfig) (Store, error) {
	switch dbc.Driver {
	case "postgres":
		return &PostgresStore{Config: dbc.Postgres}, nil
	case "mysql", "":
		return &MySQLStore{Config: dbc.MySQL}, nil
	default:
		return nil, fmt.Errorf("unsupported db_driver: %q", dbc.Driver)
	}
}

// SnapshotDir builds the snapshot directory path, creating it if needed.
func SnapshotDir(dbName, label string) string {
	return snapshotDir(config.SnapshotDir(), dbName, label)
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

// ListSnapshots returns available snapshots for a database.
func ListSnapshots(dbName string) ([]string, error) {
	return listSnapshots(config.SnapshotDir(), dbName)
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
		} else if strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".sql.gz") {
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

// sanitizeDBName strips anything that isn't [a-z0-9_] to prevent malformed SQL.
func sanitizeDBName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32)
		}
	}
	return string(result)
}
