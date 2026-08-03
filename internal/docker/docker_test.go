package docker

import (
	"strings"
	"testing"

	"github.com/XBS-Nathan/nova/internal/config"
)

func defaultOpts(t *testing.T, versions ...string) ComposeOptions {
	t.Helper()
	php := make([]PHPVersion, len(versions))
	for i, v := range versions {
		php[i] = PHPVersion{Version: v}
	}
	return ComposeOptions{
		ProjectsDir:   "/home/user/Projects",
		PHP:           php,
		MySQLVersions: []string{config.DefaultMySQLVersion},
		RedisVersions: []string{config.DefaultRedisVersion},
		MailpitVersion: config.DefaultMailpitVersion,
	}
}

func TestGenerateCompose_IncludesCaddy(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2"))
	if !strings.Contains(got, "caddy:") {
		t.Error("missing caddy service")
	}
	if !strings.Contains(got, "caddy:2-alpine") {
		t.Error("missing caddy image")
	}
}

func TestGenerateCompose_IncludesPHPVersions(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2", "8.3"))
	if !strings.Contains(got, "php82:") {
		t.Error("missing php82 service")
	}
	if !strings.Contains(got, "php83:") {
		t.Error("missing php83 service")
	}
}

func TestGenerateCompose_PHPServiceIncludesNovaEnv(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2"))
	if !strings.Contains(got, "NOVA: \"true\"") {
		t.Errorf("missing NOVA environment variable in PHP service:\n%s", got)
	}
}

func TestGenerateCompose_MountsProjectsDir(t *testing.T) {
	t.Parallel()
	opts := defaultOpts(t, "8.2")
	opts.ProjectsDir = "/home/user/Code"
	got := generateCompose(opts)
	if !strings.Contains(got, "/home/user/Code:/srv") {
		t.Error("missing projects dir mount")
	}
}

func TestGenerateCompose_IncludesVersionedMySQL(t *testing.T) {
	t.Parallel()
	opts := defaultOpts(t, "8.2")
	opts.MySQLVersions = []string{"8.0", "9.0"}
	got := generateCompose(opts)
	if !strings.Contains(got, "mysql_80:") {
		t.Errorf("missing mysql_80 service in:\n%s", got)
	}
	if !strings.Contains(got, "mysql_90:") {
		t.Errorf("missing mysql_90 service in:\n%s", got)
	}
	if !strings.Contains(got, "3306:3306") {
		t.Error("missing first mysql port 3306")
	}
	if !strings.Contains(got, "3307:3306") {
		t.Error("missing second mysql port 3307")
	}
}

func TestGenerateCompose_IncludesVersionedRedis(t *testing.T) {
	t.Parallel()
	opts := defaultOpts(t, "8.2")
	opts.RedisVersions = []string{"7", "8"}
	got := generateCompose(opts)
	if !strings.Contains(got, "redis_7:") {
		t.Errorf("missing redis_7 service in:\n%s", got)
	}
	if !strings.Contains(got, "redis_8:") {
		t.Errorf("missing redis_8 service in:\n%s", got)
	}
}

func TestGenerateCompose_IncludesVersionedPostgres(t *testing.T) {
	t.Parallel()
	opts := defaultOpts(t, "8.2")
	opts.MySQLVersions = nil
	opts.PostgresVersions = []string{"15", "16"}
	got := generateCompose(opts)
	if !strings.Contains(got, "postgres_15:") {
		t.Errorf("missing postgres_15 service in:\n%s", got)
	}
	if !strings.Contains(got, "postgres_16:") {
		t.Errorf("missing postgres_16 service in:\n%s", got)
	}
}

func TestGenerateCompose_IncludesMailpit(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2"))
	if !strings.Contains(got, "mailpit:") {
		t.Error("missing mailpit service")
	}
	if !strings.Contains(got, "axllent/mailpit:") {
		t.Error("missing mailpit image")
	}
	if !strings.Contains(got, "1025:1025") {
		t.Error("missing mailpit SMTP port")
	}
	if !strings.Contains(got, "8025:8025") {
		t.Error("missing mailpit web UI port")
	}
}

func TestGenerateCompose_SharedServices(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2"))
	if strings.Contains(got, "typesense:") {
		t.Error("typesense should not be included by default")
	}

	opts := defaultOpts(t, "8.2")
	opts.SharedServices = map[string]config.ServiceDefinition{
		"typesense": {
			Image:       "typesense/typesense:26.0",
			Ports:       []string{"8108:8108"},
			Environment: map[string]string{"TYPESENSE_API_KEY": "dev"},
			Volumes:     []string{"typesense_data:/data"},
			Command:     "--data-dir /data --enable-cors",
		},
	}
	got = generateCompose(opts)
	if !strings.Contains(got, "typesense:") {
		t.Error("typesense should be included as shared service")
	}
	if !strings.Contains(got, "typesense/typesense:26.0") {
		t.Error("missing typesense image")
	}
	if !strings.Contains(got, "typesense_data") {
		t.Error("missing typesense volume")
	}
}

func TestGenerateCompose_MountsMysqlConfD(t *testing.T) {
	t.Parallel()
	got := generateCompose(defaultOpts(t, "8.2"))
	if !strings.Contains(got, "/mysql/conf.d:/etc/mysql/conf.d") {
		t.Errorf("missing mysql conf.d mount in:\n%s", got)
	}
}

func TestPHPServiceName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version string
		want    string
	}{
		{"8.2", "php82"},
		{"8.3", "php83"},
		{"7.4", "php74"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := PHPServiceName(tt.version)
			if got != tt.want {
				t.Errorf("PHPServiceName(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestServiceName_SingleVersion(t *testing.T) {
	t.Parallel()
	if got := ServiceName("mysql", "8.0", 1); got != "mysql" {
		t.Errorf("ServiceName with 1 version = %q, want %q", got, "mysql")
	}
	if got := ServiceName("redis", "7", 1); got != "redis" {
		t.Errorf("ServiceName with 1 version = %q, want %q", got, "redis")
	}
}

func TestServiceName_MultipleVersions(t *testing.T) {
	t.Parallel()
	if got := ServiceName("mysql", "8.0", 2); got != "mysql_80" {
		t.Errorf("ServiceName with 2 versions = %q, want %q", got, "mysql_80")
	}
	if got := ServiceName("postgres", "15", 3); got != "postgres_15" {
		t.Errorf("ServiceName with 3 versions = %q, want %q", got, "postgres_15")
	}
}

func TestGenerateCompose_FrankenPHP_Octane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	got := generateCompose(ComposeOptions{
		ProjectsDir:    "/proj",
		MailpitVersion: "v1.20",
		FrankenPHP: []FrankenPHPProject{{
			Name:       "myapp",
			PHPVersion: "8.3",
			Extensions: []string{"gd"},
			Octane:     true,
			Workdir:    "/srv/myapp",
		}},
	})

	for _, want := range []string{
		"myapp_frankenphp:",
		"working_dir: /srv/myapp",
		"NOVA_APP: \"myapp\"",
		"octane:start",
		"--server=frankenphp",
		"--host=0.0.0.0",
		"--port=8000",
		"--workers=auto",
		"--max-requests=500",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in compose:\n%s", want, got)
		}
	}

	// No host port mapping — internal only.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "8000:8000") {
			t.Errorf("FrankenPHP service should not expose host port 8000, line: %q", line)
		}
	}
}

func TestGenerateCompose_FrankenPHP_ClassicMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	got := generateCompose(ComposeOptions{
		ProjectsDir:    "/proj",
		MailpitVersion: "v1.20",
		FrankenPHP: []FrankenPHPProject{{
			Name:       "myapp",
			PHPVersion: "8.3",
			Octane:     false,
			Workdir:    "/srv/myapp",
		}},
	})

	if strings.Contains(got, "octane:start") {
		t.Errorf("classic mode should not emit octane:start command, got:\n%s", got)
	}
	if !strings.Contains(got, "myapp_frankenphp:") {
		t.Errorf("classic mode missing myapp_frankenphp service, got:\n%s", got)
	}
}

func TestGenerateCompose_FrankenPHP_PortsExposedOnCaddy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	got := generateCompose(ComposeOptions{
		ProjectsDir:    "/proj",
		MailpitVersion: "v1.20",
		FrankenPHP: []FrankenPHPProject{{
			Name:       "myapp",
			PHPVersion: "8.3",
			Workdir:    "/srv/myapp",
			Ports:      []string{"8080"},
		}},
	})

	// Caddy must expose the project's extra port so external requests reach it.
	if !strings.Contains(got, "\"8080:8080\"") {
		t.Errorf("Caddy missing 8080 host-port mapping, got:\n%s", got)
	}
}

func TestGenerateCompose_NoFrankenPHP_NoChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Without any FrankenPHP entries, the output should not mention frankenphp.
	got := generateCompose(ComposeOptions{
		ProjectsDir:    "/proj",
		MailpitVersion: "v1.20",
		PHP: []PHPVersion{{Version: "8.3"}},
	})
	if strings.Contains(got, "frankenphp") {
		t.Errorf("compose should not mention frankenphp when no FrankenPHP projects, got:\n%s", got)
	}
}

func TestGenerateCompose_DNSService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	opts := defaultOpts(t, "8.2")
	opts.DNSListenIP = "100.64.0.5"
	got := generateCompose(opts)

	if !strings.Contains(got, "  dns:\n") {
		t.Fatalf("missing dns service, got:\n%s", got)
	}
	if !strings.Contains(got, "coredns/coredns") {
		t.Error("dns service missing coredns image")
	}
	// Port 53 must bind only the requested address, not 0.0.0.0.
	if !strings.Contains(got, "\"100.64.0.5:53:53/udp\"") {
		t.Errorf("dns service missing udp port binding, got:\n%s", got)
	}
	if !strings.Contains(got, "\"100.64.0.5:53:53/tcp\"") {
		t.Errorf("dns service missing tcp port binding, got:\n%s", got)
	}
	if !strings.Contains(got, "/dns:/etc/coredns") {
		t.Errorf("dns service missing Corefile mount, got:\n%s", got)
	}
}

func TestGenerateCompose_NoDNSByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	got := generateCompose(defaultOpts(t, "8.2"))
	if strings.Contains(got, "coredns") {
		t.Errorf("compose should not include dns service by default, got:\n%s", got)
	}
}
