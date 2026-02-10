package k8sdeployments

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (a *Activities) StaticBuild(ctx context.Context, input BuildImageInput) (*BuildImageResult, error) {
	a.logger.Info("StaticBuild activity started",
		"imageRef", input.ImageRef,
		"sourcePath", input.SourcePath)

	if _, err := os.Stat(input.SourcePath); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	lokiLogger := a.newBuildLokiLogger(input.Name, input.Namespace)

	dockerfile := `FROM nginx:alpine AS build
WORKDIR /site
COPY . .
RUN rm -f nginx.conf Dockerfile docker-compose.yaml docker-compose.yml .env

FROM nginxinc/nginx-unprivileged:alpine
COPY --from=build --chown=nginx:nginx /site/ /usr/share/nginx/html/
COPY --chown=nginx:nginx nginx.conf /etc/nginx/conf.d/default.conf
`
	if err := os.WriteFile(filepath.Join(input.SourcePath, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
		return nil, fmt.Errorf("write static Dockerfile: %w", err)
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
	if err := os.WriteFile(filepath.Join(input.SourcePath, "nginx.conf"), []byte(nginxConf), 0o644); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
		return nil, fmt.Errorf("write nginx.conf: %w", err)
	}

	// Static builds are tiny and usually don't benefit from registry cache
	// import/export round-trips.
	cacheRef := ""

	lokiLogger.Log("Building static site image with nginx...")

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
		return nil, fmt.Errorf("static build: %w", err)
	}

	lokiLogger.Log(fmt.Sprintf("BUILD SUCCESS: %s", input.ImageRef))
	_ = lokiLogger.Flush(ctx)

	return &BuildImageResult{ImageRef: input.ImageRef}, nil
}
