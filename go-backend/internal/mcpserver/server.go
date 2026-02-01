package mcpserver

import (
	"log/slog"
	"net/http"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	mcpServer     *mcp.Server
	authService   *auth.Service
	coolifyClient *coolify.Client
	deployService *deployments.Service
	logger        *slog.Logger
}

func NewServer(authService *auth.Service, coolifyClient *coolify.Client, deployService *deployments.Service, logger *slog.Logger) *Server {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "deploy-mcp",
			Version: "1.0.0",
		},
		nil,
	)

	s := &Server{
		mcpServer:     mcpServer,
		authService:   authService,
		coolifyClient: coolifyClient,
		deployService: deployService,
		logger:        logger,
	}

	s.registerTools()

	logger.Info("MCP server initialized")
	return s
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "whoami",
		Description: "Get information about the authenticated user",
	}, s.handleWhoami)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "deploy",
		Description: "Deploy an application from a GitHub repository",
	}, s.handleDeploy)
}

func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)
}
