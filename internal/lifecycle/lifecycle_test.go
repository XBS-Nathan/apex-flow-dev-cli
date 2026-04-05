package lifecycle

import (
	"fmt"
	"strings"
	"testing"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

// mockDocker records calls and can return errors.
type mockDocker struct {
	upCalled   bool
	downCalled bool
	upErr      error
	downErr    error
}

func (m *mockDocker) Up(_ []string) error { m.upCalled = true; return m.upErr }
func (m *mockDocker) Down() error         { m.downCalled = true; return m.downErr }
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
	linkSite     string
	unlinkSite   string
	startErr     error
	stopErr      error
	linkErr      error
	unlinkErr    error
}

func (m *mockCaddy) Start() error { m.startCalled = true; return m.startErr }
func (m *mockCaddy) Stop() error  { m.stopCalled = true; return m.stopErr }
func (m *mockCaddy) Link(site, dir, socket string) error {
	m.linkCalled = true
	m.linkSite = site
	return m.linkErr
}
func (m *mockCaddy) Unlink(site string) error {
	m.unlinkCalled = true
	m.unlinkSite = site
	return m.unlinkErr
}

func newTestProject(name string) *project.Project {
	return &project.Project{
		Name: name,
		Dir:  "/tmp/test-" + name,
		Config: &config.ProjectConfig{
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
	docker DockerService,
	caddy CaddyService,
) (*Lifecycle, *strings.Builder) {
	var buf strings.Builder
	lc := &Lifecycle{
		Docker: docker,
		Caddy:  caddy,
		Output: func(format string, a ...any) {
			fmt.Fprintf(&buf, format, a...)
		},
	}
	return lc, &buf
}

func TestStart_CallsServicesInOrder(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	lc, _ := captureOutput(d, c)
	p := newTestProject("myapp")

	// Start will fail at db.NewStore because mysql isn't running,
	// but we can verify docker and caddy were called first.
	// To test the full flow we'd need a real/mock db — test up to caddy link.
	_ = lc.Start(p, "/run/php/php8.2-fpm.sock")

	if !d.upCalled {
		t.Error("Docker.Up() was not called")
	}
	if !c.startCalled {
		t.Error("Caddy.Start() was not called")
	}
	if !c.linkCalled {
		t.Error("Caddy.Link() was not called")
	}
	if c.linkSite != "myapp" {
		t.Errorf("Caddy.Link() site = %q, want %q", c.linkSite, "myapp")
	}
}

func TestStart_StopsOnDockerError(t *testing.T) {
	d := &mockDocker{upErr: fmt.Errorf("docker broken")}
	c := &mockCaddy{}
	lc, _ := captureOutput(d, c)
	p := newTestProject("myapp")

	err := lc.Start(p, "/run/php/php8.2-fpm.sock")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker broken") {
		t.Errorf("error = %q, want to contain %q", err, "docker broken")
	}
	if c.startCalled {
		t.Error("Caddy.Start() should not be called after Docker failure")
	}
}

func TestStart_StopsOnCaddyStartError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{startErr: fmt.Errorf("caddy broken")}
	lc, _ := captureOutput(d, c)
	p := newTestProject("myapp")

	err := lc.Start(p, "/run/php/php8.2-fpm.sock")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "caddy broken") {
		t.Errorf("error = %q, want to contain %q", err, "caddy broken")
	}
	if c.linkCalled {
		t.Error("Caddy.Link() should not be called after Start failure")
	}
}

func TestStart_StopsOnCaddyLinkError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{linkErr: fmt.Errorf("link broken")}
	lc, _ := captureOutput(d, c)
	p := newTestProject("myapp")

	err := lc.Start(p, "/run/php/php8.2-fpm.sock")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "link broken") {
		t.Errorf("error = %q, want to contain %q", err, "link broken")
	}
}

func TestStop_CallsUnlink(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	lc, _ := captureOutput(d, c)
	p := newTestProject("myapp")

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

func TestStop_ReturnsUnlinkError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{unlinkErr: fmt.Errorf("unlink broken")}
	lc, _ := captureOutput(d, c)
	p := newTestProject("myapp")

	err := lc.Stop(p)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unlink broken") {
		t.Errorf("error = %q, want to contain %q", err, "unlink broken")
	}
}

func TestDown_StopsCaddyAndDocker(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	lc, _ := captureOutput(d, c)

	err := lc.Down()

	if err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	if !c.stopCalled {
		t.Error("Caddy.Stop() was not called")
	}
	if !d.downCalled {
		t.Error("Docker.Down() was not called")
	}
}

func TestDown_ContinuesAfterCaddyError(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{stopErr: fmt.Errorf("caddy stop failed")}
	lc, buf := captureOutput(d, c)

	err := lc.Down()

	if err != nil {
		t.Fatalf("Down() should not fail on Caddy error, got: %v", err)
	}
	if !d.downCalled {
		t.Error("Docker.Down() should still be called after Caddy failure")
	}
	if !strings.Contains(buf.String(), "caddy stop failed") {
		t.Error("Caddy error should be printed as warning")
	}
}

func TestDown_ReturnsDockerError(t *testing.T) {
	d := &mockDocker{downErr: fmt.Errorf("docker down failed")}
	c := &mockCaddy{}
	lc, _ := captureOutput(d, c)

	err := lc.Down()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker down failed") {
		t.Errorf("error = %q, want to contain %q", err, "docker down failed")
	}
}

func TestStart_OutputIncludesProjectInfo(t *testing.T) {
	d := &mockDocker{}
	c := &mockCaddy{}
	lc, buf := captureOutput(d, c)
	p := newTestProject("myapp")

	_ = lc.Start(p, "/run/php/php8.2-fpm.sock")

	output := buf.String()
	if !strings.Contains(output, "Starting myapp") {
		t.Error("output should contain project name")
	}
	if !strings.Contains(output, "Starting shared services") {
		t.Error("output should mention shared services step")
	}
}
