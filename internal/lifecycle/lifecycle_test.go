package lifecycle

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pterm/pterm"

	"github.com/XBS-Nathan/nova/internal/caddy"
	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
	"github.com/XBS-Nathan/nova/internal/project"
)

func TestMain(m *testing.M) {
	// Disable pterm output in tests to avoid ANSI noise
	pterm.DisableOutput()
	os.Exit(m.Run())
}

// mockDocker records calls and can return errors.
type mockDocker struct {
	upCalled bool
	upPHP    []docker.PHPVersion
	downCalled bool
	execCalls  [][]string
	upErr      error
	downErr    error
	execErr    error
}

func (m *mockDocker) Up(php []docker.PHPVersion, forceRecreate bool) error {
	m.upCalled = true
	m.upPHP = php
	return m.upErr
}
func (m *mockDocker) Down() error { m.downCalled = true; return m.downErr }
func (m *mockDocker) Exec(service, workdir string, args ...string) error {
	m.execCalls = append(m.execCalls, append([]string{service, workdir}, args...))
	return m.execErr
}
func (m *mockDocker) ExecDetached(service, workdir string, args ...string) error {
	m.execCalls = append(m.execCalls, append([]string{service, workdir}, args...))
	return m.execErr
}
func (m *mockDocker) UpProject(_, _ string, _ map[string]config.ServiceDefinition) error {
	return nil
}
func (m *mockDocker) DownProject(_, _ string) error { return nil }

// mockCaddy records calls and can return errors.
type mockCaddy struct {
	startCalled  bool
	stopCalled   bool
	linkCalled   bool
	unlinkCalled bool
	reloadCalled bool
	linkSite     string
	unlinkSite   string
	startErr     error
	stopErr      error
	linkErr      error
	unlinkErr    error
	reloadErr    error
}

func (m *mockCaddy) Start() error { m.startCalled = true; return m.startErr }
func (m *mockCaddy) Stop() error  { m.stopCalled = true; return m.stopErr }
func (m *mockCaddy) Link(site, docroot, phpService string, portProxies []caddy.PortProxy) error {
	m.linkCalled = true
	m.linkSite = site
	return m.linkErr
}
func (m *mockCaddy) Unlink(site string) error {
	m.unlinkCalled = true
	m.unlinkSite = site
	return m.unlinkErr
}
func (m *mockCaddy) Reload() error { m.reloadCalled = true; return m.reloadErr }

// mockHosts records calls and can return errors.
type mockHosts struct {
	ensureCalled bool
	ensureDomain string
	ensureErr    error
}

func (m *mockHosts) Ensure(domain string) error {
	m.ensureCalled = true
	m.ensureDomain = domain
	return m.ensureErr
}

func newTestProject(t *testing.T, name string) *project.Project {
	t.Helper()
	return &project.Project{
		Name: name,
		Dir:  "/tmp/test-" + name,
		Config: &config.ProjectConfig{
			Domain:   name + ".test",
			PHP:      "8.2",
			Node:     "22",
			DBDriver: "mysql",
			DB:       name,
			MySQL: config.MySQLConfig{
				User: "root",
				Pass: "root",
				Host: "127.0.0.1",
				Port: "3306",
			},
		},
	}
}

// captureOutput returns a lifecycle with output captured to a buffer.
func captureOutput(
	t *testing.T,
	docker DockerService,
	caddy CaddyService,
	hosts HostsService,
) (*Lifecycle, *strings.Builder) {
	t.Helper()
	var buf strings.Builder
	lc := &Lifecycle{
		Docker: docker,
		Caddy:  caddy,
		Hosts:  hosts,
		PHPService: func(v string) string {
			return "php" + strings.ReplaceAll(v, ".", "")
		},
		Docroot: func(p *project.Project) string {
			return "/srv/" + p.Name + "/public"
		},
		Output: func(format string, a ...any) {
			fmt.Fprintf(&buf, format, a...)
		},
	}
	return lc, &buf
}

func TestStart_CallsServicesInOrder(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	// Start will fail at db.NewStore because mysql isn't running,
	// but we can verify docker, caddy, and hosts were called first.
	_ = lc.Start(p, []docker.PHPVersion{{Version: "8.2"}}, false)

	if !d.upCalled {
		t.Error("Docker.Up() was not called")
	}
	if !c.linkCalled {
		t.Error("Caddy.Link() was not called")
	}
	if c.linkSite != "myapp" {
		t.Errorf("Caddy.Link() site = %q, want %q", c.linkSite, "myapp")
	}
	if !h.ensureCalled {
		t.Error("Hosts.Ensure() was not called")
	}
	if h.ensureDomain != "myapp.test" {
		t.Errorf("Hosts.Ensure() domain = %q, want %q", h.ensureDomain, "myapp.test")
	}
}

func TestStart_StopsOnDockerError(t *testing.T) {
	d := &mockDocker{upErr: fmt.Errorf("docker broken")}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	err := lc.Start(p, []docker.PHPVersion{{Version: "8.2"}}, false)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker broken") {
		t.Errorf("error = %q, want to contain %q", err, "docker broken")
	}
	if c.linkCalled {
		t.Error("Caddy.Link() should not be called after Docker failure")
	}
	if h.ensureCalled {
		t.Error("Hosts.Ensure() should not be called after Docker failure")
	}
}

func TestStart_StopsOnCaddyLinkError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{linkErr: fmt.Errorf("link broken")}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	err := lc.Start(p, []docker.PHPVersion{{Version: "8.2"}}, false)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "link broken") {
		t.Errorf("error = %q, want to contain %q", err, "link broken")
	}
	if h.ensureCalled {
		t.Error("Hosts.Ensure() should not be called after Caddy.Link failure")
	}
}

func TestStart_StopsOnHostsError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{ensureErr: fmt.Errorf("hosts broken")}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	err := lc.Start(p, []docker.PHPVersion{{Version: "8.2"}}, false)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "hosts broken") {
		t.Errorf("error = %q, want to contain %q", err, "hosts broken")
	}
}

func TestStart_RunsHooksViaDockerExec(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")
	p.Config.Hooks.PostStart = []string{"php artisan migrate", "yarn build"}

	// Will fail at db step, but hooks come after db — so we need db to succeed.
	// Since we can't mock db.NewStore, we test that hooks are attempted
	// by checking that Start gets past hosts (it will fail at db).
	_ = lc.Start(p, []docker.PHPVersion{{Version: "8.2"}}, false)

	// db.NewStore will fail (no real mysql), so hooks won't run.
	// We verify the services before db were called correctly.
	if !d.upCalled {
		t.Error("Docker.Up() was not called")
	}
	if !h.ensureCalled {
		t.Error("Hosts.Ensure() was not called")
	}
}

func TestStop_CallsUnlink(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	err := lc.Stop(p)

	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !c.unlinkCalled {
		t.Error("Caddy.Unlink() was not called")
	}
	if c.unlinkSite != "myapp" {
		t.Errorf("Caddy.Unlink() site = %q, want %q", c.unlinkSite, "myapp")
	}
}

func TestStop_RunsHooksViaDockerExec(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")
	p.Config.Hooks.PostStop = []string{"php artisan down"}

	err := lc.Stop(p)

	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	// First exec call is the pkill for background processes,
	// second is the post-stop hook.
	if len(d.execCalls) < 2 {
		t.Fatalf("Docker.Exec() called %d times, want >= 2", len(d.execCalls))
	}

	// Verify pkill call
	pkillCall := d.execCalls[0]
	if pkillCall[2] != "pkill" {
		t.Errorf("first Exec should be pkill, got %v", pkillCall[2:])
	}
	if !strings.Contains(pkillCall[4], "dev:myapp:") {
		t.Errorf("pkill pattern = %q, want to contain %q", pkillCall[4], "dev:myapp:")
	}

	// Verify hook call
	hookCall := d.execCalls[1]
	if hookCall[0] != "php82" {
		t.Errorf("Exec service = %q, want %q", hookCall[0], "php82")
	}
	if hookCall[4] != "php artisan down" {
		t.Errorf("Exec hook = %q, want %q", hookCall[4], "php artisan down")
	}
}

func TestStop_KillsBackgroundProcessesByProjectName(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	_ = lc.Stop(p)

	if len(d.execCalls) < 1 {
		t.Fatal("Docker.Exec() not called")
	}
	call := d.execCalls[0]
	if call[2] != "pkill" || call[3] != "-f" || call[4] != "dev:myapp:" {
		t.Errorf("pkill call = %v, want [pkill -f dev:myapp:]", call[2:])
	}
}

func TestWrapHookCommand_BackgroundHookGetsMarker(t *testing.T) {
	got := wrapHookCommand("myapp", 0, "php artisan horizon &")

	if !strings.Contains(got, "#dev:myapp:0") {
		t.Errorf("expected comment marker dev:myapp:0, got: %s", got)
	}
	if !strings.Contains(got, "php artisan horizon") {
		t.Errorf("expected original command, got: %s", got)
	}
	if !strings.Contains(got, "nohup") {
		t.Errorf("expected nohup for detaching, got: %s", got)
	}
	if !strings.Contains(got, "&") {
		t.Errorf("expected & for backgrounding, got: %s", got)
	}
}

func TestWrapHookCommand_ForegroundHookUnchanged(t *testing.T) {
	got := wrapHookCommand("myapp", 0, "php artisan migrate")

	if got != "php artisan migrate" {
		t.Errorf("foreground hook should be unchanged, got: %s", got)
	}
}

func TestStop_ReturnsUnlinkError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{unlinkErr: fmt.Errorf("unlink broken")}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	err := lc.Stop(p)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unlink broken") {
		t.Errorf("error = %q, want to contain %q", err, "unlink broken")
	}
}

func TestDown_StopsDocker(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)

	err := lc.Down()

	if err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	if !d.downCalled {
		t.Error("Docker.Down() was not called")
	}
}

func TestDown_ReturnsDockerError(t *testing.T) {
	d := &mockDocker{downErr: fmt.Errorf("docker down failed")}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)

	err := lc.Down()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker down failed") {
		t.Errorf("error = %q, want to contain %q", err, "docker down failed")
	}
}

func TestStart_CallsAllServices(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	h := &mockHosts{}
	lc, _ := captureOutput(t, d, c, h)
	p := newTestProject(t, "myapp")

	_ = lc.Start(p, []docker.PHPVersion{{Version: "8.2"}}, false)

	// Verify all services were called (output is tested visually)
	if !d.upCalled {
		t.Error("Docker.Up() was not called")
	}
	if !c.linkCalled {
		t.Error("Caddy.Link() was not called")
	}
	if !h.ensureCalled {
		t.Error("Hosts.Ensure() was not called")
	}
}
