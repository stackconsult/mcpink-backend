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
	Repo   string `json:"repo" jsonschema:"description=Repository name (e.g. 'myapp')"`
	Host   string `json:"host,omitempty" jsonschema:"description=Git host,enum=ml.ink,enum=github.com,default=ml.ink"`
	Branch string `json:"branch,omitempty" jsonschema:"description=Branch to deploy,default=main"`
	Name   string `json:"name" jsonschema:"description=Name for the deployment"`

	Project   string   `json:"project,omitempty" jsonschema:"description=Project name,default=default"`
	BuildPack string   `json:"build_pack,omitempty" jsonschema:"description=Build pack to use,enum=railpack,enum=dockerfile,enum=static,enum=dockercompose,default=railpack"`
	Port      int      `json:"port,omitempty" jsonschema:"description=Port the application listens on,default=3000"`
	EnvVars   []EnvVar `json:"env_vars,omitempty" jsonschema:"description=Environment variables"`

	Memory string `json:"memory,omitempty" jsonschema:"description=Memory limit,enum=128Mi,enum=256Mi,enum=512Mi,enum=1024Mi,enum=2048Mi,enum=4096Mi,default=256Mi"`
	CPU    string `json:"cpu,omitempty" jsonschema:"description=CPU cores,enum=0.5,enum=1,enum=2,enum=4,default=0.5"`

	BuildCommand string `json:"build_command,omitempty" jsonschema:"description=Custom build command (overrides auto-detected). Only used with build_pack=railpack."`
	StartCommand string `json:"start_command,omitempty" jsonschema:"description=Custom start command (overrides auto-detected). Only used with build_pack=railpack."`

	PublishDirectory string `json:"publish_directory,omitempty" jsonschema:"description=Directory containing built static files (e.g. 'dist'). When set with build_pack=railpack the app is built then served as static files via nginx."`

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

type BuildProgress struct {
	Stage       int    `json:"stage"`
	TotalStages int    `json:"total_stages"`
	Message     string `json:"message,omitempty"`
}

type ServiceInfo struct {
	ServiceID     string         `json:"service_id"`
	Name          string         `json:"name"`
	Status        string         `json:"status"`
	Repo          string         `json:"repo"`
	URL           *string        `json:"url,omitempty"`
	CommitHash    *string        `json:"commit_hash,omitempty"`
	BuildProgress *BuildProgress `json:"build_progress,omitempty"`
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

type GetServiceOutput struct {
	ServiceID      string         `json:"service_id"`
	Name           string         `json:"name"`
	Project        string         `json:"project"`
	Repo           string         `json:"repo"`
	Branch         string         `json:"branch"`
	CommitHash     string         `json:"commit_hash,omitempty"`
	BuildStatus    string         `json:"build_status"`
	RuntimeStatus  string         `json:"runtime_status"`
	URL            *string        `json:"url,omitempty"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	ErrorMessage   *string        `json:"error_message,omitempty"`
	EnvVars        []EnvVarInfo   `json:"env_vars,omitempty"`
	DeploymentLogs string         `json:"deployment_logs,omitempty"`
	RuntimeLogs    string         `json:"runtime_logs,omitempty"`
	BuildProgress  *BuildProgress `json:"build_progress,omitempty"`
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
}

type CreateRepoOutput struct {
	Repo      string `json:"repo"`
	GitRemote string `json:"git_remote"`
	ExpiresAt string `json:"expires_at"`
	Message   string `json:"message"`
}

type GetGitTokenInput struct {
	Name string `json:"name" jsonschema:"description=Repository name (e.g. 'myapp' not 'username/myapp')"`
	Host string `json:"host,omitempty" jsonschema:"description=Git host,enum=ml.ink,enum=github.com,default=ml.ink"`
}

type GetGitTokenOutput struct {
	GitRemote string `json:"git_remote"`
	ExpiresAt string `json:"expires_at"`
}
