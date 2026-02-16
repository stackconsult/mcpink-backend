package k8sdeployments

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	recordHeartbeat(ctx, "cloning")
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

	// Keep only the working tree. Shipping git history to BuildKit is slow and
	// doesn't change build outputs.
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("cleanup .git dir: %w", err)
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

	case "internal":
		parts := strings.SplitN(input.Repo, "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid internal repo format: %s", input.Repo)
		}
		return fmt.Sprintf("http://x-admin-token:%s@%s/%s/%s.git",
			a.config.GitServerAdminToken, a.config.GitServerCloneHost, parts[0], parts[1]), nil

	default:
		return "", fmt.Errorf("unsupported git provider: %s", input.GitProvider)
	}
}

func redactToken(s string) string {
	// Redact tokens from git output (e.g., x-access-token:ghs_xxx@ or x-admin-token:xxx@)
	for _, prefix := range []string{"x-access-token:", "x-admin-token:", "x-git-token:"} {
		if idx := strings.Index(s, prefix); idx >= 0 {
			end := strings.Index(s[idx:], "@")
			if end > 0 {
				s = s[:idx] + prefix + "***@" + s[idx+end+1:]
			}
		}
	}
	return s
}
