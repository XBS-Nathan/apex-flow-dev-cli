package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if err := s.waitForReady(); err != nil {
		return err
	}
	name := sanitizeDBName(dbName)
	for _, n := range []string{name, name + "_testing"} {
		sql := fmt.Sprintf(
			"SELECT 'CREATE DATABASE \"%s\"' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '%s') \\gexec\n",
			n, n,
		)
		cmd := dockerExec(s.Service,
			"psql", "-U", s.Config.User,
			"-d", "postgres",
		)
		cmd.Stdin = strings.NewReader(sql)
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

// Snapshot runs pg_dump inside the Docker container using custom format (-Fc),
// streaming the output to a file on the host.
func (s *PostgresStore) Snapshot(dbName, snapshotDir string) error {
	path := filepath.Join(snapshotDir, "dump.pgc")

	cmd := dockerExec(s.Service,
		"pg_dump",
		"-h", "127.0.0.1", "-U", s.Config.User,
		"-Fc",     // custom format (single-file, supports parallel restore)
		"-Z", "3", // compression level
		dbName,
	)
	cmd.Env = s.connEnv()

	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating snapshot file: %w", err)
	}
	defer outFile.Close()

	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump: %w", err)
	}
	return nil
}

// Restore detects the snapshot format and restores accordingly:
//   - .pgc file: pg_restore via Docker container (streamed through stdin)
//   - .sql.gz file: gunzip | psql via Docker container
//   - .sql file: psql via Docker container
//   - directory: pg_restore on host (requires local pg_restore)
func (s *PostgresStore) Restore(dbName, snapshotPath string) error {
	if IsFileSnapshot(snapshotPath) {
		if strings.HasSuffix(snapshotPath, ".pgc") {
			return s.restorePgCustom(dbName, snapshotPath)
		}
		return s.restoreFile(dbName, snapshotPath)
	}
	// Directory format requires pg_restore on the host (legacy snapshots).
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

// restorePgCustom restores from a .pgc (pg custom format) file by piping it
// into pg_restore running inside the Docker container.
func (s *PostgresStore) restorePgCustom(dbName, path string) error {
	inFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening snapshot: %w", err)
	}
	defer inFile.Close()

	cmd := dockerExec(s.Service,
		"pg_restore",
		"-h", "127.0.0.1", "-U", s.Config.User,
		"-d", dbName,
		"--clean",
		"--if-exists",
	)
	cmd.Env = s.connEnv()
	cmd.Stdin = inFile
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore: %w", err)
	}
	return nil
}

// restoreFile restores from a .sql or .sql.gz file via psql inside the Docker
// container.
func (s *PostgresStore) restoreFile(dbName, path string) error {
	inFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening snapshot: %w", err)
	}
	defer inFile.Close()

	psql := dockerExec(s.Service,
		"psql", "-U", s.Config.User,
		"-h", "127.0.0.1",
		"-d", dbName,
	)
	psql.Env = s.connEnv()
	psql.Stdout = os.Stdout
	psql.Stderr = os.Stderr

	switch {
	case strings.HasSuffix(path, ".zst"):
		decompress := exec.Command("zstd", "-d", "-q")
		decompress.Stdin = inFile
		decompress.Stderr = os.Stderr

		psql.Stdin, err = decompress.StdoutPipe()
		if err != nil {
			return fmt.Errorf("piping zstd to psql: %w", err)
		}
		if err := psql.Start(); err != nil {
			return fmt.Errorf("starting psql: %w", err)
		}
		if err := decompress.Run(); err != nil {
			return fmt.Errorf("running zstd decompress: %w", err)
		}
		if err := psql.Wait(); err != nil {
			return fmt.Errorf("psql restore: %w", err)
		}

	case strings.HasSuffix(path, ".gz"):
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

	default:
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
