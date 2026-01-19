package deployment

import "context"

type App struct {
	ID           string
	Name         string
	Status       string
	Organization string
	Hostname     string
}

type AppStatus struct {
	ID       string
	Name     string
	Status   string
	Deployed bool
	Hostname string
}

type Provider interface {
	ListApps(ctx context.Context) ([]App, error)
	GetAppStatus(ctx context.Context, appName string) (*AppStatus, error)
}
