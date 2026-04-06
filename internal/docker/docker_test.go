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
