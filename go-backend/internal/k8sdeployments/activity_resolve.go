package k8sdeployments

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
	"go.temporal.io/sdk/temporal"
)

func (a *Activities) ResolveImageRef(ctx context.Context, input ResolveImageRefInput) (*ResolveImageRefResult, error) {
	if input.ServiceID == "" {
		return nil, fmt.Errorf("service id is required")
	}
	if input.CommitSHA == "" {
		return nil, fmt.Errorf("commit sha is required")
	}

	namespace, name, _, err := a.resolveServiceIdentity(ctx, input.ServiceID)
	if err != nil {
		return nil, err
	}

	imageRef := fmt.Sprintf("%s/%s/%s:%s", a.config.RegistryAddress, namespace, name, input.CommitSHA)
	return &ResolveImageRefResult{ImageRef: imageRef}, nil
}

func (a *Activities) ResolveBuildContext(ctx context.Context, input ResolveBuildContextInput) (*ResolveBuildContextResult, error) {
	a.logger.Info("ResolveBuildContext activity started", "serviceID", input.ServiceID)

	if _, err := os.Stat(input.SourcePath); err != nil {
		if os.IsNotExist(err) {
			return nil, temporal.NewNonRetryableApplicationError(
				fmt.Sprintf("source path missing: %s", input.SourcePath),
				"source_path_missing",
				err,
			)
		}
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	namespace, name, app, err := a.resolveServiceIdentity(ctx, input.ServiceID)
	if err != nil {
		return nil, err
	}

	imageRef := fmt.Sprintf("%s/%s/%s:%s", a.config.RegistryAddress, namespace, name, input.CommitSHA)

	// Parse env vars from app
	envVars := make(map[string]string)
	if len(app.EnvVars) > 0 {
		if err := json.Unmarshal(app.EnvVars, &envVars); err != nil {
			a.logger.Warn("failed to parse env vars as map, trying array format", "error", err)
			// Try array of {key, value} format
			var envArr []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.Unmarshal(app.EnvVars, &envArr); err == nil {
				for _, e := range envArr {
					envVars[e.Key] = e.Value
				}
			}
		}
	}
	envVars["PORT"] = app.Port

	// Determine build pack
	buildPack := app.BuildPack
	switch buildPack {
	case "nixpacks":
		buildPack = "railpack"
	case "dockerfile":
		// Check that Dockerfile exists
		if _, err := os.Stat(filepath.Join(input.SourcePath, "Dockerfile")); os.IsNotExist(err) {
			return nil, fmt.Errorf("build pack is 'dockerfile' but no Dockerfile found in repo")
		}
	case "static":
		app.Port = "8080"
	case "dockercompose":
		return nil, fmt.Errorf("dockercompose build pack is not supported on k8s")
	default:
		// Auto-detect: check for Dockerfile, else railpack
		if _, err := os.Stat(filepath.Join(input.SourcePath, "Dockerfile")); err == nil {
			buildPack = "dockerfile"
		} else {
			buildPack = "railpack"
		}
	}

	a.logger.Info("ResolveBuildContext completed",
		"serviceID", input.ServiceID,
		"namespace", namespace,
		"name", name,
		"buildPack", buildPack,
		"imageRef", imageRef)

	return &ResolveBuildContextResult{
		BuildPack: buildPack,
		ImageRef:  imageRef,
		Namespace: namespace,
		Name:      name,
		Port:      app.Port,
		EnvVars:   envVars,
	}, nil
}

func (a *Activities) resolveServiceIdentity(ctx context.Context, serviceID string) (namespace, name string, app apps.App, _ error) {
	app, err := a.appsQ.GetAppByID(ctx, serviceID)
	if err != nil {
		return "", "", apps.App{}, fmt.Errorf("get app: %w", err)
	}

	project, err := a.projectsQ.GetProjectByID(ctx, app.ProjectID)
	if err != nil {
		return "", "", apps.App{}, fmt.Errorf("get project: %w", err)
	}

	user, err := a.usersQ.GetUserByID(ctx, app.UserID)
	if err != nil {
		return "", "", apps.App{}, fmt.Errorf("get user: %w", err)
	}

	username := ResolveUsername(user)
	if username == "" {
		return "", "", apps.App{}, fmt.Errorf("user %s has no gitea or github username", app.UserID)
	}
	if app.Name == nil || *app.Name == "" {
		return "", "", apps.App{}, fmt.Errorf("app %s has empty service name", app.ID)
	}

	namespace = NamespaceName(username, project.Ref)
	name = ServiceName(*app.Name)
	return namespace, name, app, nil
}
