package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/authz"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/graph"
	"github.com/augustdev/autoclip/internal/mcpserver"
	"github.com/augustdev/autoclip/internal/storage/pg"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"go.uber.org/fx"
)

type GraphQLAPIConfig struct {
	Port                string
	ValidatorType       string
	JWTJWKSURL          string
	EnableIntrospection bool
}

func NewResolver(
	pgdb *pg.DB,
	logger *slog.Logger,
	authService *auth.Service,
	githubAppService *githubapp.Service,
	coolifyClient *coolify.Client,
) *graph.Resolver {
	return &graph.Resolver{
		Db:               pgdb,
		Logger:           logger,
		AuthService:      authService,
		GitHubAppService: githubAppService,
		CoolifyClient:    coolifyClient,
	}
}

type GraphQLRouterParams struct {
	fx.In

	Logger         *slog.Logger
	Resolver       *graph.Resolver
	TokenValidator authz.TokenValidator
	DB             *pg.DB
	Config         GraphQLAPIConfig
}

func NewGraphQLRouter(
	logger *slog.Logger,
	resolver *graph.Resolver,
	tokenValidator authz.TokenValidator,
	db *pg.DB,
	authRouter chi.Router,
	authConfig auth.Config,
	authService *auth.Service,
	mcpServer *mcpserver.Server,
) *chi.Mux {
	router := chi.NewRouter()

	corsMiddleware := cors.New(cors.Options{
		AllowCredentials: true,
		AllowedOrigins:   []string{authConfig.FrontendURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept"},
		Debug:            false,
	}).Handler

	router.Use(corsMiddleware)

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

	cookieValidateFunc := func(token string) (string, error) {
		userID, err := authService.ValidateJWT(token)
		if err != nil {
			return "", err
		}
		return userID, nil
	}

	authMiddleware := authz.MiddlewareWithConfig(srv, tokenValidator.ValidateToken, logger, &authz.MiddlewareConfig{
		Cookie: &authz.CookieConfig{
			CookieName:   authConfig.SessionCookieName,
			ValidateFunc: cookieValidateFunc,
		},
	})
	router.Handle("/", playground.Handler("GraphQL playground", "/graphql"))
	router.Handle("/graphql", authMiddleware)

	if mcpServer != nil {
		router.Mount("/mcp", mcpserver.AuthMiddleware(authService, logger, mcpServer.Handler()))
		logger.Info("MCP server mounted", "path", "/mcp")
	}

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

func NewCoolifyClient(config coolify.Config, logger *slog.Logger) *coolify.Client {
	if config.BaseURL == "" || config.Token == "" {
		logger.Info("Coolify client not configured, skipping")
		return nil
	}

	client, err := coolify.NewClient(config)
	if err != nil {
		logger.Error("failed to create Coolify client", "error", err)
		return nil
	}
	return client
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
			logger.Info("Stopping server")
			return server.Shutdown(ctx)
		},
	})
}
