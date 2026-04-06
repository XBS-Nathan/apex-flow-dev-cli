package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"

	"github.com/XBS-Nathan/nova/internal/caddy"
	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/db"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
)

// DockerService manages shared and per-project Docker containers.
type DockerService interface {
	Up(php []docker.PHPVersion, forceRecreate bool) error
	Down() error
	Exec(service string, workdir string, args ...string) error
	ExecDetached(service string, workdir string, args ...string) error
	UpProject(projectName, projectDir string, services map[string]config.ServiceDefinition) error
	DownProject(projectName, projectDir string) error
}

// CaddyService manages the Caddy reverse proxy.
type CaddyService interface {
	Start() error
	Stop() error
	Link(siteName, docroot, phpService string, portProxies []caddy.PortProxy) error
	Unlink(siteName string) error
	Reload() error
}

// HostsService manages /etc/hosts entries.
type HostsService interface {
	Ensure(domain string) error
}

// Lifecycle orchestrates starting and stopping a dev environment.
type Lifecycle struct {
	Docker             DockerService
	Caddy              CaddyService
	Hosts              HostsService
	PHPService         func(version string) string
	DBServiceName      string
	ServiceVersions    config.ServiceVersions
	Docroot            func(p *project.Project) string
	NodeServiceBuilder func(p *project.Project) *config.ServiceDefinition
	Output             func(format string, a ...any)
}

func (l *Lifecycle) printf(format string, a ...any) {
	if l.Output != nil {
		l.Output(format, a...)
	} else {
		fmt.Printf(format, a...)
	}
}

// Start brings up the full dev environment for a project.
func (l *Lifecycle) Start(p *project.Project, php []docker.PHPVersion, forceRecreate bool) error {
	pterm.DefaultSection.Printfln("Starting %s", p.Name)

	phpSvc := l.PHPService(p.Config.PHP)
	docroot := l.Docroot(p)
	projectRoot := strings.TrimSuffix(docroot, "/public")

	if err := l.spin("Starting services", func() error {
		return l.Docker.Up(php, forceRecreate)
	}); err != nil {
		return fmt.Errorf("starting services: %w", err)
	}

	// Show running services with versions
	v := l.ServiceVersions
	services := []pterm.BulletListItem{
		{Level: 0, Text: pterm.LightCyan("PHP ") + pterm.Gray(p.Config.PHP)},
		{Level: 0, Text: pterm.LightCyan("Redis ") + pterm.Gray(v.Redis)},
		{Level: 0, Text: pterm.LightCyan("Mailpit ") + pterm.Gray(v.Mailpit)},
	}
	if p.Config.DBDriver == "postgres" {
		services = append(services, pterm.BulletListItem{
			Level: 0,
			Text:  pterm.LightCyan("PostgreSQL ") + pterm.Gray(v.Postgres),
		})
	} else {
		services = append(services, pterm.BulletListItem{
			Level: 0,
			Text:  pterm.LightCyan("MySQL ") + pterm.Gray(v.MySQL),
		})
	}
	pterm.DefaultBulletList.WithItems(services).Render()

	// Build port proxies — route extra ports to the node container
	nodeSvc := docker.NodeServiceName(p.Name)
	var portProxies []caddy.PortProxy
	for _, port := range p.Config.Ports {
		portProxies = append(portProxies, caddy.PortProxy{
			Port:    port,
			Backend: nodeSvc,
		})
	}

	if err := l.spin("Linking site", func() error {
		return l.Caddy.Link(p.Name, docroot, phpSvc, portProxies)
	}); err != nil {
		return fmt.Errorf("linking site: %w", err)
	}

	if err := l.spin("Adding hosts entry", func() error {
		return l.Hosts.Ensure(p.SiteDomain())
	}); err != nil {
		return fmt.Errorf("adding hosts entry: %w", err)
	}

	if err := l.spin("Creating database", func() error {
		store, err := db.NewStore(p.Config.DBConfig(), l.DBServiceName)
		if err != nil {
			return err
		}
		return store.CreateIfNotExists(p.Config.DB)
	}); err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	// Merge node service into project services if configured
	projectServices := make(map[string]config.ServiceDefinition)
	for k, v := range p.Config.Services {
		projectServices[k] = v
	}
	if l.NodeServiceBuilder != nil {
		if nodeDef := l.NodeServiceBuilder(p); nodeDef != nil {
			projectServices[nodeSvc] = *nodeDef
		}
	}

	if len(projectServices) > 0 {
		if err := l.spin("Starting project services", func() error {
			return l.Docker.UpProject(p.Name, p.Dir, projectServices)
		}); err != nil {
			return fmt.Errorf("starting project services: %w", err)
		}
	}

	if len(p.Config.Hooks.PostStart) > 0 {
		hookFailed := false
		for i, hook := range p.Config.Hooks.PostStart {
			named := wrapHookCommand(p.Name, i, hook)
			if err := l.Docker.ExecDetached(
				phpSvc, projectRoot, "sh", "-c", named,
			); err != nil {
				hookFailed = true
			}
		}
		if hookFailed {
			pterm.Warning.Println("Some hooks failed")
		} else {
			pterm.Success.Println("Hooks started")
		}
	}

	fmt.Println()
	pterm.DefaultBox.
		WithTitle(pterm.LightCyan(p.Name)).
		WithTitleTopCenter().
		WithBoxStyle(pterm.NewStyle(pterm.FgGray)).
		Printfln(
			"%s  %s\n%s  %s\n%s  %s",
			pterm.Gray("URL     "),
			pterm.LightCyan("https://"+p.SiteDomain()),
			pterm.Gray("PHP     "),
			pterm.White(p.Config.PHP),
			pterm.Gray("Database"),
			pterm.White(p.Config.DB+" ("+p.Config.DBDriver+")"),
		)

	// Hint to trust cert on first use
	certPath := filepath.Join(
		config.GlobalDir(), "caddy", "data",
		"caddy", "pki", "authorities", "local", "root.crt",
	)
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		fmt.Println()
		pterm.Info.Println(
			"HTTPS won't work until you trust the CA certificate.\n" +
				"  Run: " + pterm.LightCyan("dev trust"),
		)
	}

	return nil
}

// Stop tears down the dev environment for a project.
func (l *Lifecycle) Stop(p *project.Project) error {
	pterm.DefaultSection.Printfln("Stopping %s", p.Name)

	phpSvc := l.PHPService(p.Config.PHP)
	docroot := l.Docroot(p)
	projectRoot := strings.TrimSuffix(docroot, "/public")

	_ = l.spin("Stopping background processes", func() error {
		killPattern := hookPrefix(p.Name)
		_ = l.Docker.ExecDetached(
			phpSvc, projectRoot, "pkill", "-f", killPattern,
		)
		return nil
	})

	if len(p.Config.Hooks.PostStop) > 0 {
		_ = l.spin("Running stop hooks", func() error {
			for _, hook := range p.Config.Hooks.PostStop {
				_ = l.Docker.ExecDetached(
					phpSvc, projectRoot, "sh", "-c", hook,
				)
			}
			return nil
		})
	}

	_ = l.spin("Stopping project services", func() error {
		return l.Docker.DownProject(p.Name, p.Dir)
	})

	if err := l.spin("Unlinking site", func() error {
		return l.Caddy.Unlink(p.Name)
	}); err != nil {
		return fmt.Errorf("unlinking site: %w", err)
	}

	fmt.Println()
	pterm.Success.Printfln("%s stopped", p.Name)
	return nil
}

// Down stops all shared services.
func (l *Lifecycle) Down() error {
	pterm.DefaultSection.Printfln("Stopping all services")

	if err := l.spin("Stopping services", func() error {
		return l.Docker.Down()
	}); err != nil {
		return fmt.Errorf("stopping services: %w", err)
	}

	fmt.Println()
	pterm.Success.Println("All services stopped")
	return nil
}

// spin runs fn with a spinner, showing success or failure on completion.
// Completed steps stay visible with a ✓ or ✗ marker.
func (l *Lifecycle) spin(msg string, fn func() error) error {
	spinner, _ := pterm.DefaultSpinner.
		WithRemoveWhenDone(false).
		Start(msg + "...")

	err := fn()
	if err != nil {
		spinner.Fail(msg)
	} else {
		spinner.Success(msg)
	}
	return err
}

// hookPrefix returns the naming prefix for a project's background processes.
func hookPrefix(projectName string) string {
	return "dev:" + projectName + ":"
}

// wrapHookCommand wraps a background hook command so it can be identified
// and killed by project name on dev stop. Foreground hooks run as-is.
func wrapHookCommand(projectName string, index int, hook string) string {
	hook = strings.TrimSpace(hook)
	if !strings.HasSuffix(hook, "&") {
		return hook
	}

	cmd := strings.TrimSpace(strings.TrimSuffix(hook, "&"))
	marker := fmt.Sprintf("%s%d", hookPrefix(projectName), index)

	// nohup + redirect ensures docker exec returns immediately
	return fmt.Sprintf("nohup %s > /dev/null 2>&1 & #%s", cmd, marker)
}
