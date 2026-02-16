package gitserver

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/go-chi/chi/v5"
)

// handleInfoRefs handles GET /{owner}/{repo}.git/info/refs?service=git-{upload,receive}-pack
// This is the "discovery" step of the smart HTTP protocol.
func (s *Server) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	repoFullName := owner + "/" + repo
	service := r.URL.Query().Get("service")

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "invalid service", http.StatusBadRequest)
		return
	}

	scope := "pull"
	if service == "git-receive-pack" {
		scope = "push"
	}

	auth := s.requireRepoAuth(w, r, repoFullName, scope)
	if auth == nil {
		return
	}

	repoPath := barePath(s.config.ReposRoot, owner, repo)

	// For receive-pack, ensure the bare repo exists
	if service == "git-receive-pack" {
		var err error
		repoPath, err = ensureBareRepo(s.config.ReposRoot, owner, repo)
		if err != nil {
			s.logger.Error("failed to ensure bare repo", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// service is "git-receive-pack" or "git-upload-pack"; git expects "receive-pack"/"upload-pack"
	gitCmd := strings.TrimPrefix(service, "git-")
	cmd := exec.CommandContext(r.Context(), "git", gitCmd, "--stateless-rpc", "--advertise-refs", repoPath)
	out, err := cmd.Output()
	if err != nil {
		s.logger.Error("git advertise-refs failed", "service", service, "error", err)
		http.Error(w, "git command failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	// Write the pkt-line service advertisement header, then flush, then the actual refs
	if err := writeServiceAdvertisement(w, service); err != nil {
		return
	}
	if err := writePktFlush(w); err != nil {
		return
	}
	w.Write(out)
}

// handleUploadPack handles POST /{owner}/{repo}.git/git-upload-pack (clone/fetch)
func (s *Server) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	repoFullName := owner + "/" + repo

	auth := s.requireRepoAuth(w, r, repoFullName, "pull")
	if auth == nil {
		return
	}

	repoPath := barePath(s.config.ReposRoot, owner, repo)

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.CommandContext(r.Context(), "git", "upload-pack", "--stateless-rpc", repoPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		s.logger.Error("git upload-pack failed", "repo", repoFullName, "error", err)
		// Can't send HTTP error if we already started writing
		return
	}
}

// handleReceivePack handles POST /{owner}/{repo}.git/git-receive-pack (push)
func (s *Server) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	repoFullName := owner + "/" + repo

	auth := s.requireRepoAuth(w, r, repoFullName, "push")
	if auth == nil {
		return
	}

	repoPath, err := ensureBareRepo(s.config.ReposRoot, owner, repo)
	if err != nil {
		s.logger.Error("failed to ensure bare repo", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Snapshot refs before the push
	before, err := snapshotRefs(repoPath)
	if err != nil {
		s.logger.Error("failed to snapshot refs before push", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.CommandContext(r.Context(), "git", "receive-pack", "--stateless-rpc", repoPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		s.logger.Error("git receive-pack failed", "repo", repoFullName, "error", err)
		return
	}

	// Snapshot refs after the push and trigger deploys for changed branches
	after, err := snapshotRefs(repoPath)
	if err != nil {
		s.logger.Error("failed to snapshot refs after push", "repo", repoFullName, "error", err)
		return
	}

	s.triggerDeploysForPush(r.Context(), repoFullName, before, after)
}
