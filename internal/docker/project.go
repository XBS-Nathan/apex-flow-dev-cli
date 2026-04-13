package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/XBS-Nathan/nova/internal/config"
)

// ProjectComposeDir returns the .nova directory inside a project, creating it if needed.
func ProjectComposeDir(projectDir string) string {
	dir := filepath.Join(projectDir, ".nova")
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
		b.WriteString("      - nova\n")
		b.WriteString("\n")
	}

	// Declare named volumes (volumes that don't contain a path separator)
	var namedVolumes []string
	for _, name := range names {
		for _, v := range services[name].Volumes {
			// Named volumes look like "vol_name:/path" (no / before the colon)
			parts := strings.SplitN(v, ":", 2)
			if len(parts) == 2 && !strings.Contains(parts[0], "/") {
				namedVolumes = append(namedVolumes, parts[0])
			}
		}
	}
	if len(namedVolumes) > 0 {
		b.WriteString("volumes:\n")
		for _, v := range namedVolumes {
			fmt.Fprintf(&b, "  %s:\n", v)
		}
		b.WriteString("\n")
	}

	b.WriteString("networks:\n")
	b.WriteString("  nova:\n")
	b.WriteString("    external: true\n")

	return b.String()
}

// ProjectLogs streams logs for a service in a project's compose file.
func ProjectLogs(projectName, projectDir, service string) error {
	composeFile := ProjectComposeFile(projectDir)
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("no project services running")
	}

	args := []string{
		"compose",
		"-f", composeFile,
		"-p", fmt.Sprintf("nova-%s", projectName),
		"logs", "-f", "--tail", "100",
	}
	if service != "" {
		args = append(args, service)
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose logs: %w", err)
	}
	return nil
}

// ProjectServices returns the list of service names in a project's compose file.
func ProjectServices(projectName, projectDir string) []string {
	composeFile := ProjectComposeFile(projectDir)
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("docker", "compose",
		"-f", composeFile,
		"-p", fmt.Sprintf("nova-%s", projectName),
		"config", "--services",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var services []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			services = append(services, line)
		}
	}
	return services
}

// NodeServiceName returns the compose service name for a project's node container.
func NodeServiceName(projectName string) string {
	return "node-" + projectName
}

func projectCompose(projectName, projectDir string, args ...string) error {
	composeFile := ProjectComposeFile(projectDir)
	fullArgs := append([]string{
		"compose",
		"-f", composeFile,
		"-p", fmt.Sprintf("nova-%s", projectName),
	}, args...)

	cmd := exec.Command("docker", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose (project) %s: %s: %w",
			args[0], strings.TrimSpace(string(output)), err)
	}
	return nil
}
