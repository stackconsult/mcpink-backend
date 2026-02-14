package k8sdeployments

type Config struct {
	BuildkitHost    string
	RegistryHost    string
	RegistryAddress string
	LokiPushURL     string
	LokiQueryURL    string
	TaskQueue       string
}
