package graph

import (
	"log/slog"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
)

type Resolver struct {
	Db               *pg.DB
	Logger           *slog.Logger
	AuthService      *auth.Service
	GitHubAppService *githubapp.Service
	CoolifyClient    *coolify.Client
	AppQueries       apps.Querier
	ProjectQueries   projects.Querier
}
