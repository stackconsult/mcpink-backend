package k8sdeployments

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/railwayapp/railpack/buildkit"
	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	"go.temporal.io/sdk/temporal"
)

func (a *Activities) RailpackBuild(ctx context.Context, input BuildImageInput) (*BuildImageResult, error) {
	a.logger.Info("RailpackBuild activity started",
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
	lokiLogger.Log("Generating build plan with railpack...")

	// 1. Generate build plan
	userApp, err := app.NewApp(input.SourcePath)
	if err != nil {
		lokiLogger.Log(fmt.Sprintf("RAILPACK ERROR: failed to create app: %v", err))
		_ = lokiLogger.Flush(ctx)
		return nil, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("railpack failed to analyze source: %v", err),
			"railpack_app_error",
			err,
		)
	}

	env := app.NewEnvironment(&input.EnvVars)
	result := core.GenerateBuildPlan(userApp, env, &core.GenerateBuildPlanOptions{})

	if !result.Success {
		var errorLines []string
		for _, l := range result.Logs {
			lokiLogger.Log(fmt.Sprintf("[railpack] %s", l.Msg))
			if l.Level == "error" {
				errorLines = append(errorLines, l.Msg)
			}
		}
		errMsg := "railpack plan generation failed"
		if len(errorLines) > 0 {
			errMsg = fmt.Sprintf("railpack plan generation failed: %s", strings.Join(errorLines, "; "))
		}
		lokiLogger.Log(fmt.Sprintf("BUILD FAILED: %s", errMsg))
		_ = lokiLogger.Flush(ctx)
		return nil, temporal.NewNonRetryableApplicationError(errMsg, "railpack_plan_failed", nil)
	}

	for _, provider := range result.DetectedProviders {
		lokiLogger.Log(fmt.Sprintf("Detected provider: %s", provider))
	}
	for _, l := range result.Logs {
		lokiLogger.Log(fmt.Sprintf("[railpack] %s", l.Msg))
	}

	// 2. Build with BuildKit using railpack's own BuildKit integration
	lokiLogger.Log("Building image with railpack + BuildKit...")

	cacheRef := ""
	if a.config.RegistryHost != "" {
		cacheRef = fmt.Sprintf("%s/cache/%s/%s:buildcache", a.config.RegistryHost, input.Namespace, input.Name)
	}

	// Railpack reads BUILDKIT_HOST from env
	os.Setenv("BUILDKIT_HOST", a.config.BuildkitHost)

	buildOpts := buildkit.BuildWithBuildkitClientOptions{
		ImageName:   input.ImageRef,
		ImportCache: cacheRef,
		ExportCache: cacheRef,
		CacheKey:    fmt.Sprintf("%s/%s", input.Namespace, input.Name),
	}

	if err := buildkit.BuildWithBuildkitClient(input.SourcePath, result.Plan, buildOpts); err != nil {
		lokiLogger.Log(fmt.Sprintf("BUILD FAILED: %v", err))
		_ = lokiLogger.Flush(ctx)
		return nil, fmt.Errorf("railpack build: %w", err)
	}

	lokiLogger.Log(fmt.Sprintf("BUILD SUCCESS: %s", input.ImageRef))
	_ = lokiLogger.Flush(ctx)

	return &BuildImageResult{ImageRef: input.ImageRef}, nil
}

func (a *Activities) newBuildLokiLogger(name, namespace string) *LokiLogger {
	return NewLokiLogger(a.config.LokiPushURL, map[string]string{
		"job":       "build",
		"service":   name,
		"namespace": namespace,
	})
}
