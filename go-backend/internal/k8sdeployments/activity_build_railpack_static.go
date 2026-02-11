package k8sdeployments

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (a *Activities) RailpackStaticBuild(ctx context.Context, input BuildImageInput) (*BuildImageResult, error) {
	a.logger.Info("RailpackStaticBuild activity started",
		"imageRef", input.ImageRef,
		"sourcePath", input.SourcePath,
		"publishDirectory", input.PublishDirectory)

	if _, err := os.Stat(input.SourcePath); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	lokiLogger := a.newBuildLokiLogger(input.Name, input.Namespace)

	// Phase 1: Build with railpack into a temporary "-build" image
	buildImageRef := input.ImageRef + "-build"
	buildInput := BuildImageInput{
		SourcePath: input.SourcePath,
		ImageRef:   buildImageRef,
		BuildPack:  "railpack",
		Name:       input.Name,
		Namespace:  input.Namespace,
		EnvVars:    input.EnvVars,
	}

	lokiLogger.Log("Phase 1: Building application with Railpack...")
	buildResult, err := a.RailpackBuild(ctx, buildInput)
	if err != nil {
		return nil, fmt.Errorf("railpack-static phase 1 (build): %w", err)
	}
	lokiLogger.Log(fmt.Sprintf("Phase 1 complete: %s", buildResult.ImageRef))

	// Phase 2: Create nginx image that serves the built static files
	lokiLogger.Log("Phase 2: Creating nginx image for static serving...")

	tmpDir, err := os.MkdirTemp("", "railpack-static-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dockerfile := fmt.Sprintf(`FROM %s AS build
FROM nginxinc/nginx-unprivileged:alpine
COPY --from=build --chown=nginx:nginx /app/%s/ /usr/share/nginx/html/
COPY --chown=nginx:nginx nginx.conf /etc/nginx/conf.d/default.conf
`, buildImageRef, input.PublishDirectory)

	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return nil, fmt.Errorf("write Dockerfile: %w", err)
	}

	nginxConf := `server {
    listen 8080;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "nginx.conf"), []byte(nginxConf), 0o644); err != nil {
		return nil, fmt.Errorf("write nginx.conf: %w", err)
	}

	err = buildWithDockerfile(ctx, buildkitSolveOpts{
		BuildkitHost: a.config.BuildkitHost,
		SourcePath:   tmpDir,
		ImageRef:     input.ImageRef,
		CacheRef:     "",
		LokiLogger:   lokiLogger,
	})
	if err != nil {
		lokiLogger.Log(fmt.Sprintf("BUILD FAILED: %v", err))
		_ = lokiLogger.Flush(ctx)
		return nil, fmt.Errorf("railpack-static phase 2 (nginx): %w", err)
	}

	lokiLogger.Log(fmt.Sprintf("BUILD SUCCESS: %s", input.ImageRef))
	_ = lokiLogger.Flush(ctx)

	return &BuildImageResult{ImageRef: input.ImageRef}, nil
}
