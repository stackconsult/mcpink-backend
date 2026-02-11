package k8sdeployments

import "strings"

func effectiveAppPort(buildPack, appPort string, publishDir *string) string {
	port := strings.TrimSpace(appPort)
	if port == "" {
		port = "3000"
	}

	switch buildPack {
	case "static":
		port = "8080"
	case "railpack", "nixpacks":
		if publishDir != nil && strings.TrimSpace(*publishDir) != "" {
			port = "8080"
		}
	}

	return port
}
