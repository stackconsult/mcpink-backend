package graph

import (
	"encoding/json"

	"github.com/augustdev/autoclip/internal/graph/model"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
)

func dbServiceToModel(dbService *services.Service) *model.Service {
	var envVars []*model.EnvVar
	if len(dbService.EnvVars) > 0 {
		var rawEnvVars []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(dbService.EnvVars, &rawEnvVars); err == nil {
			envVars = make([]*model.EnvVar, len(rawEnvVars))
			for i, ev := range rawEnvVars {
				envVars[i] = &model.EnvVar{
					Key:   ev.Key,
					Value: ev.Value,
				}
			}
		}
	}
	if envVars == nil {
		envVars = []*model.EnvVar{}
	}

	return &model.Service{
		ID:          dbService.ID,
		ProjectID:   dbService.ProjectID,
		Name:        dbService.Name,
		Repo:        dbService.Repo,
		Branch:      dbService.Branch,
		EnvVars:     envVars,
		Fqdn:        dbService.Fqdn,
		Port:        dbService.Port,
		GitProvider: dbService.GitProvider,
		Memory:      dbService.Memory,
		Vcpus:       dbService.Vcpus,
		CreatedAt:   dbService.CreatedAt.Time,
		UpdatedAt:   dbService.UpdatedAt.Time,
	}
}

