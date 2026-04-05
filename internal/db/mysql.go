package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

// MySQLStore implements Store using mydumper/myloader for fast parallel snapshots
// and the mysql CLI for database management.
type MySQLStore struct {
	Config config.MySQLConfig
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
//   - .sql.gz file: gunzip | mysql
//   - .sql file: mysql < file
//   - directory: myloader (or fallback to dump.sql.gz inside the dir)
func (s *MySQLStore) Restore(dbName, snapshotPath string) error {
	if IsFileSnapshot(snapshotPath) {
		return s.mysqlRestoreFile(dbName, snapshotPath)
	}
	if _, err := exec.LookPath("myloader"); err == nil {
		return s.myloaderRestore(dbName, snapshotPath)
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

// mysqldumpSnapshot is the fallback using mysqldump + gzip into a single
// file inside the snapshot dir.
func (s *MySQLStore) mysqldumpSnapshot(dbName, snapshotDir string) error {
	path := filepath.Join(snapshotDir, "dump.sql.gz")

	dump := exec.Command("mysqldump",
		"-u", s.Config.User, fmt.Sprintf("-p%s", s.Config.Pass),
		"-h", s.Config.Host, "-P", s.Config.Port,
		"--single-transaction", dbName,
	)
	gzip := exec.Command("gzip")

	var err error
	gzip.Stdin, err = dump.StdoutPipe()
	if err != nil {
		return fmt.Errorf("piping mysqldump to gzip: %w", err)
	}

	outFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating snapshot file: %w", err)
	}
	defer outFile.Close()
	gzip.Stdout = outFile
	gzip.Stderr = os.Stderr
	dump.Stderr = os.Stderr

	if err := gzip.Start(); err != nil {
		return fmt.Errorf("starting gzip: %w", err)
	}
	if err := dump.Run(); err != nil {
		return fmt.Errorf("running mysqldump: %w", err)
	}
	if err := gzip.Wait(); err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	return nil
}

// mysqlRestoreFile restores from a .sql or .sql.gz file.
func (s *MySQLStore) mysqlRestoreFile(dbName, path string) error {
	inFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening snapshot: %w", err)
	}
	defer inFile.Close()

	mysql := exec.Command("mysql",
		"-u", s.Config.User, fmt.Sprintf("-p%s", s.Config.Pass),
		"-h", s.Config.Host, "-P", s.Config.Port,
		dbName,
	)
	mysql.Stdout = os.Stdout
	mysql.Stderr = os.Stderr

	if strings.HasSuffix(path, ".gz") {
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
	} else {
		mysql.Stdin = inFile
		if err := mysql.Run(); err != nil {
			return fmt.Errorf("mysql restore: %w", err)
		}
	}

	return nil
}

func (s *MySQLStore) exec(sql string) error {
	cmd := exec.Command("mysql",
		"-u", s.Config.User, fmt.Sprintf("-p%s", s.Config.Pass),
		"-h", s.Config.Host, "-P", s.Config.Port,
		"-e", sql,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
