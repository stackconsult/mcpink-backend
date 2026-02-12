package k8sdeployments

import (
	"errors"
	"log/slog"

	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"k8s.io/client-go/kubernetes"
)

var ErrNotImplemented = errors.New("k8s deployment activity not implemented")

type Activities struct {
	logger         *slog.Logger
	k8s            kubernetes.Interface
	githubApp      *githubapp.Service
	internalGitSvc *internalgit.Service
	servicesQ      services.Querier
	projectsQ      projects.Querier
	usersQ         users.Querier
	config         Config
}

func NewActivities(
	logger *slog.Logger,
	k8s kubernetes.Interface,
	githubApp *githubapp.Service,
	internalGitSvc *internalgit.Service,
	servicesQ services.Querier,
	projectsQ projects.Querier,
	usersQ users.Querier,
	config Config,
) *Activities {
	return &Activities{
		logger:         logger,
		k8s:            k8s,
		githubApp:      githubApp,
		internalGitSvc: internalGitSvc,
		servicesQ:      servicesQ,
		projectsQ:      projectsQ,
		usersQ:         usersQ,
		config:         config,
	}
}
