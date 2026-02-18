package graph

import (
	"github.com/augustdev/autoclip/internal/graph/model"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
)

func dbProjectToModel(dbProject *projects.Project) *model.Project {
	return &model.Project{
		ID:        dbProject.ID,
		Name:      dbProject.Name,
		Ref:       dbProject.Ref,
		CreatedAt: dbProject.CreatedAt.Time,
		UpdatedAt: dbProject.UpdatedAt.Time,
	}
}
