package mcpserver

type WhoamiInput struct{}

type WhoamiOutput struct {
	UserID         string  `json:"user_id"`
	GitHubUsername string  `json:"github_username"`
	AvatarURL      *string `json:"avatar_url,omitempty"`
	HasGitHubApp   bool    `json:"has_github_app"`
}

type EnvVar struct {
	Key   string `json:"key" jsonschema:"Environment variable name"`
	Value string `json:"value" jsonschema:"Environment variable value"`
}

type CreateAppInput struct {
	Repo   string `json:"repo" jsonschema:"GitHub repository in owner/repo format"`
	Branch string `json:"branch" jsonschema:"Branch to deploy"`
	Name   string `json:"name" jsonschema:"Name for the deployment"`

	Project   string   `json:"project,omitempty" jsonschema:"Project name to deploy to (default: user's default project)"`
	BuildPack string   `json:"build_pack,omitempty" jsonschema:"Build pack to use: nixpacks (default) or dockerfile or static or dockercompose"`
	Port      int      `json:"port,omitempty" jsonschema:"Port the application listens on (default: 3000)"`
	EnvVars   []EnvVar `json:"env_vars,omitempty" jsonschema:"Environment variables"`

	Memory string `json:"memory,omitempty" jsonschema:"Memory limit (e.g. 512m or 1g)"`
	CPU    string `json:"cpu,omitempty" jsonschema:"CPU limit (e.g. 0.5 or 1)"`

	InstallCommand string `json:"install_command,omitempty" jsonschema:"Custom install command"`
	BuildCommand   string `json:"build_command,omitempty" jsonschema:"Custom build command"`
	StartCommand   string `json:"start_command,omitempty" jsonschema:"Custom start command"`

	InstantDeploy *bool `json:"instant_deploy,omitempty" jsonschema:"Start deployment immediately (default: true)"`
}

type CreateAppOutput struct {
	AppID      string `json:"app_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Repo       string `json:"repo"`
	CommitHash string `json:"commit_hash,omitempty"`
	Message    string `json:"message"`
}

type RedeployInput struct {
	Name    string `json:"name" jsonschema:"Name of the app to redeploy (required)"`
	Project string `json:"project,omitempty" jsonschema:"Project name (default: default)"`
}

type RedeployOutput struct {
	AppID      string `json:"app_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CommitHash string `json:"commit_hash,omitempty"`
	Message    string `json:"message"`
}

type ListAppsInput struct{}

type ListAppsOutput struct {
	Apps []AppInfo `json:"apps"`
}

type AppInfo struct {
	AppID      string  `json:"app_id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Repo       string  `json:"repo"`
	URL        *string `json:"url,omitempty"`
	CommitHash *string `json:"commit_hash,omitempty"`
}

const (
	DefaultPort      = 3000
	DefaultBuildPack = "nixpacks"
)

type CreateResourceInput struct {
	Name       string `json:"name" jsonschema:"Name for the resource (required)"`
	Type       string `json:"type,omitempty" jsonschema:"Resource type (default: sqlite, only option for now)"`
	Size       string `json:"size,omitempty" jsonschema:"Size limit for databases (default: 100mb)"`
	Region     string `json:"region,omitempty" jsonschema:"Region (default: eu-west, only option for now)"`
	ProjectRef string `json:"project_ref,omitempty" jsonschema:"Project reference (default: default)"`
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

type ListResourcesInput struct {
	Project string `json:"project,omitempty" jsonschema:"Project name (default: default)"`
}

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
	Name    string `json:"name" jsonschema:"Resource name (required)"`
	Project string `json:"project,omitempty" jsonschema:"Project name (default: default)"`
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
	DefaultRegion     = "eu-west"
	DefaultProjectRef = "default"
	DefaultDBType     = "sqlite"
	DefaultDBSize     = "100mb"
)

type CreateGitHubRepoInput struct {
	Name        string `json:"name" jsonschema:"Repository name (required)"`
	Private     *bool  `json:"private,omitempty" jsonschema:"Make repository private (default: true)"`
	Description string `json:"description,omitempty" jsonschema:"Repository description"`
}

type CreateGitHubRepoOutput struct {
	RepoFullName string `json:"repo_full_name"`
	AccessToken  string `json:"access_token"`
}

type GetGitHubPushTokenInput struct {
	Repo string `json:"repo" jsonschema:"GitHub repository in owner/repo format (required)"`
}

type GetGitHubPushTokenOutput struct {
	AccessToken      string `json:"access_token"`
	ExpiresAt        string `json:"expires_at"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
}

type DebugGitHubAppInput struct{}

type DebugGitHubAppOutput struct {
	InstallationID      int64             `json:"installation_id"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
}

type GetAppDetailsInput struct {
	Name            string `json:"name" jsonschema:"App name (required)"`
	Project         string `json:"project,omitempty" jsonschema:"Project name (default: user's default project)"`
	IncludeEnv      bool   `json:"include_env,omitempty" jsonschema:"Include environment variables (default: false)"`
	DeployLogLines  int    `json:"deploy_log_lines,omitempty" jsonschema:"Number of deployment log lines to fetch (max: 500, default: 0)"`
	RuntimeLogLines int    `json:"runtime_log_lines,omitempty" jsonschema:"Number of runtime log lines to fetch (max: 500, default: 0)"`
}

type GetAppDetailsOutput struct {
	AppID         string       `json:"app_id"`
	Name          string       `json:"name"`
	Project       string       `json:"project"`
	Repo          string       `json:"repo"`
	Branch        string       `json:"branch"`
	CommitHash    string       `json:"commit_hash,omitempty"`
	BuildStatus   string       `json:"build_status"`
	RuntimeStatus string       `json:"runtime_status"`
	URL           *string      `json:"url,omitempty"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	ErrorMessage  *string      `json:"error_message,omitempty"`
	EnvVars        []EnvVarInfo `json:"env_vars,omitempty"`
	DeploymentLogs string       `json:"deployment_logs,omitempty"`
	RuntimeLogs    string       `json:"runtime_logs,omitempty"`
}

type EnvVarInfo struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

type LogLine struct {
	Timestamp string `json:"timestamp,omitempty"`
	Stream    string `json:"stream,omitempty"`
	Message   string `json:"message"`
}

const MaxLogLines = 500

type DeleteAppInput struct {
	Name    string `json:"name" jsonschema:"Name of the app to delete (required)"`
	Project string `json:"project,omitempty" jsonschema:"Project name (default: user's default project)"`
}

type DeleteAppOutput struct {
	AppID   string `json:"app_id"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type DeleteResourceInput struct {
	Name    string `json:"name" jsonschema:"Resource name (required)"`
	Project string `json:"project,omitempty" jsonschema:"Project name (default: default)"`
}

type DeleteResourceOutput struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}
