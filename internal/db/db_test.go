package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/XBS-Nathan/nova/internal/config"
)

func TestSanitizeDBName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase passthrough",  "mydb",           "mydb"},
		{"uppercase to lower",     "MyDB",           "mydb"},
		{"strips hyphens",         "my-db",          "mydb"},
		{"strips dots",            "my.db",          "mydb"},
		{"strips backticks",       "my`db",          "mydb"},
		{"strips spaces",          "my db",          "mydb"},
		{"preserves underscores",  "my_db",          "my_db"},
		{"preserves numbers",      "db123",          "db123"},
		{"all special chars",      "!@#$%^&*()",     ""},
		{"mixed",                  "My-App.v2_test", "myappv2_test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDBName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeDBName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewStore(t *testing.T) {
	t.Run("mysql driver", func(t *testing.T) {
		store, err := NewStore(config.DBConfig{Driver: "mysql"}, "mysql")
		if err != nil {
			t.Fatalf("NewStore() error = %v", err)
		}
		if _, ok := store.(*MySQLStore); !ok {
			t.Errorf("expected *MySQLStore, got %T", store)
		}
	})

	t.Run("empty driver defaults to mysql", func(t *testing.T) {
		store, err := NewStore(config.DBConfig{Driver: ""}, "mysql")
		if err != nil {
			t.Fatalf("NewStore() error = %v", err)
		}
		if _, ok := store.(*MySQLStore); !ok {
			t.Errorf("expected *MySQLStore, got %T", store)
		}
	})

	t.Run("postgres driver", func(t *testing.T) {
		store, err := NewStore(config.DBConfig{Driver: "postgres"}, "postgres")
		if err != nil {
			t.Fatalf("NewStore() error = %v", err)
		}
		if _, ok := store.(*PostgresStore); !ok {
			t.Errorf("expected *PostgresStore, got %T", store)
		}
	})

	t.Run("unsupported driver", func(t *testing.T) {
		_, err := NewStore(config.DBConfig{Driver: "sqlite"}, "sqlite")
		if err == nil {
			t.Error("expected error for unsupported driver, got nil")
		}
	})
}

func TestSnapshotDir_WithLabel(t *testing.T) {
	base := t.TempDir()

	dir := snapshotDir(base, "mydb", "before_migration")

	want := filepath.Join(base, "mydb", "mydb_before_migration")
	if dir != want {
		t.Errorf("snapshotDir() = %q, want %q", dir, want)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

func TestSnapshotDir_EmptyLabel_UsesTimestamp(t *testing.T) {
	base := t.TempDir()

	dir := snapshotDir(base, "mydb", "")

	baseName := filepath.Base(dir)
	if len(baseName) < len("mydb_20060102_150405") {
		t.Errorf("expected timestamped name, got %q", baseName)
	}
}

func TestListSnapshots_EmptyDir(t *testing.T) {
	base := t.TempDir()

	snapshots, err := listSnapshots(base, "mydb")
	if err != nil {
		t.Fatalf("listSnapshots() error = %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestListSnapshots_FindsDirectories(t *testing.T) {
	base := t.TempDir()
	dbDir := filepath.Join(base, "mydb")
	os.MkdirAll(filepath.Join(dbDir, "mydb_20240101_120000"), 0755)
	os.MkdirAll(filepath.Join(dbDir, "mydb_20240102_120000"), 0755)

	snapshots, err := listSnapshots(base, "mydb")
	if err != nil {
		t.Fatalf("listSnapshots() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}
}

func TestListSnapshots_FindsFiles(t *testing.T) {
	base := t.TempDir()
	dbDir := filepath.Join(base, "mydb")
	os.MkdirAll(dbDir, 0755)
	os.WriteFile(filepath.Join(dbDir, "mydb_20240101.sql.gz"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dbDir, "mydb_20240102.sql"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dbDir, "notes.txt"), []byte("data"), 0644)

	snapshots, err := listSnapshots(base, "mydb")
	if err != nil {
		t.Fatalf("listSnapshots() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d: %v", len(snapshots), snapshots)
	}
}

func TestIsFileSnapshot(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "dump.sql.gz")
	os.WriteFile(file, []byte("data"), 0644)

	subdir := filepath.Join(dir, "snapshot_dir")
	os.MkdirAll(subdir, 0755)

	if !IsFileSnapshot(file) {
		t.Error("expected true for file, got false")
	}
	if IsFileSnapshot(subdir) {
		t.Error("expected false for directory, got true")
	}
	if IsFileSnapshot(filepath.Join(dir, "nonexistent")) {
		t.Error("expected false for nonexistent path, got true")
	}
}
