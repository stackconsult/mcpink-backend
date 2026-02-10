package k8sdeployments

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
)

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]`)

func sanitizeDNS(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = nonAlphanumDash.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func NamespaceName(githubUsername, projectRef string) string {
	return fmt.Sprintf("dp-%s-%s", sanitizeDNS(githubUsername), sanitizeDNS(projectRef))
}

func ServiceName(appName string) string {
	return sanitizeDNS(appName)
}

// resolveUsername returns the user's GiteaUsername, falling back to GithubUsername.
func resolveUsername(user users.User) string {
	if user.GiteaUsername != nil && *user.GiteaUsername != "" {
		return *user.GiteaUsername
	}
	if user.GithubUsername != nil && *user.GithubUsername != "" {
		return *user.GithubUsername
	}
	return ""
}
