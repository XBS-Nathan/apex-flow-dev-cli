package lifecycle

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/db"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

// DockerService manages shared and per-project Docker containers.
type DockerService interface {
	Up(phpVersions []string) error
	Down() error
	UpProject(projectName, projectDir string, services map[string]config.ServiceDefinition) error
	DownProject(projectName, projectDir string) error
}

// CaddyService manages the Caddy reverse proxy.
type CaddyService interface {
	Start() error
	Stop() error
	Link(siteName, projectDir, fpmSocket string) error
	Unlink(siteName string) error
}

// Lifecycle orchestrates starting and stopping a dev environment.
type Lifecycle struct {
	Docker DockerService
	Caddy  CaddyService
	Output func(format string, a ...any) // defaults to fmt.Printf
}

func (l *Lifecycle) printf(format string, a ...any) {
	if l.Output != nil {
		l.Output(format, a...)
	} else {
		fmt.Printf(format, a...)
	}
}

// Start brings up the full dev environment for a project.
func (l *Lifecycle) Start(p *project.Project, fpmSocket string) error {
	l.printf("Starting %s...\n", p.Name)

	l.printf("  → Starting shared services...\n")
	if err := l.Docker.Up([]string{p.Config.PHP}); err != nil {
		return fmt.Errorf("starting services: %w", err)
	}

	l.printf("  → Starting Caddy...\n")
	if err := l.Caddy.Start(); err != nil {
		return fmt.Errorf("starting caddy: %w", err)
	}

	l.printf("  → Linking site...\n")
	if err := l.Caddy.Link(p.Name, p.Dir, fpmSocket); err != nil {
		return fmt.Errorf("linking site: %w", err)
	}

	l.printf("  → Creating database %s (%s)...\n", p.Config.DB, p.Config.DBDriver)
	store, err := db.NewStore(p.Config.DBConfig())
	if err != nil {
		return err
	}
	if err := store.CreateIfNotExists(p.Config.DB); err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	if len(p.Config.Services) > 0 {
		l.printf("  → Starting project services...\n")
		if err := l.Docker.UpProject(p.Name, p.Dir, p.Config.Services); err != nil {
			return fmt.Errorf("starting project services: %w", err)
		}
	}

	for _, hook := range p.Config.Hooks.PostStart {
		l.printf("  → Running: %s\n", hook)
		c := exec.Command("bash", "-c", hook)
		c.Dir = p.Dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			l.printf("  ! hook failed: %s\n", err)
		}
	}

	l.printf("\n✓ %s is running at http://%s\n", p.Name, p.SiteDomain())
	l.printf("  PHP:      %s\n", p.Config.PHP)
	l.printf("  Database: %s\n", p.Config.DB)

	return nil
}

// Stop tears down the dev environment for a project.
func (l *Lifecycle) Stop(p *project.Project) error {
	l.printf("Stopping %s...\n", p.Name)

	for _, hook := range p.Config.Hooks.PostStop {
		l.printf("  → Running: %s\n", hook)
		c := exec.Command("bash", "-c", hook)
		c.Dir = p.Dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			l.printf("  ! hook failed: %s\n", err)
		}
	}

	l.printf("  → Stopping project services...\n")
	if err := l.Docker.DownProject(p.Name, p.Dir); err != nil {
		l.printf("  ! project services: %s\n", err)
	}

	killProjectProcesses(p.Dir)

	l.printf("  → Unlinking site...\n")
	if err := l.Caddy.Unlink(p.Name); err != nil {
		return fmt.Errorf("unlinking site: %w", err)
	}

	l.printf("✓ %s stopped\n", p.Name)
	return nil
}

// Down stops all shared services.
func (l *Lifecycle) Down() error {
	l.printf("Stopping everything...\n")

	l.printf("  → Stopping Caddy...\n")
	if err := l.Caddy.Stop(); err != nil {
		l.printf("  ! Caddy: %s\n", err)
	}

	l.printf("  → Stopping Docker services...\n")
	if err := l.Docker.Down(); err != nil {
		return fmt.Errorf("stopping services: %w", err)
	}

	l.printf("✓ All services stopped\n")
	return nil
}

func killProjectProcesses(projectDir string) {
	_ = exec.Command("pkill", "-f", projectDir).Run() // exits non-zero when no matches
}
