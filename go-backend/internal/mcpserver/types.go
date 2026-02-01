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

type DeployInput struct {
	Repo   string `json:"repo" jsonschema:"GitHub repository in owner/repo format"`
	Branch string `json:"branch" jsonschema:"Branch to deploy"`

	Name      string   `json:"name,omitempty" jsonschema:"Optional name for the deployment"`
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

type DeployOutput struct {
	DeploymentUUID string  `json:"deployment_uuid"`
	UUID           string  `json:"uuid"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	Message        string  `json:"message"`
	FQDN           *string `json:"fqdn,omitempty"`
}

const (
	DefaultPort      = 3000
	DefaultBuildPack = "nixpacks"
)
