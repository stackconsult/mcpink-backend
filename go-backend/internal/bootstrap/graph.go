package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/authz"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/dns"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/graph"
	"github.com/augustdev/autoclip/internal/graph/dataloader"
	"github.com/augustdev/autoclip/internal/prometheus"
	"github.com/augustdev/autoclip/internal/storage/pg"
	deploymentsdb "github.com/augustdev/autoclip/internal/storage/pg/generated/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsdb"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/resources"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/turso"
	"github.com/augustdev/autoclip/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"go.uber.org/fx"
)

type GraphQLAPIConfig struct {
	Port                string
	EnableIntrospection bool
}

func NewResolver(
	pgdb *pg.DB,
	logger *slog.Logger,
	authService *auth.Service,
	deployService *deployments.Service,
	dnsService *dns.Service,
	githubAppService *githubapp.Service,
	serviceQueries services.Querier,
	projectQueries projects.Querier,
	resourceQueries resources.Querier,
	firebaseAuth *firebaseauth.Client,
	prometheusClient *prometheus.Client,
) *graph.Resolver {
	return &graph.Resolver{
		Db:               pgdb,
		Logger:           logger,
		AuthService:      authService,
		DeployService:    deployService,
		DNSService:       dnsService,
		GitHubAppService: githubAppService,
		ServiceQueries:   serviceQueries,
		ProjectQueries:   projectQueries,
		ResourceQueries:  resourceQueries,
		FirebaseAuth:     firebaseAuth,
		PrometheusClient: prometheusClient,
	}
}

func NewLoaderDeps(
	serviceQueries services.Querier,
	deploymentQueries deploymentsdb.Querier,
	dnsQueries dnsdb.Querier,
) *dataloader.LoaderDeps {
	return &dataloader.LoaderDeps{
		ServiceQueries:    serviceQueries,
		DeploymentQueries: deploymentQueries,
		DnsQueries:        dnsQueries,
	}
}

func NewGraphQLRouter(
	logger *slog.Logger,
	resolver *graph.Resolver,
	tokenValidator authz.TokenValidator,
	db *pg.DB,
	authRouter chi.Router,
	authConfig auth.Config,
	authService *auth.Service,
	authHandlers *auth.Handlers,
	webhookHandlers *webhooks.Handlers,
	loaderDeps *dataloader.LoaderDeps,
) *chi.Mux {
	router := chi.NewRouter()

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type", "Accept"},
	}).Handler

	router.Use(corsMiddleware)
	router.Use(dataloader.Middleware(loaderDeps))

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	router.Mount("/", authRouter)

	srv := handler.New(gqlSchema(resolver))

	srv.AddTransport(transport.SSE{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{
		MaxUploadSize: 15 * 1024 * 1024,
		MaxMemory:     15 * 1024 * 1024,
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	})

	srv.Use(extension.Introspection{})

	srv.AroundResponses(func(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
		resp := next(ctx)

		if resp != nil && resp.Errors != nil && len(resp.Errors) > 0 {
			if ctx.Err() == context.Canceled {
				return resp
			}
			oc := graphql.GetOperationContext(ctx)
			logger.Error(
				"gql error",
				"operation_name", oc.OperationName,
				"raw_query", oc.RawQuery,
				"variables", oc.Variables,
				"errors", resp.Errors,
			)
		}

		return resp
	})

	// Bearer-only auth middleware (no cookie fallback)
	authMiddleware := authz.MiddlewareWithConfig(srv, tokenValidator.ValidateToken, logger, nil)
	router.Handle("/", playground.Handler("GraphQL playground", "/graphql"))
	router.Handle("/graphql", authMiddleware)

	// Authenticated endpoint for GitHub connect (requires Bearer token)
	getUserID := func(r *http.Request) string {
		sc, err := authz.ForErr(r.Context())
		if err != nil {
			return ""
		}
		return sc.GetUserID()
	}
	router.With(authz.NewAuthMiddleware(tokenValidator, logger)).Post("/auth/github/connect", authHandlers.HandleGitHubConnect(getUserID))

	webhookHandlers.RegisterRoutes(router)

	return router
}

func gqlSchema(resolver *graph.Resolver) graphql.ExecutableSchema {
	c := graph.Config{
		Resolvers: resolver,
		Directives: graph.DirectiveRoot{
			IsAuthenticated: authz.IsAuthenticatedDirective,
		},
	}
	return graph.NewExecutableSchema(c)
}

func NewTursoClient(config turso.Config, logger *slog.Logger) (*turso.Client, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("turso: APIKey is required")
	}
	if config.OrgSlug == "" {
		return nil, fmt.Errorf("turso: OrgSlug is required")
	}

	logger.Info("Turso client initialized")
	return turso.NewClient(config, logger), nil
}

func StartServer(lc fx.Lifecycle, router *chi.Mux, config GraphQLAPIConfig, logger *slog.Logger) {
	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting server", "port", config.Port, "url", "http://localhost:"+config.Port+"/")
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("Server failed to start", "error", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down HTTP server, draining connections...")
			server.SetKeepAlivesEnabled(false)

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := server.Shutdown(shutdownCtx)
			if shutdownCtx.Err() != nil {
				logger.Warn("Graceful shutdown timed out after 2s, forcing close")
				if closeErr := server.Close(); closeErr != nil {
					logger.Error("Error force-closing server", "error", closeErr)
					return closeErr
				}
				logger.Info("HTTP server force-closed")
				return nil
			}
			if err != nil {
				logger.Error("Error shutting down server", "error", err)
				return err
			}
			logger.Info("HTTP server shut down gracefully")
			return nil
		},
	})
}
