package k8sdeployments

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/app"
	"go.temporal.io/sdk/temporal"
)

func (a *Activities) RailpackBuild(ctx context.Context, input BuildImageInput) (*BuildImageResult, error) {
	a.logger.Info("RailpackBuild activity started",
		"imageRef", input.ImageRef,
		"sourcePath", input.SourcePath)

	if _, err := os.Stat(input.SourcePath); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
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

	// 2. Write plan to temp directory for BuildKit frontend
	planDir, err := os.MkdirTemp("", "railpack-plan-*")
	if err != nil {
		return nil, fmt.Errorf("create plan dir: %w", err)
	}
	defer os.RemoveAll(planDir)

	planBytes, err := json.Marshal(result.Plan)
	if err != nil {
		return nil, fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "railpack-plan.json"), planBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write plan file: %w", err)
	}

	// 3. Build with Railpack BuildKit frontend
	cacheKey := fmt.Sprintf("%s/%s", input.Namespace, input.Name)
	cacheRef := ""
	if a.config.RegistryAddress != "" {
		cacheRef = fmt.Sprintf("%s/cache/%s/%s:buildcache", a.config.RegistryAddress, input.Namespace, input.Name)
	}

	lokiLogger.Log("Building image with Railpack frontend...")

	if err := buildWithRailpackFrontend(ctx, planDir, railpackFrontendOpts{
		CacheKey:    cacheKey,
		SecretsHash: hashEnvVars(input.EnvVars),
		Secrets:     input.EnvVars,
	}, buildkitSolveOpts{
		BuildkitHost: a.config.BuildkitHost,
		SourcePath:   input.SourcePath,
		ImageRef:     input.ImageRef,
		CacheRef:     cacheRef,
		LokiLogger:   lokiLogger,
	}); err != nil {
		if isPathMissingErr(err) {
			return nil, sourcePathMissingError(input.SourcePath, err)
		}
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

func hashEnvVars(envVars map[string]string) string {
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
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
