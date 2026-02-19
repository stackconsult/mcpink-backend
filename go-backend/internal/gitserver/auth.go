package gitserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

type AuthResult struct {
	TokenID  string
	UserID   string
	RepoID   *string
	Scopes   []string
	IsAdmin  bool
	RepoFull *string // full_name from joined internal_repos
}

func (s *Server) authenticateRequest(r *http.Request) *AuthResult {
	_, password, ok := r.BasicAuth()
	if !ok || password == "" {
		return nil
	}

	// Check admin token first (constant-time comparison).
	// Admin token is only accepted on internal (non-external) requests.
	if s.config.AdminToken != "" && subtle.ConstantTimeCompare([]byte(password), []byte(s.config.AdminToken)) == 1 {
		if s.isExternalRequest(r) {
			s.logger.Warn("admin token rejected on external request")
			return nil
		}
		return &AuthResult{
			IsAdmin: true,
			Scopes:  []string{"push", "pull"},
		}
	}

	// Hash the token and look up in DB
	hash := hashToken(password)
	token, err := s.gitTokensQ.GetTokenByHash(r.Context(), hash)
	if err != nil {
		s.logger.Debug("token lookup failed", "error", err)
		return nil
	}

	go func() {
		if err := s.gitTokensQ.UpdateLastUsed(context.Background(), token.ID); err != nil {
			s.logger.Warn("failed to update token last_used_at", "error", err)
		}
	}()

	return &AuthResult{
		TokenID:  token.ID,
		UserID:   token.UserID,
		RepoID:   token.RepoID,
		Scopes:   token.Scopes,
		IsAdmin:  false,
		RepoFull: token.RepoFullName,
	}
}

// isExternalRequest returns true if the request came through the external
// IngressRoute (i.e., from the internet). Internal K8s Service traffic
// doesn't have X-Forwarded-For headers.
func (s *Server) isExternalRequest(r *http.Request) bool {
	return r.Header.Get("X-Forwarded-For") != ""
}

// hasScope checks if the auth result includes a specific scope.
func (auth *AuthResult) hasScope(scope string) bool {
	if auth.IsAdmin {
		return true
	}
	for _, s := range auth.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// matchesRepo checks if the token is authorized for the given repo.
// Admin tokens and user-wide tokens (nil RepoID) match any repo.
// Repo-scoped tokens must match the specific repo full name.
func (auth *AuthResult) matchesRepo(repoFullName string) bool {
	if auth.IsAdmin {
		return true
	}
	// Nil repo_id = user-wide token, matches any repo
	if auth.RepoID == nil {
		return true
	}
	// Repo-scoped: check full name matches
	if auth.RepoFull != nil && *auth.RepoFull == repoFullName {
		return true
	}
	return false
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) *AuthResult {
	auth := s.authenticateRequest(r)
	if auth == nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil
	}
	return auth
}

// hashToken computes SHA-256 hash of a raw token string.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *Server) requireRepoAuth(w http.ResponseWriter, r *http.Request, repoFullName, scope string) *AuthResult {
	auth := s.requireAuth(w, r)
	if auth == nil {
		return nil
	}
	if !auth.hasScope(scope) {
		http.Error(w, "insufficient scope", http.StatusForbidden)
		return nil
	}
	if !auth.matchesRepo(repoFullName) {
		http.Error(w, "token not authorized for this repository", http.StatusForbidden)
		return nil
	}
	return auth
}
