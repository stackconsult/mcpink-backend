package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/augustdev/autoclip/internal/bootstrap"
	"github.com/augustdev/autoclip/internal/statuschecker"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"k8s.io/client-go/kubernetes"
)

type config struct {
	fx.Out

	StatusChecker statuschecker.Config
}

func main() {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.Provide(
			bootstrap.NewLogger,
			bootstrap.LoadConfig[config],
			bootstrap.NewK8sClient,
			statuschecker.New,
		),
		fx.Invoke(
			startStatusChecker,
		),
	).Run()
}

func startStatusChecker(lc fx.Lifecycle, checker *statuschecker.Checker, cfg statuschecker.Config, k8s kubernetes.Interface, logger *slog.Logger) {
	router := chi.NewRouter()
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting status checker", "port", cfg.Port, "interval", cfg.Interval, "checks", len(cfg.Checks))
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("Status checker HTTP server failed", "error", err)
				}
			}()
			go checker.Run(context.Background())
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down status checker...")
			checker.Stop()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return server.Shutdown(shutdownCtx)
		},
	})
}
