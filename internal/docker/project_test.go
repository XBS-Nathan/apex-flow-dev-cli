package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

func TestGenerateProjectCompose_Empty(t *testing.T) {
	result := GenerateProjectCompose(nil)
	if !strings.Contains(result, "services:") {
		t.Error("expected services key in output")
	}
}

func TestGenerateProjectCompose_SingleService(t *testing.T) {
	services := map[string]config.ServiceDefinition{
		"mssql": {
			Image: "mcr.microsoft.com/azure-sql-edge:latest",
			Ports: []string{"1439:1433"},
			Environment: map[string]string{
				"ACCEPT_EULA":      "Y",
				"MSSQL_SA_PASSWORD": "PASSword123@",
			},
		},
	}

	result := GenerateProjectCompose(services)

	checks := []string{
		"mssql:",
		"image: mcr.microsoft.com/azure-sql-edge:latest",
		"1439:1433",
		"ACCEPT_EULA:",
		"MSSQL_SA_PASSWORD:",
		"networks:",
		"dev:",
		"external: true",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected %q in output, got:\n%s", check, result)
		}
	}
}

func TestGenerateProjectCompose_MultipleServices(t *testing.T) {
	services := map[string]config.ServiceDefinition{
		"elasticsearch": {
			Image: "elasticsearch:8.12.0",
			Ports: []string{"9200:9200"},
			Environment: map[string]string{
				"discovery.type": "single-node",
			},
		},
		"meilisearch": {
			Image:   "getmeili/meilisearch:latest",
			Ports:   []string{"7700:7700"},
			Command: "meilisearch --no-analytics",
		},
	}

	result := GenerateProjectCompose(services)

	if !strings.Contains(result, "elasticsearch:") {
		t.Error("expected elasticsearch service")
	}
	if !strings.Contains(result, "meilisearch:") {
		t.Error("expected meilisearch service")
	}
	if !strings.Contains(result, "command: meilisearch --no-analytics") {
		t.Error("expected command for meilisearch")
	}

	// Verify deterministic ordering (elasticsearch before meilisearch)
	esIdx := strings.Index(result, "elasticsearch:")
	msIdx := strings.Index(result, "meilisearch:")
	if esIdx > msIdx {
		t.Error("expected alphabetical ordering: elasticsearch before meilisearch")
	}
}

func TestGenerateProjectCompose_WithVolumes(t *testing.T) {
	services := map[string]config.ServiceDefinition{
		"minio": {
			Image:   "minio/minio:latest",
			Ports:   []string{"9000:9000"},
			Volumes: []string{"minio_data:/data"},
			Command: "server /data",
		},
	}

	result := GenerateProjectCompose(services)

	if !strings.Contains(result, "volumes:") {
		t.Error("expected volumes key")
	}
	if !strings.Contains(result, "minio_data:/data") {
		t.Error("expected volume mount")
	}
}

func TestProjectComposeFile(t *testing.T) {
	dir := t.TempDir()
	path := ProjectComposeFile(dir)
	want := filepath.Join(dir, ".dev", "docker-compose.yml")
	if path != want {
		t.Errorf("ProjectComposeFile() = %q, want %q", path, want)
	}
}

func TestProjectDown_NoComposeFile(t *testing.T) {
	dir := t.TempDir()
	// Should not error when there's no compose file
	err := ProjectDown("test-project", dir)
	if err != nil {
		t.Errorf("ProjectDown() error = %v, want nil", err)
	}
}

func TestProjectUp_EmptyServices(t *testing.T) {
	dir := t.TempDir()
	// Should be a no-op with empty services
	err := ProjectUp("test-project", dir, nil)
	if err != nil {
		t.Errorf("ProjectUp() error = %v, want nil", err)
	}
	// Should not have created a compose file
	if _, err := os.Stat(ProjectComposeFile(dir)); err == nil {
		t.Error("expected no compose file for empty services")
	}
}
