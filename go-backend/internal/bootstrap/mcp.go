package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/authz"
	"github.com/augustdev/autoclip/internal/mcp_oauth"
	"github.com/augustdev/autoclip/internal/mcpserver"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"go.uber.org/fx"
)

type MCPAPIConfig struct {
	Port string
}

func NewMCPRouter(
	logger *slog.Logger,
	tokenValidator authz.TokenValidator,
	authService *auth.Service,
	mcpServer *mcpserver.Server,
	mcpOAuthHandlers *mcp_oauth.Handlers,
	mcpOAuthConfig mcp_oauth.Config,
) *chi.Mux {
	router := chi.NewRouter()

	corsMiddleware := cors.New(cors.Options{
		AllowCredentials: true,
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept"},
	}).Handler
	router.Use(corsMiddleware)

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mcpOAuthHandlers.RegisterRoutes(router, authz.NewAuthMiddleware(tokenValidator, logger))

	router.Mount("/", mcpserver.AuthMiddleware(authService, logger, mcpOAuthConfig.Issuer, mcpServer.Handler()))

	return router
}

func StartMCPServer(lc fx.Lifecycle, router *chi.Mux, config MCPAPIConfig, logger *slog.Logger) {
	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting MCP server", "port", config.Port, "url", "http://localhost:"+config.Port+"/")
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("MCP server failed to start", "error", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down MCP server, draining connections...")
			server.SetKeepAlivesEnabled(false)

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := server.Shutdown(shutdownCtx)
			if shutdownCtx.Err() != nil {
				logger.Warn("MCP graceful shutdown timed out after 2s, forcing close")
				if closeErr := server.Close(); closeErr != nil {
					logger.Error("Error force-closing MCP server", "error", closeErr)
					return closeErr
				}
				logger.Info("MCP server force-closed")
				return nil
			}
			if err != nil {
				logger.Error("Error shutting down MCP server", "error", err)
				return err
			}
			logger.Info("MCP server shut down gracefully")
			return nil
		},
	})
}
