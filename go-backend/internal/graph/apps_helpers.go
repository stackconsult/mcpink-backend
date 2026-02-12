package graph

import (
	"encoding/json"

	"github.com/augustdev/autoclip/internal/graph/model"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
)

func dbAppToModel(dbApp *apps.App) *model.App {
	var envVars []*model.EnvVar
	if len(dbApp.EnvVars) > 0 {
		var rawEnvVars []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(dbApp.EnvVars, &rawEnvVars); err == nil {
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

	return &model.App{
		ID:            dbApp.ID,
		ProjectID:     dbApp.ProjectID,
		Name:          dbApp.Name,
		Repo:          dbApp.Repo,
		Branch:        dbApp.Branch,
		BuildStatus:   dbApp.BuildStatus,
		RuntimeStatus: dbApp.RuntimeStatus,
		ErrorMessage:  dbApp.ErrorMessage,
		EnvVars:       envVars,
		Fqdn:          dbApp.Fqdn,
		Memory:        dbApp.Memory,
		CPU:           dbApp.Cpu,
		CreatedAt:     dbApp.CreatedAt.Time,
		UpdatedAt:     dbApp.UpdatedAt.Time,
	}
}
