package caddy

// Service wraps the caddy package functions into a struct
// that satisfies lifecycle.CaddyService.
type Service struct{}

func (Service) Start() error                                  { return Start() }
func (Service) Stop() error                                   { return Stop() }
func (Service) Link(siteName, projectDir, fpmSocket string) error { return Link(siteName, projectDir, fpmSocket) }
func (Service) Unlink(siteName string) error                  { return Unlink(siteName) }
