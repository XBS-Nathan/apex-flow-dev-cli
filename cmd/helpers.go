package cmd

import (
	"fmt"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/dns"
	"github.com/XBS-Nathan/nova/internal/docker"
)

// dnsListenIP resolves the bind address for the shared .test DNS
// responder, or "" when disabled. A detection failure disables DNS with
// a warning instead of blocking the command.
func dnsListenIP(global *config.GlobalConfig) string {
	if !global.DNS {
		return ""
	}
	ip, err := dns.DetectListenIP(global.DNSBind)
	if err != nil {
		fmt.Printf("  ! dns enabled but skipped: %v\n", err)
		return ""
	}
	return ip
}

// dbServiceForProject returns the docker compose service name for
// the project's database (e.g. "mysql", "postgres", or "mysql_80" if multiple).
func dbServiceForProject(
	projectCfg *config.ProjectConfig,
	global *config.GlobalConfig,
) string {
	collected := config.CollectVersions(global.ProjectsDir, projectCfg)

	if projectCfg.DBDriver == "postgres" {
		return docker.ServiceName("postgres", projectCfg.DBVersion, len(collected.Postgres))
	}
	return docker.ServiceName("mysql", projectCfg.DBVersion, len(collected.MySQL))
}
