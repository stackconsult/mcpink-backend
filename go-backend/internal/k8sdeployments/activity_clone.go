package k8sdeployments

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (a *Activities) CloneRepo(ctx context.Context, input CloneRepoInput) (*CloneRepoResult, error) {
	a.logger.Info("CloneRepo activity started",
		"serviceID", input.ServiceID,
		"repo", input.Repo,
		"branch", input.Branch,
		"gitProvider", input.GitProvider)

	dir, err := os.MkdirTemp("/tmp", fmt.Sprintf("build-%s-", input.ServiceID))
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	cloneURL, err := a.resolveCloneURL(ctx, input)
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("resolve clone URL: %w", err)
	}

	args := []string{"clone", "--depth", "1"}
	if input.Branch != "" {
		args = append(args, "--branch", input.Branch)
	}
	args = append(args, cloneURL, dir)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("git clone failed: %w\noutput: %s", err, redactToken(string(output)))
	}

	commitSHA := input.CommitSHA
	if commitSHA == "" {
		revCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
		out, err := revCmd.Output()
		if err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
		}
		commitSHA = strings.TrimSpace(string(out))
	}

	a.logger.Info("CloneRepo completed", "serviceID", input.ServiceID, "commitSHA", commitSHA, "dir", dir)
	return &CloneRepoResult{
		SourcePath: dir,
		CommitSHA:  commitSHA,
	}, nil
}

func (a *Activities) resolveCloneURL(ctx context.Context, input CloneRepoInput) (string, error) {
	switch input.GitProvider {
	case "github":
		token, err := a.githubApp.CreateInstallationToken(ctx, input.InstallationID, nil)
		if err != nil {
			return "", fmt.Errorf("create github installation token: %w", err)
		}
		return fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token.Token, input.Repo), nil

	case "gitea":
		if a.internalGitSvc == nil {
			return "", fmt.Errorf("internal git service not configured")
		}
		parts := strings.SplitN(input.Repo, "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid gitea repo format: %s", input.Repo)
		}
		cfg := a.internalGitSvc.Client().Config()
		return a.internalGitSvc.Client().GetHTTPSCloneURL(parts[0], parts[1], cfg.AdminToken), nil

	default:
		return "", fmt.Errorf("unsupported git provider: %s", input.GitProvider)
	}
}

func redactToken(s string) string {
	// Redact tokens from git output (e.g., x-access-token:ghs_xxx@)
	if idx := strings.Index(s, "x-access-token:"); idx >= 0 {
		end := strings.Index(s[idx:], "@")
		if end > 0 {
			s = s[:idx] + "x-access-token:***@" + s[idx+end+1:]
		}
	}
	return s
}
