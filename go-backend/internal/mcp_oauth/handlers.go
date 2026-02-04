package mcp_oauth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	service     *Service
	authService *auth.Service
	authConfig  auth.Config
	config      Config
	logger      *slog.Logger
}

func NewHandlers(
	service *Service,
	authService *auth.Service,
	authConfig auth.Config,
	config Config,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		service:     service,
		authService: authService,
		authConfig:  authConfig,
		config:      config,
		logger:      logger,
	}
}

func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Get("/.well-known/oauth-protected-resource", h.HandleProtectedResourceMetadata)
	r.Get("/.well-known/oauth-authorization-server", h.HandleAuthServerMetadata)
	r.Post("/oauth/register", h.HandleRegister)
	r.Get("/oauth/authorize", h.HandleAuthorize)
	r.Get("/oauth/context", h.HandleContext)
	r.Post("/oauth/complete", h.HandleComplete)
	r.Post("/oauth/token", h.HandleToken)
}

func (h *Handlers) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]any{
		"resource":              h.config.Issuer,
		"authorization_servers": []string{h.config.Issuer},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

func (h *Handlers) HandleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]any{
		"issuer":                                h.config.Issuer,
		"authorization_endpoint":               h.config.Issuer + "/oauth/authorize",
		"token_endpoint":                       h.config.Issuer + "/oauth/token",
		"registration_endpoint":                h.config.Issuer + "/oauth/register",
		"response_types_supported":             []string{"code"},
		"grant_types_supported":                []string{"authorization_code"},
		"code_challenge_methods_supported":     []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// HandleRegister implements RFC 7591 Dynamic Client Registration.
// MCP clients register themselves before starting the OAuth flow.
func (h *Handlers) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
		ClientURI    string   `json:"client_uri"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_client_metadata",
			"error_description": "invalid request body",
		})
		return
	}

	// Validate redirect URIs - ensure valid URL format
	for _, uri := range req.RedirectURIs {
		if _, err := url.Parse(uri); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_redirect_uri",
				"error_description": "invalid URL format",
			})
			return
		}
	}

	// Generate a client_id - we don't store this since we use PKCE
	// The client_id is just used for display purposes in the consent screen
	clientID := req.ClientName
	if clientID == "" {
		clientID = "mcp-client"
	}

	response := map[string]any{
		"client_id":                clientID,
		"client_name":              req.ClientName,
		"redirect_uris":            req.RedirectURIs,
		"grant_types":              []string{"authorization_code"},
		"response_types":           []string{"code"},
		"token_endpoint_auth_method": "none",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (h *Handlers) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")
	state := r.URL.Query().Get("state")

	if clientID == "" || redirectURI == "" || codeChallenge == "" {
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		http.Error(w, "only S256 code_challenge_method is supported", http.StatusBadRequest)
		return
	}

	if _, err := url.Parse(redirectURI); err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// Store OAuth params in cookie for retrieval after GitHub OAuth
	oauthContext := url.Values{
		"client_id":      {clientID},
		"redirect_uri":   {redirectURI},
		"code_challenge": {codeChallenge},
		"state":          {state},
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "mcp_oauth_context",
		Value:    oauthContext.Encode(),
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.authConfig.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Set redirect cookie so GitHub callback goes to /oauth/consent
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_redirect",
		Value:    "/oauth/consent",
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.authConfig.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.config.Issuer+"/auth/github", http.StatusTemporaryRedirect)
}

func (h *Handlers) HandleContext(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.authConfig.SessionCookieName)
	if err != nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	userID, err := h.authService.ValidateJWT(cookie.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	oauthCookie, err := r.Cookie("mcp_oauth_context")
	if err != nil {
		http.Error(w, "no oauth context", http.StatusBadRequest)
		return
	}

	values, err := url.ParseQuery(oauthCookie.Value)
	if err != nil {
		http.Error(w, "invalid oauth context", http.StatusBadRequest)
		return
	}

	// Check if user needs onboarding (no API keys = new user)
	needsOnboarding := false
	apiKeys, err := h.authService.ListAPIKeys(r.Context(), userID)
	if err == nil && len(apiKeys) == 0 {
		needsOnboarding = true
	}

	response := map[string]any{
		"client_id":        values.Get("client_id"),
		"redirect_uri":     values.Get("redirect_uri"),
		"state":            values.Get("state"),
		"user_id":          userID,
		"needs_onboarding": needsOnboarding,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type CompleteRequest struct {
	APIKeyName string `json:"api_key_name"`
}

func (h *Handlers) HandleComplete(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.authConfig.SessionCookieName)
	if err != nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	userID, err := h.authService.ValidateJWT(cookie.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	oauthCookie, err := r.Cookie("mcp_oauth_context")
	if err != nil {
		http.Error(w, "no oauth context", http.StatusBadRequest)
		return
	}

	values, err := url.ParseQuery(oauthCookie.Value)
	if err != nil {
		http.Error(w, "invalid oauth context", http.StatusBadRequest)
		return
	}

	clientID := values.Get("client_id")
	redirectURI := values.Get("redirect_uri")
	codeChallenge := values.Get("code_challenge")
	state := values.Get("state")

	var req CompleteRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	apiKeyName := req.APIKeyName
	if apiKeyName == "" {
		apiKeyName = fmt.Sprintf("MCP Client (%s)", clientID)
	}

	apiKeyResult, err := h.authService.GenerateAPIKey(r.Context(), userID, apiKeyName)
	if err != nil {
		h.logger.Error("failed to create api key", "error", err, "user_id", userID)
		http.Error(w, "failed to create api key", http.StatusInternalServerError)
		return
	}

	authCode, err := h.service.GenerateAuthCode(
		userID,
		clientID,
		redirectURI,
		codeChallenge,
		apiKeyResult.ID,
		apiKeyResult.FullKey,
	)
	if err != nil {
		h.logger.Error("failed to generate auth code", "error", err, "user_id", userID)
		http.Error(w, "failed to generate auth code", http.StatusInternalServerError)
		return
	}

	// Clear the OAuth context cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "mcp_oauth_context",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	redirectURL, _ := url.Parse(redirectURI)
	q := redirectURL.Query()
	q.Set("code", authCode)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()

	response := map[string]any{
		"redirect_url": redirectURL.String(),
		"code":         authCode,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handlers) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	redirectURI := r.FormValue("redirect_uri")

	if grantType != "authorization_code" {
		respondTokenError(w, "unsupported_grant_type", "only authorization_code is supported")
		return
	}

	if code == "" {
		respondTokenError(w, "invalid_request", "code is required")
		return
	}

	if codeVerifier == "" {
		respondTokenError(w, "invalid_request", "code_verifier is required")
		return
	}

	claims, err := h.service.ValidateAuthCode(code)
	if err != nil {
		h.logger.Debug("invalid auth code", "error", err)
		respondTokenError(w, "invalid_grant", "invalid or expired code")
		return
	}

	if redirectURI != "" && redirectURI != claims.RedirectURI {
		respondTokenError(w, "invalid_grant", "redirect_uri mismatch")
		return
	}

	if !VerifyPKCE(codeVerifier, claims.CodeChallenge) {
		respondTokenError(w, "invalid_grant", "invalid code_verifier")
		return
	}

	response := map[string]any{
		"access_token": claims.APIKey,
		"token_type":   "Bearer",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(response)
}

func respondTokenError(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func NewMCPOAuthService(config Config, authConfig auth.Config) *Service {
	return NewService(config, authConfig.JWTSecret)
}
