//go:build integration

package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

func requireMySQLCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("mysql"); err != nil {
		t.Skip("mysql CLI not found in PATH — install mysql-client to run these tests")
	}
}

func setupMySQL(t *testing.T) config.MySQLConfig {
	t.Helper()
	requireMySQLCLI(t)

	ctx := context.Background()
	container, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("root"),
		mysql.WithPassword("root"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("ready for connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start MySQL container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	return config.MySQLConfig{
		User: "root",
		Pass: "root",
		Host: host,
		Port: port.Port(),
	}
}

func TestMySQLStore_CreateIfNotExists_CreatesDatabase(t *testing.T) {
	cfg := setupMySQL(t)
	store := &MySQLStore{Config: cfg}

	err := store.CreateIfNotExists("integration_test")
	if err != nil {
		t.Fatalf("CreateIfNotExists() error = %v", err)
	}

	// Verify both databases exist by connecting to them
	err = store.exec(fmt.Sprintf("USE `%s`", "integration_test"))
	if err != nil {
		t.Errorf("main database not created: %v", err)
	}
	err = store.exec(fmt.Sprintf("USE `%s`", "integration_test_testing"))
	if err != nil {
		t.Errorf("testing database not created: %v", err)
	}
}

func TestMySQLStore_CreateIfNotExists_IsIdempotent(t *testing.T) {
	cfg := setupMySQL(t)
	store := &MySQLStore{Config: cfg}

	if err := store.CreateIfNotExists("idempotent_test"); err != nil {
		t.Fatalf("first CreateIfNotExists() error = %v", err)
	}
	if err := store.CreateIfNotExists("idempotent_test"); err != nil {
		t.Fatalf("second CreateIfNotExists() error = %v", err)
	}
}

func TestMySQLStore_Drop_RemovesBothDatabases(t *testing.T) {
	cfg := setupMySQL(t)
	store := &MySQLStore{Config: cfg}

	if err := store.CreateIfNotExists("drop_test"); err != nil {
		t.Fatalf("CreateIfNotExists() error = %v", err)
	}

	if err := store.Drop("drop_test"); err != nil {
		t.Fatalf("Drop() error = %v", err)
	}

	// Verify databases are gone
	err := store.exec("USE `drop_test`")
	if err == nil {
		t.Error("main database still exists after Drop()")
	}
	err = store.exec("USE `drop_test_testing`")
	if err == nil {
		t.Error("testing database still exists after Drop()")
	}
}

func TestMySQLStore_Snapshot_And_Restore(t *testing.T) {
	cfg := setupMySQL(t)
	store := &MySQLStore{Config: cfg}

	// Create database and a table with data
	if err := store.CreateIfNotExists("snap_test"); err != nil {
		t.Fatalf("CreateIfNotExists() error = %v", err)
	}
	if err := store.exec("USE snap_test; CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(50)); INSERT INTO users VALUES (1, 'Alice'), (2, 'Bob')"); err != nil {
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
	if err := store.exec("USE snap_test; SELECT * FROM users WHERE name = 'Alice'"); err != nil {
		t.Error("data not restored: could not query users table")
	}
}
