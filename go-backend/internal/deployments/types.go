package deployments

type DeployWorkflowInput struct {
	AppID         string
	UserID        string
	ProjectID     string
	GitHubAppUUID string
	Repo          string
	Branch        string
	Name          string
	BuildPack     string
	Port          string
	EnvVars       []EnvVar
}

type DeployWorkflowResult struct {
	AppID    string
	AppUUID      string
	FQDN         string
	Status       string
	ErrorMessage string
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type BuildStatus string

const (
	BuildStatusQueued   BuildStatus = "queued"
	BuildStatusBuilding BuildStatus = "building"
	BuildStatusSuccess  BuildStatus = "success"
	BuildStatusFailed   BuildStatus = "failed"
)

type RuntimeStatus string

const (
	RuntimeStatusRunning RuntimeStatus = "running"
	RuntimeStatusStopped RuntimeStatus = "stopped"
	RuntimeStatusExited  RuntimeStatus = "exited"
)

type RedeployWorkflowInput struct {
	AppID          string
	CoolifyAppUUID string
}

type RedeployWorkflowResult struct {
	AppID        string
	FQDN         string
	Status       string
	ErrorMessage string
}
