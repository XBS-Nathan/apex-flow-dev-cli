package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
)

// ProjectComposeDir returns the .dev directory inside a project, creating it if needed.
func ProjectComposeDir(projectDir string) string {
	dir := filepath.Join(projectDir, ".dev")
	os.MkdirAll(dir, 0755)
	return dir
}

// ProjectComposeFile returns the path to a project's docker-compose.yml.
func ProjectComposeFile(projectDir string) string {
	return filepath.Join(ProjectComposeDir(projectDir), "docker-compose.yml")
}

// ProjectUp generates a per-project docker-compose.yml and starts the services.
func ProjectUp(projectName, projectDir string, services map[string]config.ServiceDefinition) error {
	if len(services) == 0 {
		return nil
	}

	content := GenerateProjectCompose(services)
	path := ProjectComposeFile(projectDir)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing project compose file: %w", err)
	}

	return projectCompose(projectName, projectDir, "up", "-d")
}

// ProjectDown stops per-project Docker services.
func ProjectDown(projectName, projectDir string) error {
	path := ProjectComposeFile(projectDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // no project services to stop
	}

	return projectCompose(projectName, projectDir, "down")
}

// GenerateProjectCompose builds a docker-compose.yml string from service definitions.
func GenerateProjectCompose(services map[string]config.ServiceDefinition) string {
	var b strings.Builder

	b.WriteString("services:\n")

	// Sort keys for deterministic output
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		svc := services[name]
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", svc.Image)
		b.WriteString("    restart: unless-stopped\n")

		if len(svc.Ports) > 0 {
			b.WriteString("    ports:\n")
			for _, p := range svc.Ports {
				fmt.Fprintf(&b, "      - %q\n", p)
			}
		}

		if len(svc.Environment) > 0 {
			b.WriteString("    environment:\n")
			// Sort env keys for deterministic output
			envKeys := make([]string, 0, len(svc.Environment))
			for k := range svc.Environment {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			for _, k := range envKeys {
				fmt.Fprintf(&b, "      %s: %q\n", k, svc.Environment[k])
			}
		}

		if len(svc.Volumes) > 0 {
			b.WriteString("    volumes:\n")
			for _, v := range svc.Volumes {
				fmt.Fprintf(&b, "      - %s\n", v)
			}
		}

		if svc.Command != "" {
			fmt.Fprintf(&b, "    command: %s\n", svc.Command)
		}

		b.WriteString("    networks:\n")
		b.WriteString("      - dev\n")
		b.WriteString("\n")
	}

	b.WriteString("networks:\n")
	b.WriteString("  dev:\n")
	b.WriteString("    external: true\n")

	return b.String()
}

func projectCompose(projectName, projectDir string, args ...string) error {
	composeFile := ProjectComposeFile(projectDir)
	fullArgs := append([]string{
		"compose",
		"-f", composeFile,
		"-p", fmt.Sprintf("dev-%s", projectName),
	}, args...)

	cmd := exec.Command("docker", fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose (project) %s: %w", args[0], err)
	}
	return nil
}
