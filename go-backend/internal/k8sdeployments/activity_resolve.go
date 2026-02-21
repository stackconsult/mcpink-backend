package k8sdeployments

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"go.temporal.io/sdk/temporal"
)

func buildImageTag(commitSHA string, svc services.Service) string {
	bc := parseBuildConfig(svc.BuildConfig)
	envHash := hashEnvVarsRaw(svc.EnvVars)
	if svc.BuildPack == "railpack" && bc.PublishDirectory == "" && bc.RootDirectory == "" && bc.BuildCommand == "" && bc.StartCommand == "" && envHash == "" {
		return commitSHA
	}
	h := sha256.Sum256([]byte(svc.BuildPack + "\x00" + bc.PublishDirectory + "\x00" + bc.RootDirectory + "\x00" + bc.DockerfilePath + "\x00" + bc.BuildCommand + "\x00" + bc.StartCommand + "\x00" + envHash))
	return fmt.Sprintf("%s-%x", commitSHA, h[:4])
}

func hashEnvVarsRaw(raw json.RawMessage) string {
	envVars := parseEnvVars(raw)
	if len(envVars) == 0 {
		return ""
	}
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(envVars[k]))
		h.Write([]byte("\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (a *Activities) ResolveImageRef(ctx context.Context, input ResolveImageRefInput) (*ResolveImageRefResult, error) {
	if input.ServiceID == "" {
		return nil, fmt.Errorf("service id is required")
	}
	if input.CommitSHA == "" {
		return nil, fmt.Errorf("commit sha is required")
	}

	id, err := a.resolveServiceIdentity(ctx, input.ServiceID)
	if err != nil {
		return nil, err
	}

	tag := buildImageTag(input.CommitSHA, id.Service)
	imageRef := fmt.Sprintf("%s/%s/%s:%s", a.config.RegistryAddress, id.Namespace, id.Name, tag)
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

	id, err := a.resolveServiceIdentity(ctx, input.ServiceID)
	if err != nil {
		return nil, err
	}

	bc := parseBuildConfig(id.Service.BuildConfig)

	// Apply root_directory: narrow build context to subdirectory
	effectiveSourcePath := input.SourcePath
	if bc.RootDirectory != "" {
		effectiveSourcePath = filepath.Join(input.SourcePath, bc.RootDirectory)
		if _, err := os.Stat(effectiveSourcePath); err != nil {
			if os.IsNotExist(err) {
				return nil, temporal.NewNonRetryableApplicationError(
					fmt.Sprintf("root_directory %q not found in repo", bc.RootDirectory),
					"source_path_missing",
					err,
				)
			}
			return nil, fmt.Errorf("stat effective source path: %w", err)
		}
	}

	tag := buildImageTag(input.CommitSHA, id.Service)
	imageRef := fmt.Sprintf("%s/%s/%s:%s", a.config.RegistryAddress, id.Namespace, id.Name, tag)

	// Determine build pack
	buildPack := id.Service.BuildPack
	switch buildPack {
	case "railpack", "nixpacks":
		buildPack = "railpack"
	case "dockerfile":
		dockerfileName := "Dockerfile"
		if bc.DockerfilePath != "" {
			dockerfileName = bc.DockerfilePath
		}
		dockerfileFull := filepath.Join(effectiveSourcePath, dockerfileName)
		if _, err := os.Stat(dockerfileFull); os.IsNotExist(err) {
			return nil, fmt.Errorf("build pack is 'dockerfile' but %q not found in repo", dockerfileName)
		}
		// Auto-detect port from EXPOSE if user didn't explicitly provide one.
		// Check for "" (new product server) or "3000" (old product server default).
		if id.Service.Port == "" {
			if detected := extractPortFromDockerfile(dockerfileFull); detected != "" {
				id.Service.Port = detected
			}
		}
	case "static":
		id.Service.Port = "8080"
	case "dockercompose":
		return nil, fmt.Errorf("dockercompose build pack is not supported on k8s")
	default:
		// Auto-detect: check for Dockerfile (custom path or default), else railpack
		dockerfileName := "Dockerfile"
		if bc.DockerfilePath != "" {
			dockerfileName = bc.DockerfilePath
		}
		dockerfileFull := filepath.Join(effectiveSourcePath, dockerfileName)
		if _, err := os.Stat(dockerfileFull); err == nil {
			buildPack = "dockerfile"
			if id.Service.Port == "" {
				if detected := extractPortFromDockerfile(dockerfileFull); detected != "" {
					id.Service.Port = detected
				}
			}
		} else {
			buildPack = "railpack"
		}
	}

	id.Service.Port = effectiveAppPort(buildPack, id.Service.Port, bc.PublishDirectory)

	envVars := parseEnvVars(id.Service.EnvVars)
	envVars["PORT"] = id.Service.Port

	a.logger.Info("ResolveBuildContext completed",
		"serviceID", input.ServiceID,
		"namespace", id.Namespace,
		"name", id.Name,
		"buildPack", buildPack,
		"imageRef", imageRef,
		"effectiveSourcePath", effectiveSourcePath)

	return &ResolveBuildContextResult{
		BuildPack:           buildPack,
		ImageRef:            imageRef,
		Namespace:           id.Namespace,
		Name:                id.Name,
		Port:                id.Service.Port,
		EnvVars:             envVars,
		PublishDirectory:    bc.PublishDirectory,
		EffectiveSourcePath: effectiveSourcePath,
		DockerfilePath:      bc.DockerfilePath,
		BuildCommand:        bc.BuildCommand,
		StartCommand:        bc.StartCommand,
	}, nil
}

func parseEnvVars(raw json.RawMessage) map[string]string {
	envVars := make(map[string]string)
	if len(raw) == 0 {
		return envVars
	}
	if err := json.Unmarshal(raw, &envVars); err != nil {
		var envArr []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(raw, &envArr); err == nil {
			for _, e := range envArr {
				envVars[e.Key] = e.Value
			}
		}
	}
	return envVars
}

type serviceIdentity struct {
	Namespace  string
	Name       string
	Tenant     string
	ProjectRef string
	Service    services.Service
}

func (a *Activities) resolveServiceIdentity(ctx context.Context, serviceID string) (*serviceIdentity, error) {
	svc, err := a.servicesQ.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("get service: %w", err)
	}

	project, err := a.projectsQ.GetProjectByID(ctx, svc.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	user, err := a.usersQ.GetUserByID(ctx, svc.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	tenant := user.ID
	if tenant == "" {
		return nil, fmt.Errorf("user %s has no ID", svc.UserID)
	}
	if svc.Name == nil || *svc.Name == "" {
		return nil, fmt.Errorf("service %s has empty service name", svc.ID)
	}

	return &serviceIdentity{
		Namespace:  NamespaceName(tenant, project.Ref),
		Name:       ServiceName(*svc.Name),
		Tenant:     tenant,
		ProjectRef: project.Ref,
		Service:    svc,
	}, nil
}
