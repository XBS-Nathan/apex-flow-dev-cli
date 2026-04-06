package cmd

import (
	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/docker"
)

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
