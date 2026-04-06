package db

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
)

// PostgresStore implements Store using psql/pg_dump inside a Docker container.
type PostgresStore struct {
	Config  config.PostgresConfig
	Service string // docker compose service name, e.g. "postgres16"
}

func (s *PostgresStore) CreateIfNotExists(dbName string) error {
	name := sanitizeDBName(dbName)
	for _, n := range []string{name, name + "_testing"} {
		sql := fmt.Sprintf(
			"SELECT 'CREATE DATABASE \"%s\"' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '%s')\\gexec",
			n, n,
		)
		cmd := dockerExec(s.Service,
			"psql", "-U", s.Config.User,
			"-d", "postgres",
			"-c", sql,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("postgres create %s: %s: %w",
				n, strings.TrimSpace(string(output)), err)
		}
	}
	return nil
}

func (s *PostgresStore) Drop(dbName string) error {
	name := sanitizeDBName(dbName)
	sql := fmt.Sprintf(
		"DROP DATABASE IF EXISTS \"%s\"; DROP DATABASE IF EXISTS \"%s_testing\";",
		name, name,
	)
	return s.exec(sql)
}

// Snapshot uses pg_dump with directory format and parallel jobs.
func (s *PostgresStore) Snapshot(dbName, snapshotDir string) error {
	cmd := exec.Command("pg_dump",
		"-h", s.Config.Host, "-p", s.Config.Port, "-U", s.Config.User,
		"-Fd",          // directory format
		"-j", "4",      // parallel jobs
		"-Z", "lz4:3",  // lz4 compression
		"-f", snapshotDir,
		dbName,
	)
	cmd.Env = s.connEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump: %w", err)
	}
	return nil
}

// Restore detects the snapshot format and restores accordingly:
//   - .sql.gz file: gunzip | psql
//   - .sql file: psql < file
//   - directory: pg_restore -j4 (parallel)
func (s *PostgresStore) Restore(dbName, snapshotPath string) error {
	if IsFileSnapshot(snapshotPath) {
		return s.restoreFile(dbName, snapshotPath)
	}
	cmd := exec.Command("pg_restore",
		"-h", s.Config.Host, "-p", s.Config.Port, "-U", s.Config.User,
		"-d", dbName,
		"-j", "4",
		"--clean",
		"--if-exists",
		snapshotPath,
	)
	cmd.Env = s.connEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore: %w", err)
	}
	return nil
}

// restoreFile restores from a .sql or .sql.gz file via psql.
func (s *PostgresStore) restoreFile(dbName, path string) error {
	inFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening snapshot: %w", err)
	}
	defer inFile.Close()

	psql := exec.Command("psql",
		"-h", s.Config.Host, "-p", s.Config.Port, "-U", s.Config.User,
		"-d", dbName,
	)
	psql.Env = s.connEnv()
	psql.Stdout = os.Stdout
	psql.Stderr = os.Stderr

	if strings.HasSuffix(path, ".gz") {
		gunzip := exec.Command("gunzip")
		gunzip.Stdin = inFile
		gunzip.Stderr = os.Stderr

		psql.Stdin, err = gunzip.StdoutPipe()
		if err != nil {
			return fmt.Errorf("piping gunzip to psql: %w", err)
		}

		if err := psql.Start(); err != nil {
			return fmt.Errorf("starting psql: %w", err)
		}
		if err := gunzip.Run(); err != nil {
			return fmt.Errorf("running gunzip: %w", err)
		}
		if err := psql.Wait(); err != nil {
			return fmt.Errorf("psql restore: %w", err)
		}
	} else {
		psql.Stdin = inFile
		if err := psql.Run(); err != nil {
			return fmt.Errorf("psql restore: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) connEnv() []string {
	return append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", s.Config.Pass),
	)
}

func (s *PostgresStore) exec(sql string) error {
	if err := s.waitForReady(); err != nil {
		return err
	}
	cmd := dockerExec(s.Service,
		"psql", "-U", s.Config.User,
		"-d", "postgres",
		"-c", sql,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("postgres: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// waitForReady polls Postgres until it accepts connections.
func (s *PostgresStore) waitForReady() error {
	return docker.WaitForReady(s.Service, 120*time.Second, []string{
		"pg_isready", "-U", s.Config.User,
	})
}
