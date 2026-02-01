package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/augustdev/autoclip/internal/github_oauth"
)

type Handlers struct {
	service     *Service
	githubOAuth *github_oauth.OAuthService
	config      Config
	logger      *slog.Logger
}

func NewHandlers(
	service *Service,
	githubOAuth *github_oauth.OAuthService,
	config Config,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		service:     service,
		githubOAuth: githubOAuth,
		config:      config,
		logger:      logger,
	}
}

func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Get("/auth/github", h.HandleGitHubLogin)
	r.Get("/auth/github/callback", h.HandleGitHubCallback)
	r.Get("/auth/githubapp/callback", h.HandleGitHubAppCallback)
	r.Post("/auth/logout", h.HandleLogout)
	r.Get("/auth/me", h.HandleMe)
}

func (h *Handlers) HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		h.logger.Error("failed to generate state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   h.config.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Check for additional scopes (e.g., ?scope=repo)
	additionalScope := r.URL.Query().Get("scope")
	var additionalScopes []string
	if additionalScope != "" {
		additionalScopes = []string{additionalScope}
		// Store redirect URL so we return to settings after OAuth
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_redirect",
			Value:    "/settings/access",
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			Secure:   h.config.SessionCookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}

	authURL := h.githubOAuth.GetAuthURLWithScopes(state, additionalScopes)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *Handlers) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	// Helper to get redirect URL and clear the cookie
	getRedirectURL := func() string {
		redirectURL := h.config.FrontendURL
		if redirectCookie, err := r.Cookie("oauth_redirect"); err == nil && redirectCookie.Value != "" {
			redirectURL = h.config.FrontendURL + redirectCookie.Value
			http.SetCookie(w, &http.Cookie{
				Name:     "oauth_redirect",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
		}
		return redirectURL
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Handle user denied access
	if errorParam == "access_denied" {
		h.logger.Info("user denied oauth access")
		http.Redirect(w, r, getRedirectURL(), http.StatusTemporaryRedirect)
		return
	}

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		h.logger.Error("invalid state", "error", err)
		http.Redirect(w, r, getRedirectURL()+"?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}

	if code == "" {
		h.logger.Error("no code in callback")
		http.Redirect(w, r, getRedirectURL()+"?error=no_code", http.StatusTemporaryRedirect)
		return
	}

	session, err := h.service.HandleOAuthCallback(r.Context(), code)
	if err != nil {
		h.logger.Error("oauth callback failed", "error", err)
		http.Redirect(w, r, getRedirectURL()+"?error=auth_failed", http.StatusTemporaryRedirect)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.config.SessionCookieName,
		Value:    session.Token,
		Path:     "/",
		MaxAge:   int(h.config.JWTExpiry.Seconds()),
		HttpOnly: true,
		Secure:   h.config.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Domain:   h.config.SessionCookieDomain,
	})

	http.Redirect(w, r, getRedirectURL(), http.StatusTemporaryRedirect)
}

func (h *Handlers) HandleGitHubAppCallback(w http.ResponseWriter, r *http.Request) {
	installationIDStr := r.URL.Query().Get("installation_id")
	setupAction := r.URL.Query().Get("setup_action")

	// Redirect URL for success/error
	successURL := h.config.FrontendURL + "/githubapp/success"
	errorURL := h.config.FrontendURL + "/settings/access?error=githubapp_failed"

	if installationIDStr == "" || setupAction != "install" {
		// Not an install action or missing installation_id
		http.Redirect(w, r, successURL, http.StatusTemporaryRedirect)
		return
	}

	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		h.logger.Error("invalid installation_id", "error", err, "installation_id", installationIDStr)
		http.Redirect(w, r, errorURL, http.StatusTemporaryRedirect)
		return
	}

	// Get user from session cookie
	cookie, err := r.Cookie(h.config.SessionCookieName)
	if err != nil {
		h.logger.Error("no session cookie for github app callback", "error", err)
		http.Redirect(w, r, errorURL, http.StatusTemporaryRedirect)
		return
	}

	userID, err := h.service.ValidateJWT(cookie.Value)
	if err != nil {
		h.logger.Error("invalid jwt in github app callback", "error", err)
		http.Redirect(w, r, errorURL, http.StatusTemporaryRedirect)
		return
	}

	// Save installation ID to database
	if err := h.service.SetGitHubAppInstallation(r.Context(), userID, installationID); err != nil {
		h.logger.Error("failed to save github app installation", "error", err, "user_id", userID, "installation_id", installationID)
		http.Redirect(w, r, errorURL, http.StatusTemporaryRedirect)
		return
	}

	// Get user to retrieve github username for the source name
	user, err := h.service.GetUserByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user for coolify source creation", "error", err, "user_id", userID)
		http.Redirect(w, r, errorURL, http.StatusTemporaryRedirect)
		return
	}

	// Create Coolify GitHub App source for this user
	coolifyUUID, err := h.service.CreateCoolifyGitHubAppSource(r.Context(), userID, installationID, user.GithubUsername)
	if err != nil {
		h.logger.Error("failed to create coolify github app source", "error", err, "user_id", userID, "installation_id", installationID)
		http.Redirect(w, r, errorURL, http.StatusTemporaryRedirect)
		return
	}

	h.logger.Info("github app installed", "user_id", userID, "installation_id", installationID, "coolify_uuid", coolifyUUID)
	http.Redirect(w, r, successURL, http.StatusTemporaryRedirect)
}

func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Domain:   h.config.SessionCookieDomain,
	})

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handlers) HandleMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.config.SessionCookieName)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := h.service.ValidateJWT(cookie.Value)
	if err != nil {
		h.logger.Error("invalid jwt", "error", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.service.GetUserByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	avatarURL := ""
	if user.AvatarUrl != nil {
		avatarURL = *user.AvatarUrl
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id": "%s", "githubUsername": "%s", "avatarUrl": "%s"}`,
		user.ID, user.GithubUsername, avatarURL)
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
