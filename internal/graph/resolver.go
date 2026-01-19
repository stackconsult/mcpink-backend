package graph

import (
	"log/slog"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/storage/pg"
)

type Resolver struct {
	Db          *pg.DB
	Logger      *slog.Logger
	AuthService *auth.Service
}
