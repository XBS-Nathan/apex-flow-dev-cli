package docker

// Service wraps the docker package functions into a struct
// that satisfies lifecycle.DockerService.
type Service struct{}

func (Service) Up() error   { return Up() }
func (Service) Down() error { return Down() }
