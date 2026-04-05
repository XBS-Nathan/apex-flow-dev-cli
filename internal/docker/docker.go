package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

// ComposeFile returns the path to the shared docker-compose.yml.
func ComposeFile() string {
	return filepath.Join(config.GlobalDir(), "docker-compose.yml")
}

// EnsureComposeFile writes the default docker-compose.yml if it doesn't exist.
func EnsureComposeFile() error {
	path := ComposeFile()
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	return os.WriteFile(path, []byte(defaultCompose), 0644)
}

// Up starts shared Docker services.
func Up() error {
	if err := EnsureComposeFile(); err != nil {
		return err
	}
	return compose("up", "-d")
}

// Down stops shared Docker services.
func Down() error {
	return compose("down")
}

// IsUp checks if shared services are running.
func IsUp() bool {
	cmd := exec.Command("docker", "compose", "-f", ComposeFile(), "ps", "--status", "running", "-q")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
}

func compose(args ...string) error {
	fullArgs := append([]string{"compose", "-f", ComposeFile()}, args...)
	cmd := exec.Command("docker", fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w", args[0], err)
	}
	return nil
}

const defaultCompose = `services:
  mysql:
    image: mysql:8.0
    restart: unless-stopped
    ports:
      - "3306:3306"
    environment:
      MYSQL_ROOT_PASSWORD: root
    volumes:
      - mysql_data:/var/lib/mysql

  redis:
    image: redis:8
    restart: unless-stopped
    ports:
      - "6379:6379"
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - redis_data:/data

  typesense:
    image: typesense/typesense:26.0
    restart: unless-stopped
    ports:
      - "8108:8108"
    environment:
      TYPESENSE_API_KEY: dev
    command: "--data-dir /data --enable-cors"
    volumes:
      - typesense_data:/data

  docuseal:
    image: docuseal/docuseal:latest
    restart: unless-stopped
    ports:
      - "3000:3000"
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/docuseal
    volumes:
      - docuseal_data:/data/docuseal

  postgres:
    image: postgres:15
    restart: unless-stopped
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: docuseal
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  mysql_data:
  redis_data:
  typesense_data:
  docuseal_data:
  postgres_data:
`
