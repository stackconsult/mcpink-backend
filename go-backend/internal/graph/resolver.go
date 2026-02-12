package graph

import (
	"log/slog"

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/prometheus"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/resources"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
)

type Resolver struct {
	Db               *pg.DB
	Logger           *slog.Logger
	AuthService      *auth.Service
	GitHubAppService *githubapp.Service
	ServiceQueries   services.Querier
	ProjectQueries   projects.Querier
	ResourceQueries  resources.Querier
	FirebaseAuth     *firebaseauth.Client
	PrometheusClient *prometheus.Client
}
