package k8sdeployments

type DeployServiceInput struct {
	ServiceID      string
	DeploymentID   string
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
	DeploymentID   string
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

// Deployment-aware status activity inputs

type UpdateDeploymentBuildingInput struct {
	DeploymentID string
}

type UpdateDeploymentDeployingInput struct {
	DeploymentID string
}

type MarkDeploymentActiveInput struct {
	ServiceID    string
	DeploymentID string
	URL          string
	CommitSHA    string
	ImageRef     string
}

type MarkDeploymentFailedInput struct {
	DeploymentID string
	ErrorMessage string
}

type UpdateDeploymentBuildProgressInput struct {
	DeploymentID  string
	BuildProgress []byte
}

type AttachCustomDomainWorkflowInput struct {
	CustomDomainID string
	ServiceID      string
	Namespace      string
	ServiceName    string
	CustomDomain   string
}

type AttachCustomDomainWorkflowResult struct {
	Status       string
	ErrorMessage string
}

type DetachCustomDomainWorkflowInput struct {
	CustomDomainID string
	ServiceID      string
	Namespace      string
	ServiceName    string
}

type DetachCustomDomainWorkflowResult struct {
	Status       string
	ErrorMessage string
}

type ApplyCustomDomainIngressInput struct {
	Namespace   string
	ServiceName string
	Domain      string
}

type DeleteCustomDomainIngressInput struct {
	Namespace   string
	ServiceName string
}

type ApplyCustomDomainCertificateInput struct {
	Namespace   string
	ServiceName string
	Domain      string
}

type WaitForCertificateReadyInput struct {
	Namespace       string
	CertificateName string
}

type UpdateCustomDomainStatusInput struct {
	CustomDomainID string
	Status         string
}
