//go:build integration

package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func requirePostgresCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql CLI not found in PATH — install postgresql-client to run these tests")
	}
}

func setupPostgres(t *testing.T) config.PostgresConfig {
	t.Helper()
	requirePostgresCLI(t)

	ctx := context.Background()
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start Postgres container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	return config.PostgresConfig{
		User: "postgres",
		Pass: "postgres",
		Host: host,
		Port: port.Port(),
	}
}

// psqlExecDB runs SQL against a specific database (not the default "postgres" db).
func psqlExecDB(cfg config.PostgresConfig, dbName, sql string) error {
	cmd := exec.Command("psql",
		"-h", cfg.Host, "-p", cfg.Port, "-U", cfg.User,
		"-d", dbName,
		"-c", sql,
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", cfg.Pass))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func TestPostgresStore_CreateIfNotExists_CreatesDatabase(t *testing.T) {
	cfg := setupPostgres(t)
	store := &PostgresStore{Config: cfg}

	err := store.CreateIfNotExists("integration_test")
	if err != nil {
		t.Fatalf("CreateIfNotExists() error = %v", err)
	}

	// Verify databases exist by querying pg_database
	if err := psqlExecDB(cfg, "postgres", "SELECT datname FROM pg_database WHERE datname = 'integration_test'"); err != nil {
		t.Errorf("main database not created: %v", err)
	}
	if err := psqlExecDB(cfg, "postgres", "SELECT datname FROM pg_database WHERE datname = 'integration_test_testing'"); err != nil {
		t.Errorf("testing database not created: %v", err)
	}
}

func TestPostgresStore_CreateIfNotExists_IsIdempotent(t *testing.T) {
	cfg := setupPostgres(t)
	store := &PostgresStore{Config: cfg}

	if err := store.CreateIfNotExists("idempotent_test"); err != nil {
		t.Fatalf("first CreateIfNotExists() error = %v", err)
	}
	if err := store.CreateIfNotExists("idempotent_test"); err != nil {
		t.Fatalf("second CreateIfNotExists() error = %v", err)
	}
}

func TestPostgresStore_Drop_RemovesBothDatabases(t *testing.T) {
	cfg := setupPostgres(t)
	store := &PostgresStore{Config: cfg}

	if err := store.CreateIfNotExists("drop_test"); err != nil {
		t.Fatalf("CreateIfNotExists() error = %v", err)
	}

	if err := store.Drop("drop_test"); err != nil {
		t.Fatalf("Drop() error = %v", err)
	}

	// Verify databases are gone — connecting to them should fail
	if err := psqlExecDB(cfg, "drop_test", "SELECT 1"); err == nil {
		t.Error("main database still exists after Drop()")
	}
	if err := psqlExecDB(cfg, "drop_test_testing", "SELECT 1"); err == nil {
		t.Error("testing database still exists after Drop()")
	}
}

func TestPostgresStore_Snapshot_And_Restore(t *testing.T) {
	cfg := setupPostgres(t)
	store := &PostgresStore{Config: cfg}

	// Create database with test data
	if err := store.CreateIfNotExists("snap_test"); err != nil {
		t.Fatalf("CreateIfNotExists() error = %v", err)
	}
	if err := psqlExecDB(cfg, "snap_test", "CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(50)); INSERT INTO users VALUES (1, 'Alice'), (2, 'Bob');"); err != nil {
		t.Fatalf("creating test data: %v", err)
	}

	// Snapshot
	snapshotDir := t.TempDir()
	if err := store.Snapshot("snap_test", snapshotDir); err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	// Verify snapshot directory has files
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatalf("reading snapshot dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("snapshot directory is empty")
	}

	// Drop and recreate the database
	if err := store.Drop("snap_test"); err != nil {
		t.Fatalf("Drop() error = %v", err)
	}
	if err := store.CreateIfNotExists("snap_test"); err != nil {
		t.Fatalf("re-CreateIfNotExists() error = %v", err)
	}

	// Restore
	if err := store.Restore("snap_test", snapshotDir); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify data is back
	if err := psqlExecDB(cfg, "snap_test", "SELECT name FROM users WHERE name = 'Alice'"); err != nil {
		t.Errorf("data not restored: %v", err)
	}
}
