package k8sdeployments

const TaskQueue = "k8s-native"

type DeployServiceInput struct {
	ServiceID      string
	Repo           string
	Branch         string
	GitProvider    string
	InstallationID int64
	CommitSHA      string
	AppsDomain     string
}

type DeployServiceResult struct {
	ServiceID    string
	Status       string
	URL          string
	CommitSHA    string
	ErrorMessage string
}

// Type aliases preserve backward compatibility for callers and Temporal registration.
type (
	CreateServiceWorkflowInput  = DeployServiceInput
	CreateServiceWorkflowResult = DeployServiceResult

	RedeployServiceWorkflowInput  = DeployServiceInput
	RedeployServiceWorkflowResult = DeployServiceResult
)

type DeleteServiceWorkflowInput struct {
	ServiceID string
	Namespace string
	Name      string
}

type DeleteServiceWorkflowResult struct {
	ServiceID    string
	Status       string
	ErrorMessage string
}

type BuildServiceWorkflowInput struct {
	ServiceID      string
	Repo           string
	Branch         string
	GitProvider    string
	InstallationID int64
	CommitSHA      string
}

type BuildServiceWorkflowResult struct {
	ImageRef  string
	CommitSHA string
	Port      string
}

type CloneRepoInput struct {
	ServiceID      string
	Repo           string
	Branch         string
	GitProvider    string
	InstallationID int64
	CommitSHA      string
}

type CloneRepoResult struct {
	SourcePath string
	CommitSHA  string
}

type ResolveBuildContextInput struct {
	ServiceID  string
	SourcePath string
	CommitSHA  string
}

type ResolveImageRefInput struct {
	ServiceID string
	CommitSHA string
}

type ResolveImageRefResult struct {
	ImageRef string
}

type ResolveBuildContextResult struct {
	BuildPack           string
	ImageRef            string
	Namespace           string
	Name                string
	Port                string
	EnvVars             map[string]string
	PublishDirectory    string
	EffectiveSourcePath string
	DockerfilePath      string
	BuildCommand        string
	StartCommand        string
}

type BuildImageInput struct {
	SourcePath       string
	ImageRef         string
	BuildPack        string
	Name             string
	Namespace        string
	EnvVars          map[string]string
	PublishDirectory string
	DockerfilePath   string
	BuildCommand     string
	StartCommand     string
}

type BuildImageResult struct {
	ImageRef string
}

type DeployInput struct {
	ServiceID  string
	ImageRef   string
	CommitSHA  string
	AppsDomain string
	Port       string // resolved port from build phase; empty = re-read from DB
}

type DeployResult struct {
	Namespace      string
	DeploymentName string
	URL            string
}

type WaitForRolloutInput struct {
	Namespace      string
	DeploymentName string
}

type WaitForRolloutResult struct {
	Status string
}

type DeleteServiceInput struct {
	ServiceID string
	Namespace string
	Name      string
}

type DeleteServiceResult struct {
	Status string
}

type UpdateBuildStatusInput struct {
	ServiceID   string
	BuildStatus string
}

type MarkServiceRunningInput struct {
	ServiceID string
	URL       string
	CommitSHA string
}

type MarkServiceFailedInput struct {
	ServiceID    string
	ErrorMessage string
}
