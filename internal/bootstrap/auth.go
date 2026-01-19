package bootstrap

import (
	"github.com/go-chi/chi/v5"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/mcp"
)

func NewAuthRouter(authHandlers *auth.Handlers, mcpHandlers *mcp.Handlers) chi.Router {
	router := chi.NewRouter()
	authHandlers.RegisterRoutes(router)
	mcpHandlers.RegisterRoutes(router)
	return router
}
