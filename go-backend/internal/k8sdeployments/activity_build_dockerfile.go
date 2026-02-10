package k8sdeployments

import (
	"context"
	"fmt"
	"os"
)

func (a *Activities) DockerfileBuild(ctx context.Context, input BuildImageInput) (*BuildImageResult, error) {
	a.logger.Info("DockerfileBuild activity started",
		"imageRef", input.ImageRef,
		"sourcePath", input.SourcePath)

	if _, err := os.Stat(input.SourcePath); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	lokiLogger := a.newBuildLokiLogger(input.Name, input.Namespace)

	cacheRef := ""
	if a.config.RegistryAddress != "" {
		cacheRef = fmt.Sprintf("%s/cache/%s/%s:buildcache", a.config.RegistryAddress, input.Namespace, input.Name)
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
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
		lokiLogger.Log(fmt.Sprintf("BUILD FAILED: %v", err))
		_ = lokiLogger.Flush(ctx)
		return nil, fmt.Errorf("dockerfile build: %w", err)
	}

	lokiLogger.Log(fmt.Sprintf("BUILD SUCCESS: %s", input.ImageRef))
	_ = lokiLogger.Flush(ctx)

	return &BuildImageResult{ImageRef: input.ImageRef}, nil
}
