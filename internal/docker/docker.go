package docker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/phpimage"
)

// ComposeFile returns the path to the shared docker-compose.yml.
func ComposeFile() string {
	return filepath.Join(config.GlobalDir(), "docker-compose.yml")
}

// PHPServiceName converts a PHP version like "8.2" to a service name like "php82".
func PHPServiceName(version string) string {
	return "php" + strings.ReplaceAll(version, ".", "")
}

// ServiceName returns a compose service name. If there's only one version,
// returns the simple name (e.g. "mysql"). With multiple versions, appends
// the version with an underscore (e.g. "mysql_80").
func ServiceName(svcType, version string, total int) string {
	if total <= 1 {
		return svcType
	}
	return svcType + "_" + strings.ReplaceAll(version, ".", "")
}

// PHPVersion pairs a PHP version with its config for image tag resolution.
type PHPVersion struct {
	Version    string
	Extensions []string
	Ports      []string // extra ports to forward via Caddy, e.g. ["8080"]
}

// ComposeOptions controls which services are included in the compose file.
type ComposeOptions struct {
	ProjectsDir   string
	PHP           []PHPVersion
	MySQLVersions    []string // e.g. ["8.0", "9.0"]
	PostgresVersions []string // e.g. ["15", "16"]
	RedisVersions    []string // e.g. ["7", "8"]
	MailpitVersion string
	SharedServices map[string]config.ServiceDefinition
	ForceRecreate  bool
}

// Up generates the compose file and starts services.
func Up(opts ComposeOptions) error {
	content := generateCompose(opts)
	if err := os.WriteFile(ComposeFile(), []byte(content), 0644); err != nil {
		return fmt.Errorf("writing compose file: %w", err)
	}
	args := []string{"up", "-d", "--remove-orphans"}
	if opts.ForceRecreate {
		args = append(args, "--force-recreate")
	}
	return composeQuiet(args...)
}

// Pull re-downloads the latest images for shared services.
func Pull() error {
	return composeQuiet("pull")
}

// WaitForReady polls a service with a check command until it succeeds.
func WaitForReady(service string, timeout time.Duration, check []string) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		args := append([]string{"compose", "-f", ComposeFile(), "exec", "-T", service}, check...)
		cmd := exec.Command("docker", args...)
		if cmd.Run() == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("%s not ready after %s", service, timeout)
}

// Logs streams container logs. If service is empty, shows all containers.
// Follows logs in real-time.
func Logs(service string) error {
	args := []string{"logs", "-f", "--tail", "100"}
	if service != "" {
		args = append(args, service)
	}
	return compose(args...)
}

// Down stops shared Docker services.
func Down() error {
	return composeQuiet("down")
}

// Exec runs an interactive command in a running service container (with TTY).
func Exec(service, workdir string, args ...string) error {
	execArgs := append([]string{"compose", "-f", ComposeFile(), "exec", "-w", workdir, service}, args...)
	cmd := exec.Command("docker", execArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// If the command inside the container failed, the user already
		// saw the error output. Exit silently with the same code.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("docker compose exec: %w", err)
	}
	return nil
}

// ExecDetached runs a non-interactive command in a container (no TTY).
// Used for hooks and background processes.
func ExecDetached(service, workdir string, args ...string) error {
	execArgs := append([]string{"exec", "-T", "-w", workdir, service}, args...)
	return compose(execArgs...)
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

// composeQuiet runs compose with output suppressed.
// On error, returns the captured stderr for diagnostics.
func composeQuiet(args ...string) error {
	fullArgs := append([]string{"compose", "-f", ComposeFile()}, args...)
	cmd := exec.Command("docker", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose %s: %s: %w",
			args[0], strings.TrimSpace(string(output)), err)
	}
	return nil
}

func generateCompose(opts ComposeOptions) string {
	globalDir := config.GlobalDir()
	var b strings.Builder
	var volumes []string

	b.WriteString("services:\n")

	// Collect extra ports from all PHP services for Caddy
	extraPorts := make(map[string]bool)
	for _, php := range opts.PHP {
		for _, port := range php.Ports {
			extraPorts[port] = true
		}
	}

	// Caddy (always included)
	b.WriteString("  caddy:\n")
	b.WriteString("    image: caddy:2-alpine\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"80:80\"\n")
	b.WriteString("      - \"443:443\"\n")
	// Extra ports are served via Caddy for SSL termination
	for port := range extraPorts {
		fmt.Fprintf(&b, "      - \"%s:%s\"\n", port, port)
	}
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s/caddy:/etc/caddy\n", globalDir)
	fmt.Fprintf(&b, "      - %s/caddy/data:/data\n", globalDir)
	fmt.Fprintf(&b, "      - %s:/srv\n", opts.ProjectsDir)
	b.WriteString("    networks: [nova]\n\n")

	// PHP (one per version)
	for _, php := range opts.PHP {
		name := PHPServiceName(php.Version)
		image := phpimage.ImageTag(phpimage.ImageConfig{
			PHPVersion: php.Version,
			Extensions: php.Extensions,
		})
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", image)
		b.WriteString("    pull_policy: never\n")
		fmt.Fprintf(&b, "    user: \"%d:%d\"\n", os.Getuid(), os.Getgid())
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    environment:\n")
		b.WriteString("      NOVA: \"true\"\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/srv\n", opts.ProjectsDir)
		fmt.Fprintf(&b, "      - %s/php/%s/conf.d:/usr/local/etc/php/conf.custom\n",
			globalDir, php.Version)
		b.WriteString("    networks: [nova]\n\n")
	}

	// MySQL (one per version)
	for i, ver := range opts.MySQLVersions {
		name := ServiceName("mysql", ver, len(opts.MySQLVersions))
		// Always include version in volume name to prevent data corruption on version change
		volName := "mysql_" + strings.ReplaceAll(ver, ".", "") + "_data"
		hostPort := 3306 + i
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: mysql:%s\n", ver)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"%d:3306\"\n", hostPort)
		b.WriteString("    command: --default-authentication-plugin=mysql_native_password --tls-version=''\n")
		b.WriteString("    environment:\n")
		b.WriteString("      MYSQL_ROOT_PASSWORD: root\n")
		b.WriteString("      MYSQL_ROOT_HOST: '%'\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/var/lib/mysql\n", volName)
		fmt.Fprintf(&b, "      - %s/mysql/conf.d:/etc/mysql/conf.d\n", globalDir)
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD\", \"mysqladmin\", \"ping\", \"-h\", \"127.0.0.1\", \"-uroot\", \"-proot\", \"--ssl-mode=DISABLED\"]\n")
		b.WriteString("      interval: 2s\n")
		b.WriteString("      timeout: 5s\n")
		b.WriteString("      retries: 60\n")
		b.WriteString("      start_period: 10s\n")
		b.WriteString("    networks: [nova]\n\n")
		volumes = append(volumes, volName)
	}

	// Postgres (one per version)
	for i, ver := range opts.PostgresVersions {
		name := ServiceName("postgres", ver, len(opts.PostgresVersions))
		volName := "postgres_" + strings.ReplaceAll(ver, ".", "") + "_data"
		hostPort := 5432 + i
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: postgres:%s\n", ver)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"%d:5432\"\n", hostPort)
		b.WriteString("    environment:\n")
		b.WriteString("      POSTGRES_USER: postgres\n")
		b.WriteString("      POSTGRES_PASSWORD: postgres\n")
		b.WriteString("      POSTGRES_DB: postgres\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/var/lib/postgresql/data\n", volName)
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"pg_isready -U postgres\"]\n")
		b.WriteString("      interval: 2s\n")
		b.WriteString("      timeout: 5s\n")
		b.WriteString("      retries: 60\n")
		b.WriteString("      start_period: 10s\n")
		b.WriteString("    networks: [nova]\n\n")
		volumes = append(volumes, volName)
	}

	// Redis (one per version)
	for i, ver := range opts.RedisVersions {
		name := ServiceName("redis", ver, len(opts.RedisVersions))
		volName := name + "_data"
		hostPort := 6379 + i
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: redis:%s\n", ver)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"%d:6379\"\n", hostPort)
		b.WriteString("    command: redis-server --appendonly yes\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/data\n", volName)
		b.WriteString("    networks: [nova]\n\n")
		volumes = append(volumes, volName)
	}

	// Mailpit (always included)
	fmt.Fprintf(&b, "  mailpit:\n")
	fmt.Fprintf(&b, "    image: axllent/mailpit:%s\n", opts.MailpitVersion)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"1025:1025\"\n")
	b.WriteString("      - \"8025:8025\"\n")
	b.WriteString("    networks: [nova]\n\n")

	// Shared services (collected from all projects' shared_services)
	sharedNames := make([]string, 0, len(opts.SharedServices))
	for name := range opts.SharedServices {
		sharedNames = append(sharedNames, name)
	}
	sort.Strings(sharedNames)

	for _, name := range sharedNames {
		svc := opts.SharedServices[name]
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
			envKeys := make([]string, 0, len(svc.Environment))
			for k := range svc.Environment {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			for _, k := range envKeys {
				fmt.Fprintf(&b, "      %s: %q\n", k, svc.Environment[k])
			}
		}

		if svc.Command != "" {
			fmt.Fprintf(&b, "    command: %s\n", svc.Command)
		}

		if len(svc.Volumes) > 0 {
			b.WriteString("    volumes:\n")
			for _, v := range svc.Volumes {
				fmt.Fprintf(&b, "      - %s\n", v)
			}
			// Track named volumes (source doesn't start with / or .)
			for _, v := range svc.Volumes {
				source := strings.SplitN(v, ":", 2)[0]
				if !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, ".") {
					volumes = append(volumes, source)
				}
			}
		}

		b.WriteString("    networks: [nova]\n\n")
	}

	// Volumes (only for included services)
	b.WriteString("volumes:\n")
	for _, v := range volumes {
		fmt.Fprintf(&b, "  %s:\n", v)
	}
	b.WriteString("\n")

	b.WriteString("networks:\n")
	b.WriteString("  nova:\n")
	b.WriteString("    name: nova\n")

	return b.String()
}
