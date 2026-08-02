package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/project"
)

func TestNodeServiceForProject(t *testing.T) {
	t.Parallel()

	global := &config.GlobalConfig{
		ProjectsDir: "/home/user/projects",
	}

	tests := []struct {
		name       string
		project    *project.Project
		wantNil    bool
		wantImage  string
		checkCmd   func(t *testing.T, cmd string)
		checkEnv   func(t *testing.T, env map[string]string)
	}{
		{
			name: "nil when NodeCommand is empty",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand: "",
					Node:        "20",
				},
			},
			wantNil: true,
		},
		{
			name: "pnpm enables corepack in a writable install dir",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "pnpm dev",
					Node:           "20",
					PackageManager: "pnpm",
				},
			},
			wantImage: "node:20-alpine",
			checkCmd: func(t *testing.T, cmd string) {
				t.Helper()
				if !strings.Contains(cmd, "corepack enable --install-directory /tmp/corepack-bin pnpm") {
					t.Errorf("command should enable pnpm into /tmp/corepack-bin, got: %s", cmd)
				}
				if !strings.Contains(cmd, "export PATH=/tmp/corepack-bin:$PATH") {
					t.Errorf("command should prepend /tmp/corepack-bin to PATH, got: %s", cmd)
				}
				if !strings.Contains(cmd, "pnpm dev") {
					t.Errorf("command should contain 'pnpm dev', got: %s", cmd)
				}
			},
		},
		{
			name: "yarn enables corepack in a writable install dir",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "yarn dev",
					Node:           "18",
					PackageManager: "yarn",
				},
			},
			wantImage: "node:18-alpine",
			checkCmd: func(t *testing.T, cmd string) {
				t.Helper()
				if !strings.Contains(cmd, "corepack enable --install-directory /tmp/corepack-bin yarn") {
					t.Errorf("command should enable yarn into /tmp/corepack-bin, got: %s", cmd)
				}
				if !strings.Contains(cmd, "export PATH=/tmp/corepack-bin:$PATH") {
					t.Errorf("command should prepend /tmp/corepack-bin to PATH, got: %s", cmd)
				}
				if !strings.Contains(cmd, "yarn dev") {
					t.Errorf("command should contain 'yarn dev', got: %s", cmd)
				}
			},
		},
		{
			name: "npm has no corepack in command",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "npm run dev",
					Node:           "22",
					PackageManager: "npm",
				},
			},
			wantImage: "node:22-alpine",
			checkCmd: func(t *testing.T, cmd string) {
				t.Helper()
				if strings.Contains(cmd, "corepack") {
					t.Errorf("command should not contain 'corepack' for npm, got: %s", cmd)
				}
				if !strings.Contains(cmd, "npm run dev") {
					t.Errorf("command should contain 'npm run dev', got: %s", cmd)
				}
			},
		},
		{
			name: "image uses correct node version",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "npm start",
					Node:           "21",
					PackageManager: "npm",
				},
			},
			wantImage: "node:21-alpine",
		},
		{
			name: "environment vars include NOVA and NODE_ENV",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "npm start",
					Node:           "20",
					PackageManager: "npm",
				},
			},
			wantImage: "node:20-alpine",
			checkEnv: func(t *testing.T, env map[string]string) {
				t.Helper()
				if env["NOVA"] != "true" {
					t.Errorf("NOVA env = %q, want %q", env["NOVA"], "true")
				}
				if env["NODE_ENV"] != "development" {
					t.Errorf("NODE_ENV env = %q, want %q", env["NODE_ENV"], "development")
				}
			},
		},
		{
			name: "volumes mount projects dir to /srv",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "npm start",
					Node:           "20",
					PackageManager: "npm",
				},
			},
			wantImage: "node:20-alpine",
		},
		{
			name: "command includes cd to correct workdir",
			project: &project.Project{
				Name: "myapp",
				Dir:  "/home/user/projects/myapp",
				Config: &config.ProjectConfig{
					NodeCommand:    "npm start",
					Node:           "20",
					PackageManager: "npm",
				},
			},
			wantImage: "node:20-alpine",
			checkCmd: func(t *testing.T, cmd string) {
				t.Helper()
				if !strings.Contains(cmd, "cd /srv/myapp") {
					t.Errorf("command should contain 'cd /srv/myapp', got: %s", cmd)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := nodeServiceForProject(tt.project, global)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil ServiceDefinition, got nil")
			}

			if tt.wantImage != "" && got.Image != tt.wantImage {
				t.Errorf("image = %q, want %q", got.Image, tt.wantImage)
			}

			if tt.checkCmd != nil {
				tt.checkCmd(t, got.Command)
			}

			if tt.checkEnv != nil {
				tt.checkEnv(t, got.Environment)
			}

			// All non-nil results should mount projects dir to /srv.
			wantVolume := "/home/user/projects:/srv"
			if len(got.Volumes) == 0 {
				t.Fatal("expected at least one volume, got none")
			}
			if got.Volumes[0] != wantVolume {
				t.Errorf("volume = %q, want %q", got.Volumes[0], wantVolume)
			}

			// All non-nil results run as the host user so files written to
			// the bind mount stay owned by the host user, with HOME and
			// COREPACK_HOME pointed at writable locations.
			wantUser := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
			if got.User != wantUser {
				t.Errorf("user = %q, want %q", got.User, wantUser)
			}
			if got.Environment["HOME"] != "/tmp" {
				t.Errorf("HOME env = %q, want %q", got.Environment["HOME"], "/tmp")
			}
			if got.Environment["COREPACK_HOME"] != "/tmp/corepack" {
				t.Errorf("COREPACK_HOME env = %q, want %q",
					got.Environment["COREPACK_HOME"], "/tmp/corepack")
			}
		})
	}
}

func TestWorkerServicesForProject_RunsAsHostUser(t *testing.T) {
	t.Parallel()

	global := &config.GlobalConfig{
		ProjectsDir: "/home/user/projects",
	}
	p := &project.Project{
		Name: "myapp",
		Dir:  "/home/user/projects/myapp",
		Config: &config.ProjectConfig{
			PHP: "8.4",
			Workers: map[string]string{
				"horizon": "php artisan horizon",
			},
		},
	}

	services := workerServicesForProject(p, global)

	svc, ok := services["horizon-myapp"]
	if !ok {
		t.Fatalf("expected horizon-myapp service, got %v", services)
	}

	wantUser := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	if svc.User != wantUser {
		t.Errorf("user = %q, want %q", svc.User, wantUser)
	}
}
