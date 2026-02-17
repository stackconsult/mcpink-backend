package mcpserver

type WhoamiInput struct{}

type WhoamiOutput struct {
	UserID         string  `json:"user_id"`
	GitHubUsername *string `json:"github_username,omitempty"`
	AvatarURL      *string `json:"avatar_url,omitempty"`
	HasGitHubApp   bool    `json:"has_github_app"`
}

type EnvVar struct {
	Key   string `json:"key" jsonschema:"description=Environment variable name"`
	Value string `json:"value" jsonschema:"description=Environment variable value"`
}

type CreateServiceInput struct {
	Repo   string `json:"repo" jsonschema:"description=Repository name as returned by create_repo (e.g. 'myapp-xkcd'). For ml.ink repos this includes a random slug — use the exact 'repo' value from create_repo response."`
	Host   string `json:"host,omitempty" jsonschema:"description=Git host,enum=ml.ink,enum=github.com,default=ml.ink"`
	Branch string `json:"branch,omitempty" jsonschema:"description=Branch to deploy,default=main"`
	Name   string `json:"name" jsonschema:"description=Name for the deployment"`
	Region string `json:"region,omitempty" jsonschema:"description=Cluster region to deploy to,enum=eu-central-1,default=eu-central-1"`

	Project   string   `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
	BuildPack string   `json:"build_pack,omitempty" jsonschema:"description=Build pack to use. 'railpack' (default) auto-detects and builds most apps. 'static' serves files as-is with no build step. 'dockerfile' uses a custom Dockerfile. Use 'railpack' with publish_directory for Vite/React/Vue SPAs that need a build step then static serving via nginx.,enum=railpack,enum=dockerfile,enum=static,enum=dockercompose,default=railpack"`
	Port      *int     `json:"port,omitempty" jsonschema:"description=Port the application listens on"`
	EnvVars   []EnvVar `json:"env_vars,omitempty" jsonschema:"description=Environment variables"`

	Memory string `json:"memory,omitempty" jsonschema:"description=Memory limit. 256Mi for most apps; 512Mi for heavier apps.,enum=256Mi,enum=512Mi,enum=1024Mi,enum=2048Mi,enum=4096Mi,default=256Mi"`
	VCPUs  string `json:"vcpus,omitempty" jsonschema:"description=vCPUs,enum=0.5,enum=1,enum=2,enum=4,default=0.5"`

	BuildCommand string `json:"build_command,omitempty" jsonschema:"description=Custom build command (overrides auto-detected). Only used with build_pack=railpack."`
	StartCommand string `json:"start_command,omitempty" jsonschema:"description=Custom start command (overrides auto-detected). Only used with build_pack=railpack."`

	PublishDirectory string `json:"publish_directory,omitempty" jsonschema:"description=Directory containing built static files (e.g. 'dist'). When set with build_pack=railpack the app is built then served as static files via nginx. Recommended for Vite/React/Vue SPAs."`

	RootDirectory  string `json:"root_directory,omitempty" jsonschema:"description=Subdirectory within the repo to use as build context (e.g. 'frontend' or 'services/api'). For monorepo deployments."`
	DockerfilePath string `json:"dockerfile_path,omitempty" jsonschema:"description=Path to Dockerfile relative to root_directory (e.g. 'worker.Dockerfile' or 'build/Dockerfile'). Only used with build_pack=dockerfile."`
}

type CreateServiceOutput struct {
	ServiceID  string `json:"service_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Repo       string `json:"repo"`
	CommitHash string `json:"commit_hash,omitempty"`
	Message    string `json:"message"`
}

type RedeployServiceInput struct {
	Name    string `json:"name" jsonschema:"description=Name of the service to redeploy (required)"`
	Project string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
}

type RedeployServiceOutput struct {
	ServiceID  string `json:"service_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CommitHash string `json:"commit_hash,omitempty"`
	Message    string `json:"message"`
}

type ListServicesInput struct{}

type ListServicesOutput struct {
	Services []ServiceInfo `json:"services"`
}

type DeploymentDetails struct {
	Status       string  `json:"status"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Logs         string  `json:"logs,omitempty"`
}

type RuntimeDetails struct {
	Status string `json:"status"`
	Logs   string `json:"logs,omitempty"`
}

type ServiceInfo struct {
	ServiceID  string             `json:"service_id"`
	Name       string             `json:"name"`
	Repo       string             `json:"repo"`
	URL        *string            `json:"url,omitempty"`
	Deployment *DeploymentDetails `json:"deployment,omitempty"`
}

const (
	DefaultPort      = 3000
	DefaultBuildPack = "railpack"
)

type CreateResourceInput struct {
	Name   string `json:"name" jsonschema:"description=Name for the resource (required)"`
	Type   string `json:"type,omitempty" jsonschema:"description=Resource type,enum=sqlite,default=sqlite"`
	Size   string `json:"size,omitempty" jsonschema:"description=Size limit for databases,default=100mb"`
	Region string `json:"region,omitempty" jsonschema:"description=Region,enum=eu-west,default=eu-west"`
}

type CreateResourceOutput struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Region     string `json:"region"`
	URL        string `json:"database_url"`
	AuthToken  string `json:"auth_token"`
	Status     string `json:"status"`
}

type ListResourcesInput struct{}

type ListResourcesOutput struct {
	Resources []ResourceInfo `json:"resources"`
}

type ResourceInfo struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Region     string `json:"region"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type GetResourceDetailsInput struct {
	Name string `json:"name" jsonschema:"description=Resource name (required)"`
}

type GetResourceDetailsOutput struct {
	ResourceID  string `json:"resource_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Region      string `json:"region"`
	DatabaseURL string `json:"database_url"`
	AuthToken   string `json:"auth_token"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

const (
	DefaultRegion = "eu-west"
	DefaultDBType = "sqlite"
	DefaultDBSize = "100mb"
)

type GetServiceInput struct {
	Name            string `json:"name" jsonschema:"description=Service name (required)"`
	Project         string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
	IncludeEnv      bool   `json:"include_env,omitempty" jsonschema:"description=Include environment variables,default=false"`
	DeployLogLines  int    `json:"deploy_log_lines,omitempty" jsonschema:"description=Number of deployment log lines to fetch (max: 500),default=0"`
	RuntimeLogLines int    `json:"runtime_log_lines,omitempty" jsonschema:"description=Number of runtime log lines to fetch (max: 500),default=0"`
}

type CustomDomainDetails struct {
	Domain string  `json:"domain"`
	Status string  `json:"status"`
	Error  *string `json:"error,omitempty"`
}

type GetServiceOutput struct {
	Deployment   *DeploymentDetails   `json:"deployment,omitempty"`
	Runtime      *RuntimeDetails      `json:"runtime,omitempty"`
	ServiceID    string               `json:"service_id"`
	Name         string               `json:"name"`
	Project      string               `json:"project"`
	Repo         string               `json:"repo"`
	Branch       string               `json:"branch"`
	URL          *string              `json:"url,omitempty"`
	CreatedAt    string               `json:"created_at"`
	UpdatedAt    string               `json:"updated_at"`
	EnvVars      []EnvVarInfo         `json:"env_vars,omitempty"`
	CustomDomain *CustomDomainDetails `json:"custom_domain,omitempty"`
}

type EnvVarInfo struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

const MaxLogLines = 500

type DeleteServiceInput struct {
	Name    string `json:"name" jsonschema:"description=Name of the service to delete (required)"`
	Project string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
}

type DeleteServiceOutput struct {
	ServiceID string `json:"service_id"`
	Name      string `json:"name"`
	Message   string `json:"message"`
}

type DeleteResourceInput struct {
	Name string `json:"name" jsonschema:"description=Resource name (required)"`
}

type DeleteResourceOutput struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// Unified repo tools

type CreateRepoInput struct {
	Name        string `json:"name" jsonschema:"description=Repository name (e.g. 'myapp' not 'username/myapp')"`
	Host        string `json:"host,omitempty" jsonschema:"description=Git host,enum=ml.ink,enum=github.com,default=ml.ink"`
	Description string `json:"description,omitempty" jsonschema:"description=Repository description"`
	Project     string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
}

type CreateRepoOutput struct {
	Repo      string `json:"repo"`
	GitRemote string `json:"git_remote"`
	ExpiresAt string `json:"expires_at"`
	Message   string `json:"message"`
}

type GetGitTokenInput struct {
	Name    string `json:"name" jsonschema:"description=Repository name (e.g. 'myapp' not 'username/myapp')"`
	Host    string `json:"host,omitempty" jsonschema:"description=Git host,enum=ml.ink,enum=github.com,default=ml.ink"`
	Project string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
}

type GetGitTokenOutput struct {
	GitRemote string `json:"git_remote"`
	ExpiresAt string `json:"expires_at"`
}

// Custom domain (backed by delegated zones)

type AddCustomDomainInput struct {
	Name    string `json:"name" jsonschema:"description=Name of the service to attach a custom domain to"`
	Domain  string `json:"domain" jsonschema:"description=Custom domain to attach (e.g. 'api.apps.example.com'). Must be under a delegated zone."`
	Project string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
}

type AddCustomDomainOutput struct {
	ServiceID string `json:"service_id"`
	Domain    string `json:"domain"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type RemoveCustomDomainInput struct {
	Name    string `json:"name" jsonschema:"description=Name of the service to remove custom domain from"`
	Project string `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
}

type RemoveCustomDomainOutput struct {
	ServiceID string `json:"service_id"`
	Message   string `json:"message"`
}

// Delegation tools

type DelegateZoneInput struct {
	Zone string `json:"zone" jsonschema:"description=Subdomain zone to delegate (e.g. 'apps.example.com')"`
}

type MCPDNSRecord struct {
	Host     string `json:"host"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	Verified bool   `json:"verified"`
}

type DelegateZoneOutput struct {
	ZoneID     string         `json:"zone_id"`
	Zone       string         `json:"zone"`
	Status     string         `json:"status"`
	DNSRecords []MCPDNSRecord `json:"dns_records"`
}

type VerifyDelegationInput struct {
	Zone string `json:"zone" jsonschema:"description=Zone to verify (e.g. 'apps.example.com')"`
}

type VerifyDelegationOutput struct {
	ZoneID     string         `json:"zone_id"`
	Zone       string         `json:"zone"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	DNSRecords []MCPDNSRecord `json:"dns_records,omitempty"`
}

type RemoveDelegationInput struct {
	Zone string `json:"zone" jsonschema:"description=Zone to remove (e.g. 'apps.example.com')"`
}

type RemoveDelegationOutput struct {
	ZoneID  string `json:"zone_id"`
	Message string `json:"message"`
}

type ListDelegationsInput struct{}

type DelegationInfo struct {
	ZoneID    string  `json:"zone_id"`
	Zone      string  `json:"zone"`
	Status    string  `json:"status"`
	Error     *string `json:"error,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type ListDelegationsOutput struct {
	Delegations []DelegationInfo `json:"delegations"`
}
