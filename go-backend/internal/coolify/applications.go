package coolify

import (
	"context"
	"fmt"
	"net/url"
)

type ApplicationsService struct {
	client *Client
}

type BuildPack string

const (
	BuildPackNixpacks      BuildPack = "nixpacks"
	BuildPackStatic        BuildPack = "static"
	BuildPackDockerfile    BuildPack = "dockerfile"
	BuildPackDockerCompose BuildPack = "dockercompose"
)

type RedirectType string

const (
	RedirectWWW    RedirectType = "www"
	RedirectNonWWW RedirectType = "non-www"
	RedirectBoth   RedirectType = "both"
)

type Application struct {
	ID                  int     `json:"id,omitempty"`
	UUID                string  `json:"uuid,omitempty"`
	Name                string  `json:"name,omitempty"`
	Description         *string `json:"description,omitempty"`
	FQDN                *string `json:"fqdn,omitempty"`
	ConfigHash          string  `json:"config_hash,omitempty"`
	Status              string  `json:"status,omitempty"`
	RepositoryProjectID *int    `json:"repository_project_id,omitempty"`
	SourceID            *int    `json:"source_id,omitempty"`
	PrivateKeyID        *int    `json:"private_key_id,omitempty"`
	DestinationType     string  `json:"destination_type,omitempty"`
	DestinationID       int     `json:"destination_id,omitempty"`
	EnvironmentID       int     `json:"environment_id,omitempty"`
	CreatedAt           string  `json:"created_at,omitempty"`
	UpdatedAt           string  `json:"updated_at,omitempty"`
	DeletedAt           *string `json:"deleted_at,omitempty"`

	GitRepository                string  `json:"git_repository,omitempty"`
	GitBranch                    string  `json:"git_branch,omitempty"`
	GitCommitSHA                 string  `json:"git_commit_sha,omitempty"`
	GitFullURL                   *string `json:"git_full_url,omitempty"`
	ManualWebhookSecretGitHub    *string `json:"manual_webhook_secret_github,omitempty"`
	ManualWebhookSecretGitLab    *string `json:"manual_webhook_secret_gitlab,omitempty"`
	ManualWebhookSecretBitbucket *string `json:"manual_webhook_secret_bitbucket,omitempty"`
	ManualWebhookSecretGitea     *string `json:"manual_webhook_secret_gitea,omitempty"`

	BuildPack                       string  `json:"build_pack,omitempty"`
	StaticImage                     string  `json:"static_image,omitempty"`
	InstallCommand                  string  `json:"install_command,omitempty"`
	BuildCommand                    string  `json:"build_command,omitempty"`
	StartCommand                    string  `json:"start_command,omitempty"`
	BaseDirectory                   string  `json:"base_directory,omitempty"`
	PublishDirectory                string  `json:"publish_directory,omitempty"`
	Dockerfile                      *string `json:"dockerfile,omitempty"`
	DockerfileLocation              string  `json:"dockerfile_location,omitempty"`
	DockerfileTargetBuild           *string `json:"dockerfile_target_build,omitempty"`
	DockerComposeLocation           string  `json:"docker_compose_location,omitempty"`
	DockerCompose                   *string `json:"docker_compose,omitempty"`
	DockerComposeRaw                *string `json:"docker_compose_raw,omitempty"`
	DockerComposeDomains            *string `json:"docker_compose_domains,omitempty"`
	DockerComposeCustomStartCommand *string `json:"docker_compose_custom_start_command,omitempty"`
	DockerComposeCustomBuildCommand *string `json:"docker_compose_custom_build_command,omitempty"`

	DockerRegistryImageName *string `json:"docker_registry_image_name,omitempty"`
	DockerRegistryImageTag  *string `json:"docker_registry_image_tag,omitempty"`

	PortsExposes  string  `json:"ports_exposes,omitempty"`
	PortsMappings *string `json:"ports_mappings,omitempty"`

	HealthCheckEnabled      bool    `json:"health_check_enabled,omitempty"`
	HealthCheckPath         string  `json:"health_check_path,omitempty"`
	HealthCheckPort         *string `json:"health_check_port,omitempty"`
	HealthCheckHost         *string `json:"health_check_host,omitempty"`
	HealthCheckMethod       string  `json:"health_check_method,omitempty"`
	HealthCheckReturnCode   int     `json:"health_check_return_code,omitempty"`
	HealthCheckScheme       string  `json:"health_check_scheme,omitempty"`
	HealthCheckResponseText *string `json:"health_check_response_text,omitempty"`
	HealthCheckInterval     int     `json:"health_check_interval,omitempty"`
	HealthCheckTimeout      int     `json:"health_check_timeout,omitempty"`
	HealthCheckRetries      int     `json:"health_check_retries,omitempty"`
	HealthCheckStartPeriod  int     `json:"health_check_start_period,omitempty"`
	CustomHealthcheckFound  bool    `json:"custom_healthcheck_found,omitempty"`

	LimitsMemory            string  `json:"limits_memory,omitempty"`
	LimitsMemorySwap        string  `json:"limits_memory_swap,omitempty"`
	LimitsMemorySwappiness  int     `json:"limits_memory_swappiness,omitempty"`
	LimitsMemoryReservation string  `json:"limits_memory_reservation,omitempty"`
	LimitsCPUs              string  `json:"limits_cpus,omitempty"`
	LimitsCPUSet            *string `json:"limits_cpuset,omitempty"`
	LimitsCPUShares         int     `json:"limits_cpu_shares,omitempty"`

	CustomLabels             *string `json:"custom_labels,omitempty"`
	CustomDockerRunOptions   string  `json:"custom_docker_run_options,omitempty"`
	CustomNetworkAliases     *string `json:"custom_network_aliases,omitempty"`
	CustomNginxConfiguration *string `json:"custom_nginx_configuration,omitempty"`

	PostDeploymentCommand          string `json:"post_deployment_command,omitempty"`
	PostDeploymentCommandContainer string `json:"post_deployment_command_container,omitempty"`
	PreDeploymentCommand           string `json:"pre_deployment_command,omitempty"`
	PreDeploymentCommandContainer  string `json:"pre_deployment_command_container,omitempty"`

	PreviewURLTemplate string  `json:"preview_url_template,omitempty"`
	Redirect           *string `json:"redirect,omitempty"`
	WatchPaths         *string `json:"watch_paths,omitempty"`

	SwarmReplicas             int     `json:"swarm_replicas,omitempty"`
	SwarmPlacementConstraints *string `json:"swarm_placement_constraints,omitempty"`

	ComposeParsingVersion *string `json:"compose_parsing_version,omitempty"`

	IsHTTPBasicAuthEnabled bool    `json:"is_http_basic_auth_enabled,omitempty"`
	HTTPBasicAuthUsername  *string `json:"http_basic_auth_username,omitempty"`
	HTTPBasicAuthPassword  *string `json:"http_basic_auth_password,omitempty"`
}

type CreatePrivateGitHubAppRequest struct {
	ProjectUUID     string    `json:"project_uuid"`
	ServerUUID      string    `json:"server_uuid"`
	EnvironmentName string    `json:"environment_name,omitempty"`
	EnvironmentUUID string    `json:"environment_uuid,omitempty"`
	GitHubAppUUID   string    `json:"github_app_uuid"`
	GitRepository   string    `json:"git_repository"`
	GitBranch       string    `json:"git_branch"`
	PortsExposes    string    `json:"ports_exposes"`
	BuildPack       BuildPack `json:"build_pack"`

	DestinationUUID         string `json:"destination_uuid,omitempty"`
	Name                    string `json:"name,omitempty"`
	Description             string `json:"description,omitempty"`
	Domains                 string `json:"domains,omitempty"`
	GitCommitSHA            string `json:"git_commit_sha,omitempty"`
	DockerRegistryImageName string `json:"docker_registry_image_name,omitempty"`
	DockerRegistryImageTag  string `json:"docker_registry_image_tag,omitempty"`

	IsStatic    *bool  `json:"is_static,omitempty"`
	IsSPA       *bool  `json:"is_spa,omitempty"`
	StaticImage string `json:"static_image,omitempty"`

	IsAutoDeployEnabled *bool `json:"is_auto_deploy_enabled,omitempty"`
	IsForceHTTPSEnabled *bool `json:"is_force_https_enabled,omitempty"`
	InstantDeploy       *bool `json:"instant_deploy,omitempty"`

	InstallCommand string `json:"install_command,omitempty"`
	BuildCommand   string `json:"build_command,omitempty"`
	StartCommand   string `json:"start_command,omitempty"`

	BaseDirectory    string `json:"base_directory,omitempty"`
	PublishDirectory string `json:"publish_directory,omitempty"`

	PortsMappings string `json:"ports_mappings,omitempty"`

	HealthCheckEnabled      *bool  `json:"health_check_enabled,omitempty"`
	HealthCheckPath         string `json:"health_check_path,omitempty"`
	HealthCheckPort         string `json:"health_check_port,omitempty"`
	HealthCheckHost         string `json:"health_check_host,omitempty"`
	HealthCheckMethod       string `json:"health_check_method,omitempty"`
	HealthCheckReturnCode   *int   `json:"health_check_return_code,omitempty"`
	HealthCheckScheme       string `json:"health_check_scheme,omitempty"`
	HealthCheckResponseText string `json:"health_check_response_text,omitempty"`
	HealthCheckInterval     *int   `json:"health_check_interval,omitempty"`
	HealthCheckTimeout      *int   `json:"health_check_timeout,omitempty"`
	HealthCheckRetries      *int   `json:"health_check_retries,omitempty"`
	HealthCheckStartPeriod  *int   `json:"health_check_start_period,omitempty"`

	LimitsMemory            string `json:"limits_memory,omitempty"`
	LimitsMemorySwap        string `json:"limits_memory_swap,omitempty"`
	LimitsMemorySwappiness  *int   `json:"limits_memory_swappiness,omitempty"`
	LimitsMemoryReservation string `json:"limits_memory_reservation,omitempty"`
	LimitsCPUs              string `json:"limits_cpus,omitempty"`
	LimitsCPUSet            string `json:"limits_cpuset,omitempty"`
	LimitsCPUShares         *int   `json:"limits_cpu_shares,omitempty"`

	CustomLabels           string `json:"custom_labels,omitempty"`
	CustomDockerRunOptions string `json:"custom_docker_run_options,omitempty"`

	PostDeploymentCommand          string `json:"post_deployment_command,omitempty"`
	PostDeploymentCommandContainer string `json:"post_deployment_command_container,omitempty"`
	PreDeploymentCommand           string `json:"pre_deployment_command,omitempty"`
	PreDeploymentCommandContainer  string `json:"pre_deployment_command_container,omitempty"`

	ManualWebhookSecretGitHub    string `json:"manual_webhook_secret_github,omitempty"`
	ManualWebhookSecretGitLab    string `json:"manual_webhook_secret_gitlab,omitempty"`
	ManualWebhookSecretBitbucket string `json:"manual_webhook_secret_bitbucket,omitempty"`
	ManualWebhookSecretGitea     string `json:"manual_webhook_secret_gitea,omitempty"`

	Redirect RedirectType `json:"redirect,omitempty"`

	Dockerfile         string `json:"dockerfile,omitempty"`
	DockerfileLocation string `json:"dockerfile_location,omitempty"`

	DockerComposeLocation           string                `json:"docker_compose_location,omitempty"`
	DockerComposeCustomStartCommand string                `json:"docker_compose_custom_start_command,omitempty"`
	DockerComposeCustomBuildCommand string                `json:"docker_compose_custom_build_command,omitempty"`
	DockerComposeDomains            []DockerComposeDomain `json:"docker_compose_domains,omitempty"`

	WatchPaths string `json:"watch_paths,omitempty"`

	UseBuildServer *bool `json:"use_build_server,omitempty"`

	IsHTTPBasicAuthEnabled *bool  `json:"is_http_basic_auth_enabled,omitempty"`
	HTTPBasicAuthUsername  string `json:"http_basic_auth_username,omitempty"`
	HTTPBasicAuthPassword  string `json:"http_basic_auth_password,omitempty"`

	ConnectToDockerNetwork *bool `json:"connect_to_docker_network,omitempty"`

	ForceDomainOverride *bool `json:"force_domain_override,omitempty"`
	AutogenerateDomain  *bool `json:"autogenerate_domain,omitempty"`

	IsContainerLabelEscapeEnabled *bool `json:"is_container_label_escape_enabled,omitempty"`
}

type DockerComposeDomain struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

type CreateApplicationResponse struct {
	UUID string `json:"uuid"`
}

type StartApplicationResponse struct {
	Message        string `json:"message"`
	DeploymentUUID string `json:"deployment_uuid"`
}

type ListApplicationsOptions struct {
	Tag string
}

func (s *ApplicationsService) List(ctx context.Context, opts *ListApplicationsOptions) ([]Application, error) {
	var query url.Values
	if opts != nil && opts.Tag != "" {
		query = url.Values{}
		query.Set("tag", opts.Tag)
	}

	var apps []Application
	if err := s.client.do(ctx, "GET", "/api/v1/applications", query, nil, &apps); err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}
	return apps, nil
}

func (s *ApplicationsService) Get(ctx context.Context, uuid string) (*Application, error) {
	if uuid == "" {
		return nil, fmt.Errorf("coolify: uuid is required")
	}

	var app Application
	if err := s.client.do(ctx, "GET", "/api/v1/applications/"+uuid, nil, nil, &app); err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	return &app, nil
}

func (s *ApplicationsService) CreatePrivateGitHubApp(ctx context.Context, req *CreatePrivateGitHubAppRequest) (*CreateApplicationResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("coolify: request is required")
	}
	if req.ProjectUUID == "" {
		return nil, fmt.Errorf("coolify: project_uuid is required")
	}
	if req.ServerUUID == "" {
		return nil, fmt.Errorf("coolify: server_uuid is required")
	}
	if req.EnvironmentName == "" && req.EnvironmentUUID == "" {
		return nil, fmt.Errorf("coolify: environment_name or environment_uuid is required")
	}
	if req.GitHubAppUUID == "" {
		return nil, fmt.Errorf("coolify: github_app_uuid is required")
	}
	if req.GitRepository == "" {
		return nil, fmt.Errorf("coolify: git_repository is required")
	}
	if req.GitBranch == "" {
		return nil, fmt.Errorf("coolify: git_branch is required")
	}
	if req.PortsExposes == "" {
		return nil, fmt.Errorf("coolify: ports_exposes is required")
	}
	if req.BuildPack == "" {
		return nil, fmt.Errorf("coolify: build_pack is required")
	}

	var resp CreateApplicationResponse
	if err := s.client.do(ctx, "POST", "/api/v1/applications/private-github-app", nil, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}
	return &resp, nil
}

type StartOptions struct {
	Force         bool
	InstantDeploy bool
}

func (s *ApplicationsService) Start(ctx context.Context, uuid string, opts *StartOptions) (*StartApplicationResponse, error) {
	if uuid == "" {
		return nil, fmt.Errorf("coolify: uuid is required")
	}

	var query url.Values
	if opts != nil {
		query = url.Values{}
		if opts.Force {
			query.Set("force", "true")
		}
		if opts.InstantDeploy {
			query.Set("instant_deploy", "true")
		}
	}

	var resp StartApplicationResponse
	if err := s.client.do(ctx, "POST", "/api/v1/applications/"+uuid+"/start", query, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to start application: %w", err)
	}
	return &resp, nil
}

func (s *ApplicationsService) Stop(ctx context.Context, uuid string) error {
	if uuid == "" {
		return fmt.Errorf("coolify: uuid is required")
	}

	if err := s.client.do(ctx, "POST", "/api/v1/applications/"+uuid+"/stop", nil, nil, nil); err != nil {
		return fmt.Errorf("failed to stop application: %w", err)
	}
	return nil
}

func (s *ApplicationsService) Restart(ctx context.Context, uuid string) (*StartApplicationResponse, error) {
	if uuid == "" {
		return nil, fmt.Errorf("coolify: uuid is required")
	}

	var resp StartApplicationResponse
	if err := s.client.do(ctx, "POST", "/api/v1/applications/"+uuid+"/restart", nil, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to restart application: %w", err)
	}
	return &resp, nil
}

func (s *ApplicationsService) Delete(ctx context.Context, uuid string) error {
	if uuid == "" {
		return fmt.Errorf("coolify: uuid is required")
	}

	if err := s.client.do(ctx, "DELETE", "/api/v1/applications/"+uuid, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}
	return nil
}

type DeployOptions struct {
	Force bool
}

type DeployResponse struct {
	Deployments []DeploymentInfo `json:"deployments"`
}

type DeploymentInfo struct {
	Message        string `json:"message"`
	ResourceUUID   string `json:"resource_uuid"`
	DeploymentUUID string `json:"deployment_uuid"`
}

func (s *ApplicationsService) Deploy(ctx context.Context, uuid string, opts *DeployOptions) (*DeployResponse, error) {
	if uuid == "" {
		return nil, fmt.Errorf("coolify: uuid is required")
	}

	query := url.Values{}
	query.Set("uuid", uuid)
	if opts != nil && opts.Force {
		query.Set("force", "true")
	}

	var resp DeployResponse
	if err := s.client.do(ctx, "GET", "/api/v1/deploy", query, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to deploy application: %w", err)
	}
	return &resp, nil
}
