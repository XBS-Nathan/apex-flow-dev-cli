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

// MySQLStore implements Store using the mysql CLI inside a Docker container.
type MySQLStore struct {
	Config  config.MySQLConfig
	Service string // docker compose service name, e.g. "mysql80"
}

func (s *MySQLStore) CreateIfNotExists(dbName string) error {
	name := sanitizeDBName(dbName)
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; CREATE DATABASE IF NOT EXISTS `%s_testing`;",
		name, name,
	)
	return s.exec(sql)
}

func (s *MySQLStore) Drop(dbName string) error {
	name := sanitizeDBName(dbName)
	sql := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; DROP DATABASE IF EXISTS `%s_testing`;",
		name, name,
	)
	return s.exec(sql)
}

// Snapshot uses mydumper for parallel, directory-based dumps.
// Falls back to mysqldump if mydumper is not installed.
func (s *MySQLStore) Snapshot(dbName, snapshotDir string) error {
	if _, err := exec.LookPath("mydumper"); err == nil {
		return s.mydumperSnapshot(dbName, snapshotDir)
	}
	return s.mysqldumpSnapshot(dbName, snapshotDir)
}

// Restore detects the snapshot format and restores accordingly:
//   - .sql.zst file: zstd -d | mysql (in container)
//   - .sql.gz file: gunzip | mysql (in container)
//   - .sql file: mysql < file (in container)
//   - directory: myloader on host, or fallback to dump file inside the dir
func (s *MySQLStore) Restore(dbName, snapshotPath string) error {
	if IsFileSnapshot(snapshotPath) {
		return s.mysqlRestoreFile(dbName, snapshotPath)
	}
	if _, err := exec.LookPath("myloader"); err == nil {
		return s.myloaderRestore(dbName, snapshotPath)
	}
	// Try zstd first, then gz fallback for legacy snapshots.
	zstPath := filepath.Join(snapshotPath, "dump.sql.zst")
	if _, err := os.Stat(zstPath); err == nil {
		return s.mysqlRestoreFile(dbName, zstPath)
	}
	return s.mysqlRestoreFile(dbName, filepath.Join(snapshotPath, "dump.sql.gz"))
}

func (s *MySQLStore) mydumperSnapshot(dbName, snapshotDir string) error {
	cmd := exec.Command("mydumper",
		"--host", s.Config.Host,
		"--port", s.Config.Port,
		"--user", s.Config.User,
		"--password", s.Config.Pass,
		"--database", dbName,
		"--outputdir", snapshotDir,
		"--threads", "4",
		"--compress",
		"--lock-all-tables",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mydumper: %w", err)
	}
	return nil
}

func (s *MySQLStore) myloaderRestore(dbName, snapshotDir string) error {
	cmd := exec.Command("myloader",
		"--host", s.Config.Host,
		"--port", s.Config.Port,
		"--user", s.Config.User,
		"--password", s.Config.Pass,
		"--database", dbName,
		"--directory", snapshotDir,
		"--threads", "4",
		"--overwrite-tables",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("myloader: %w", err)
	}
	return nil
}

// mysqldumpSnapshot runs mysqldump inside the Docker container and pipes the
// output through zstd into a single file inside the snapshot dir.
func (s *MySQLStore) mysqldumpSnapshot(dbName, snapshotDir string) error {
	path := filepath.Join(snapshotDir, "dump.sql.zst")

	dump := dockerExec(s.Service,
		"mysqldump",
		"-u", s.Config.User, fmt.Sprintf("-p%s", s.Config.Pass),
		"-h", "127.0.0.1",
		"--single-transaction", dbName,
	)
	zstd := exec.Command("zstd", "-T0", "-q")

	var err error
	zstd.Stdin, err = dump.StdoutPipe()
	if err != nil {
		return fmt.Errorf("piping mysqldump to zstd: %w", err)
	}

	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating snapshot file: %w", err)
	}
	defer outFile.Close()
	zstd.Stdout = outFile
	zstd.Stderr = os.Stderr
	dump.Stderr = os.Stderr

	if err := zstd.Start(); err != nil {
		return fmt.Errorf("starting zstd: %w", err)
	}
	if err := dump.Run(); err != nil {
		return fmt.Errorf("running mysqldump: %w", err)
	}
	if err := zstd.Wait(); err != nil {
		return fmt.Errorf("zstd: %w", err)
	}
	return nil
}

// mysqlRestoreFile restores from a .sql, .sql.gz, or .sql.zst file by piping
// data into the mysql client running inside the Docker container.
func (s *MySQLStore) mysqlRestoreFile(dbName, path string) error {
	inFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening snapshot: %w", err)
	}
	defer inFile.Close()

	mysql := dockerExec(s.Service,
		"mysql",
		"-u", s.Config.User, fmt.Sprintf("-p%s", s.Config.Pass),
		"-h", "127.0.0.1",
		dbName,
	)
	mysql.Stdout = os.Stdout
	mysql.Stderr = os.Stderr

	switch {
	case strings.HasSuffix(path, ".zst"):
		decompress := exec.Command("zstd", "-d", "-q")
		decompress.Stdin = inFile
		decompress.Stderr = os.Stderr

		mysql.Stdin, err = decompress.StdoutPipe()
		if err != nil {
			return fmt.Errorf("piping zstd to mysql: %w", err)
		}
		if err := mysql.Start(); err != nil {
			return fmt.Errorf("starting mysql: %w", err)
		}
		if err := decompress.Run(); err != nil {
			return fmt.Errorf("running zstd decompress: %w", err)
		}
		if err := mysql.Wait(); err != nil {
			return fmt.Errorf("mysql restore: %w", err)
		}

	case strings.HasSuffix(path, ".gz"):
		gunzip := exec.Command("gunzip")
		gunzip.Stdin = inFile
		gunzip.Stderr = os.Stderr

		mysql.Stdin, err = gunzip.StdoutPipe()
		if err != nil {
			return fmt.Errorf("piping gunzip to mysql: %w", err)
		}
		if err := mysql.Start(); err != nil {
			return fmt.Errorf("starting mysql: %w", err)
		}
		if err := gunzip.Run(); err != nil {
			return fmt.Errorf("running gunzip: %w", err)
		}
		if err := mysql.Wait(); err != nil {
			return fmt.Errorf("mysql restore: %w", err)
		}

	default:
		mysql.Stdin = inFile
		if err := mysql.Run(); err != nil {
			return fmt.Errorf("mysql restore: %w", err)
		}
	}

	return nil
}

func (s *MySQLStore) exec(sql string) error {
	if err := s.waitForReady(); err != nil {
		return err
	}
	cmd := dockerExec(s.Service,
		"mysql",
		"-u", s.Config.User,
		fmt.Sprintf("-p%s", s.Config.Pass),
		"-h", "127.0.0.1",
		"-e", sql,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// waitForReady polls MySQL until it accepts connections.
func (s *MySQLStore) waitForReady() error {
	return docker.WaitForReady(s.Service, 120*time.Second, []string{
		"mysqladmin", "ping", "-h", "127.0.0.1",
		"-uroot", "-proot", "--ssl-mode=DISABLED",
	})
}
