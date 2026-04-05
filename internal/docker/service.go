package docker

import "github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"

type Service struct {
	ProjectsDir string
}

func (s Service) Up(phpVersions []string) error {
	return Up(s.ProjectsDir, phpVersions)
}
func (s Service) Down() error { return Down() }
func (s Service) Exec(service, workdir string, args ...string) error {
	return Exec(service, workdir, args...)
}
func (s Service) UpProject(projectName, projectDir string, services map[string]config.ServiceDefinition) error {
	return ProjectUp(projectName, projectDir, services)
}
func (s Service) DownProject(projectName, projectDir string) error {
	return ProjectDown(projectName, projectDir)
}
