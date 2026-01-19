package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/augustdev/autoclip/internal/github"
)

type Handlers struct {
	service     *Service
	githubOAuth *github.OAuthService
	config      Config
	logger      *slog.Logger
}

func NewHandlers(
	service *Service,
	githubOAuth *github.OAuthService,
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

	authURL := h.githubOAuth.GetAuthURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *Handlers) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		h.logger.Error("invalid state", "error", err)
		http.Redirect(w, r, h.config.FrontendURL+"?error=invalid_state", http.StatusTemporaryRedirect)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	if code == "" {
		h.logger.Error("no code in callback")
		http.Redirect(w, r, h.config.FrontendURL+"?error=no_code", http.StatusTemporaryRedirect)
		return
	}

	session, err := h.service.HandleOAuthCallback(r.Context(), code)
	if err != nil {
		h.logger.Error("oauth callback failed", "error", err)
		http.Redirect(w, r, h.config.FrontendURL+"?error=auth_failed", http.StatusTemporaryRedirect)
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

	http.Redirect(w, r, h.config.FrontendURL, http.StatusTemporaryRedirect)
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
	if user.AvatarUrl.Valid {
		avatarURL = user.AvatarUrl.String
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id": %d, "githubUsername": "%s", "avatarUrl": "%s"}`,
		user.ID, user.GithubUsername, avatarURL)
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
