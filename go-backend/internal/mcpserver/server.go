package mcpserver

import (
	"log/slog"
	"net/http"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/deployments"
	"github.com/augustdev/autoclip/internal/dns"
	"github.com/augustdev/autoclip/internal/githubapp"
	"github.com/augustdev/autoclip/internal/internalgit"
	"github.com/augustdev/autoclip/internal/resources"
	"github.com/invopop/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var reflector = jsonschema.Reflector{
	DoNotReference: true,
}

func schemaFor[T any]() any {
	return reflector.Reflect(new(T))
}

type Server struct {
	mcpServer        *mcp.Server
	authService      *auth.Service
	deployService    *deployments.Service
	dnsService       *dns.Service
	resourcesService *resources.Service
	githubAppService *githubapp.Service
	internalGitSvc   *internalgit.Service
	logger           *slog.Logger
	lokiQueryURL     string
	lokiUsername     string
	lokiPassword     string
}

type LokiConfig struct {
	QueryURL string
	Username string
	Password string
}

func NewServer(authService *auth.Service, deployService *deployments.Service, dnsService *dns.Service, resourcesService *resources.Service, githubAppService *githubapp.Service, internalGitSvc *internalgit.Service, lokiCfg LokiConfig, logger *slog.Logger) *Server {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "Ink MCP",
			Title:   "Ink MCP - deploy your apps and servers on the cloud",
			Version: "0.1.0",
		},
		&mcp.ServerOptions{
			Instructions: "Ink MCP server has capabilities to deploy most single port apps NextJS, React, flask etc as well as many other servers and returns the URL of the deployed application. It can provision sqlite databases too. 99% of all apps and servers should be deployable with this MCP.",
		},
	)

	s := &Server{
		mcpServer:        mcpServer,
		authService:      authService,
		deployService:    deployService,
		dnsService:       dnsService,
		resourcesService: resourcesService,
		githubAppService: githubAppService,
		internalGitSvc:   internalGitSvc,
		logger:           logger,
		lokiQueryURL:     lokiCfg.QueryURL,
		lokiUsername:     lokiCfg.Username,
		lokiPassword:     lokiCfg.Password,
	}

	s.registerTools()

	logger.Info("MCP server initialized")
	return s
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "whoami",
		Description: "Get information about the authenticated user",
		InputSchema: schemaFor[WhoamiInput](),
	}, s.handleWhoami)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all projects for the authenticated user",
		InputSchema: schemaFor[ListProjectsInput](),
	}, s.handleListProjects)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_service",
		Description: "Create and deploy a service. Use host='ml.ink' (default) for private repos or host='github.com' for GitHub.",
		InputSchema: schemaFor[CreateServiceInput](),
	}, s.handleCreateService)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "redeploy_service",
		Description: "Redeploy an existing service to pull latest code",
		InputSchema: schemaFor[RedeployServiceInput](),
	}, s.handleRedeployService)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_services",
		Description: "List all deployed services",
		InputSchema: schemaFor[ListServicesInput](),
	}, s.handleListServices)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_resource",
		Description: "Create a new resource (e.g., sqlite database). Returns connection URL and auth token.",
		InputSchema: schemaFor[CreateResourceInput](),
	}, s.handleCreateResource)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_resources",
		Description: "List all resources (databases, etc.) for the authenticated user",
		InputSchema: schemaFor[ListResourcesInput](),
	}, s.handleListResources)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_resource",
		Description: "Get detailed information about a resource including connection URL and auth token",
		InputSchema: schemaFor[GetResourceDetailsInput](),
	}, s.handleGetResourceDetails)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_resource",
		Description: "Delete a resource (e.g., sqlite database). This permanently removes the resource.",
		InputSchema: schemaFor[DeleteResourceInput](),
	}, s.handleDeleteResource)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_service",
		Description: "Get detailed information about a deployed service. Returns deployment status (queued/building/deploying/active/failed/cancelled) and runtime status (running/deploying/failed/not_deployed). Use deploy_log_lines and runtime_log_lines to fetch logs.",
		InputSchema: schemaFor[GetServiceInput](),
	}, s.handleGetService)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_service",
		Description: "Delete a service. This permanently removes the deployment.",
		InputSchema: schemaFor[DeleteServiceInput](),
	}, s.handleDeleteService)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_repo",
		Description: "Create a git repository. Use host='ml.ink' (default) for instant private repos, or host='github.com' for GitHub. IMPORTANT: For ml.ink repos, the returned 'repo' field contains the actual repo name (with a random slug appended, e.g. 'myapp-xkcd'). Always use this returned repo name for create_service and other operations.",
		InputSchema: schemaFor[CreateRepoInput](),
	}, s.handleCreateRepo)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_git_token",
		Description: "Get a temporary git token to push code. Example: name='myapp', host='ml.ink' (default) or host='github.com'.",
		InputSchema: schemaFor[GetGitTokenInput](),
	}, s.handleGetGitToken)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "add_custom_domain",
		Description: "Attach a custom domain to a service. Returns DNS records to configure.",
		InputSchema: schemaFor[AddCustomDomainInput](),
	}, s.handleAddCustomDomain)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_custom_domain",
		Description: "Remove a custom domain from a service.",
		InputSchema: schemaFor[RemoveCustomDomainInput](),
	}, s.handleRemoveCustomDomain)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_delegations",
		Description: "List all delegated zones with their status. Zone delegation can be set up at https://ml.ink/dns",
		InputSchema: schemaFor[ListDelegationsInput](),
	}, s.handleListDelegations)
}

func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})
}
