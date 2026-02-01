package graph

import (
	"context"

	"github.com/augustdev/autoclip/internal/graph/model"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
)

func (r *Resolver) getAppsForProject(ctx context.Context, projectID string) ([]*model.App, error) {
	dbApps, err := r.AppQueries.ListAppsByProjectID(ctx, apps.ListAppsByProjectIDParams{
		ProjectID: projectID,
		Limit:     1000,
		Offset:    0,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*model.App, len(dbApps))
	for i, dbApp := range dbApps {
		result[i] = dbAppToModel(&dbApp)
	}
	return result, nil
}

func dbProjectToModel(dbProject *projects.Project, projectApps []*model.App) *model.Project {
	if projectApps == nil {
		projectApps = []*model.App{}
	}
	return &model.Project{
		ID:        dbProject.ID,
		Name:      dbProject.Name,
		Ref:       dbProject.Ref,
		Apps:      projectApps,
		CreatedAt: dbProject.CreatedAt.Time,
		UpdatedAt: dbProject.UpdatedAt.Time,
	}
}
