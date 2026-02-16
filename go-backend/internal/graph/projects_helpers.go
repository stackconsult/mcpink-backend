package graph

import (
	"context"

	"github.com/augustdev/autoclip/internal/graph/dataloader"
	"github.com/augustdev/autoclip/internal/graph/model"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
)

func enrichServices(ctx context.Context, dbServices []services.Service) ([]*model.Service, error) {
	if len(dbServices) == 0 {
		return []*model.Service{}, nil
	}

	loaders := dataloader.For(ctx)
	svcIDs := make([]string, len(dbServices))
	for i, s := range dbServices {
		svcIDs[i] = s.ID
	}

	// Batch-load deployments and custom domains. Errors are non-fatal for enrichment.
	deps, depsErr := loaders.LatestDeploymentByServiceID.LoadAll(ctx, svcIDs)
	domains, domainsErr := loaders.CustomDomainByServiceID.LoadAll(ctx, svcIDs)

	result := make([]*model.Service, len(dbServices))
	for i, s := range dbServices {
		result[i] = dbServiceToModel(&s)
		if depsErr == nil && i < len(deps) && deps[i] != nil {
			enrichServiceWithDeployment(result[i], deps[i])
		}
		if domainsErr == nil && i < len(domains) && domains[i] != nil {
			result[i].CustomDomain = &domains[i].Domain
			result[i].CustomDomainStatus = &domains[i].Status
		}
	}
	return result, nil
}

func dbProjectToModel(dbProject *projects.Project) *model.Project {
	return &model.Project{
		ID:        dbProject.ID,
		Name:      dbProject.Name,
		Ref:       dbProject.Ref,
		CreatedAt: dbProject.CreatedAt.Time,
		UpdatedAt: dbProject.UpdatedAt.Time,
	}
}
