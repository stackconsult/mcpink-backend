package account

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
)

type Activities struct {
	projectsQ projects.Querier
	logger    *slog.Logger
}

func NewActivities(projectsQ projects.Querier, logger *slog.Logger) *Activities {
	return &Activities{
		projectsQ: projectsQ,
		logger:    logger,
	}
}

type CreateDefaultProjectInput struct {
	UserID string
}

type CreateDefaultProjectResult struct {
	ProjectID string
}

func (a *Activities) CreateDefaultProject(ctx context.Context, input CreateDefaultProjectInput) (*CreateDefaultProjectResult, error) {
	a.logger.Info("Creating default project", "userID", input.UserID)

	project, err := a.projectsQ.CreateDefaultProject(ctx, input.UserID)
	if err != nil {
		a.logger.Error("Failed to create default project",
			"userID", input.UserID,
			"error", err)
		return nil, fmt.Errorf("failed to create default project: %w", err)
	}

	a.logger.Info("Created default project",
		"userID", input.UserID,
		"projectID", project.ID)

	return &CreateDefaultProjectResult{ProjectID: project.ID}, nil
}
