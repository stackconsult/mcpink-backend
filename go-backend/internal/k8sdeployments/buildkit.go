package k8sdeployments

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"
)

const railpackFrontendImage = "ghcr.io/railwayapp/railpack-frontend:v0.17.1"

type buildkitSolveOpts struct {
	BuildkitHost string
	SourcePath   string
	ImageRef     string
	CacheRef     string
	LokiLogger   *LokiLogger
}

type railpackFrontendOpts struct {
	CacheKey    string
	SecretsHash string
	Secrets     map[string]string
}

func buildWithDockerfile(ctx context.Context, opts buildkitSolveOpts) error {
	c, err := client.New(ctx, opts.BuildkitHost)
	if err != nil {
		return fmt.Errorf("connect to buildkit: %w", err)
	}
	defer c.Close()

	srcFS, err := fsutil.NewFS(opts.SourcePath)
	if err != nil {
		return fmt.Errorf("create fs for source: %w", err)
	}

	solveOpt := client.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: map[string]string{},
		LocalMounts: map[string]fsutil.FS{
			"context":    srcFS,
			"dockerfile": srcFS,
		},
		Exports: []client.ExportEntry{
			{
				Type: client.ExporterImage,
				Attrs: map[string]string{
					"name": opts.ImageRef,
					"push": "true",
				},
			},
		},
	}

	if opts.CacheRef != "" {
		solveOpt.CacheImports = []client.CacheOptionsEntry{
			{Type: "registry", Attrs: map[string]string{"ref": opts.CacheRef}},
		}
		solveOpt.CacheExports = []client.CacheOptionsEntry{
			{Type: "registry", Attrs: map[string]string{"ref": opts.CacheRef, "mode": "max"}},
		}
	}

	ch := make(chan *client.SolveStatus)
	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		_, err := c.Solve(egCtx, nil, solveOpt, ch)
		return err
	})

	eg.Go(func() error {
		for status := range ch {
			if opts.LokiLogger != nil {
				for _, v := range status.Vertexes {
					if v.Name != "" {
						opts.LokiLogger.Log(v.Name)
					}
				}
				for _, l := range status.Logs {
					opts.LokiLogger.Log(string(l.Data))
				}
			}
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("buildkit solve: %w", err)
	}

	if opts.LokiLogger != nil {
		_ = opts.LokiLogger.Flush(ctx)
	}

	return nil
}

func buildWithRailpackFrontend(ctx context.Context, planDir string, rpOpts railpackFrontendOpts, opts buildkitSolveOpts) error {
	c, err := client.New(ctx, opts.BuildkitHost)
	if err != nil {
		return fmt.Errorf("connect to buildkit: %w", err)
	}
	defer c.Close()

	srcFS, err := fsutil.NewFS(opts.SourcePath)
	if err != nil {
		return fmt.Errorf("create fs for source: %w", err)
	}

	planFS, err := fsutil.NewFS(planDir)
	if err != nil {
		return fmt.Errorf("create fs for plan: %w", err)
	}

	frontendAttrs := map[string]string{
		"source": railpackFrontendImage,
	}
	if rpOpts.CacheKey != "" {
		frontendAttrs["build-arg:cache-key"] = rpOpts.CacheKey
	}
	if rpOpts.SecretsHash != "" {
		frontendAttrs["build-arg:secrets-hash"] = rpOpts.SecretsHash
	}

	secretsMap := make(map[string][]byte)
	for k, v := range rpOpts.Secrets {
		secretsMap[k] = []byte(v)
	}

	solveOpt := client.SolveOpt{
		Frontend:      "gateway.v0",
		FrontendAttrs: frontendAttrs,
		LocalMounts: map[string]fsutil.FS{
			"context":    srcFS,
			"dockerfile": planFS,
		},
		Session: []session.Attachable{
			secretsprovider.FromMap(secretsMap),
		},
		Exports: []client.ExportEntry{
			{
				Type: client.ExporterImage,
				Attrs: map[string]string{
					"name": opts.ImageRef,
					"push": "true",
				},
			},
		},
	}

	if opts.CacheRef != "" {
		solveOpt.CacheImports = []client.CacheOptionsEntry{
			{Type: "registry", Attrs: map[string]string{"ref": opts.CacheRef}},
		}
		solveOpt.CacheExports = []client.CacheOptionsEntry{
			{Type: "registry", Attrs: map[string]string{"ref": opts.CacheRef, "mode": "max"}},
		}
	}

	ch := make(chan *client.SolveStatus)
	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		_, err := c.Solve(egCtx, nil, solveOpt, ch)
		return err
	})

	eg.Go(func() error {
		for status := range ch {
			if opts.LokiLogger != nil {
				for _, v := range status.Vertexes {
					if v.Name != "" {
						opts.LokiLogger.Log(v.Name)
					}
				}
				for _, l := range status.Logs {
					opts.LokiLogger.Log(string(l.Data))
				}
			}
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("buildkit solve: %w", err)
	}

	if opts.LokiLogger != nil {
		_ = opts.LokiLogger.Flush(ctx)
	}

	return nil
}
