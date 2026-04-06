package docker

import "github.com/XBS-Nathan/nova/internal/config"

// Service wraps the docker package functions for the lifecycle interface.
type Service struct {
	ProjectsDir    string
	Collected      config.CollectedVersions
	MailpitVersion string
}

func (s Service) Up(php []PHPVersion, forceRecreate bool) error {
	return Up(ComposeOptions{
		ProjectsDir:      s.ProjectsDir,
		PHP:              php,
		MySQLVersions:    s.Collected.MySQL,
		PostgresVersions: s.Collected.Postgres,
		RedisVersions:    s.Collected.Redis,
		MailpitVersion:   s.MailpitVersion,
		SharedServices:   s.Collected.SharedServices,
		ForceRecreate:    forceRecreate,
	})
}

func (s Service) Down() error { return Down() }

func (s Service) Exec(service, workdir string, args ...string) error {
	return Exec(service, workdir, args...)
}

func (s Service) ExecDetached(service, workdir string, args ...string) error {
	return ExecDetached(service, workdir, args...)
}

func (s Service) UpProject(
	projectName, projectDir string,
	services map[string]config.ServiceDefinition,
) error {
	return ProjectUp(projectName, projectDir, services)
}

func (s Service) DownProject(projectName, projectDir string) error {
	return ProjectDown(projectName, projectDir)
}
