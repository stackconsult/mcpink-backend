package mcpserver

import (
	"log/slog"
	"net/http"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/coolify"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/logs"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	mcpServer        *mcp.Server
	authService      *auth.Service
	coolifyClient    *coolify.Client
	deployService    *deployments.Service
	resourcesService *resources.Service
	githubAppService *githubapp.Service
	logProvider      logs.Provider
	logger           *slog.Logger
}

func NewServer(authService *auth.Service, coolifyClient *coolify.Client, deployService *deployments.Service, resourcesService *resources.Service, githubAppService *githubapp.Service, logProvider logs.Provider, logger *slog.Logger) *Server {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "Ink MCP",
			Title:   "Ink MCP - deploy your apps and servers on the cloud",
			Version: "0.1.0",
		},
		&mcp.ServerOptions{
			Instructions: "This server has capabilities to deploy most single port apps NextJS, React, flask etc as well as many other servers and returns the URL of the deployed application. It can provision sqlite databases too. 99% of apps should be deployable with this MCP.",
		},
	)

	s := &Server{
		mcpServer:        mcpServer,
		authService:      authService,
		coolifyClient:    coolifyClient,
		deployService:    deployService,
		resourcesService: resourcesService,
		githubAppService: githubAppService,
		logProvider:      logProvider,
		logger:           logger,
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
		Name:        "create_app",
		Description: "Create and deploy an application from a GitHub repository",
	}, s.handleCreateApp)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "redeploy",
		Description: "Redeploy an existing application to pull latest code",
	}, s.handleRedeploy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_apps",
		Description: "List all deployed applications",
	}, s.handleListApps)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_resource",
		Description: "Create a new resource (e.g., sqlite database). Returns connection URL and auth token.",
	}, s.handleCreateResource)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_resources",
		Description: "List all resources (databases, etc.) for the authenticated user",
	}, s.handleListResources)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_resource",
		Description: "Get detailed information about a resource including connection URL and auth token",
	}, s.handleGetResourceDetails)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_resource",
		Description: "Delete a resource (e.g., sqlite database). This permanently removes the resource.",
	}, s.handleDeleteResource)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_github_repo",
		Description: "Create a new GitHub repository and return a temporary access token for pushing code. Requires OAuth `repo` scope. This tool should only be used if `gh` cli is not installed or configured.",
	}, s.handleCreateGitHubRepo)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "github_get_push_token",
		Description: "Get a temporary access token for pushing to an existing GitHub repository. Requires GitHub App to be installed and have access to the repository. This tool should only be used if `git` is not configured.",
	}, s.handleGetGitHubPushToken)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_app_details",
		Description: "Get detailed information about a deployed application, optionally including environment variables and logs",
	}, s.handleGetAppDetails)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_app",
		Description: "Delete an application. This removes it from Coolify and marks it as deleted in the database.",
	}, s.handleDeleteApp)
}

func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})
}
