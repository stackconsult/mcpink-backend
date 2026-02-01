package graph

import (
	"log/slog"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/storage/pg"
)

type Resolver struct {
	Db               *pg.DB
	Logger           *slog.Logger
	AuthService      *auth.Service
	GitHubAppService *githubapp.Service
	CoolifyClient    *coolify.Client
}
