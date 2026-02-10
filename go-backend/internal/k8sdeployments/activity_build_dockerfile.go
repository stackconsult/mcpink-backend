package k8sdeployments

import (
	"context"
	"fmt"
	"os"

	"go.temporal.io/sdk/temporal"
)

func (a *Activities) DockerfileBuild(ctx context.Context, input BuildImageInput) (*BuildImageResult, error) {
	a.logger.Info("DockerfileBuild activity started",
		"imageRef", input.ImageRef,
		"sourcePath", input.SourcePath)

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

	lokiLogger := a.newBuildLokiLogger(input.Name, input.Namespace)

	cacheRef := ""
	if a.config.RegistryHost != "" {
		cacheRef = fmt.Sprintf("%s/cache/%s/%s:buildcache", a.config.RegistryHost, input.Namespace, input.Name)
	}

	lokiLogger.Log("Building image from Dockerfile with BuildKit...")

	err := buildWithDockerfile(ctx, buildkitSolveOpts{
		BuildkitHost: a.config.BuildkitHost,
		SourcePath:   input.SourcePath,
		ImageRef:     input.ImageRef,
		CacheRef:     cacheRef,
		LokiLogger:   lokiLogger,
	})
	if err != nil {
		lokiLogger.Log(fmt.Sprintf("BUILD FAILED: %v", err))
		_ = lokiLogger.Flush(ctx)
		return nil, fmt.Errorf("dockerfile build: %w", err)
	}

	lokiLogger.Log(fmt.Sprintf("BUILD SUCCESS: %s", input.ImageRef))
	_ = lokiLogger.Flush(ctx)

	return &BuildImageResult{ImageRef: input.ImageRef}, nil
}
