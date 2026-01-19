package mcp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/deployment/flyio"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	authService *auth.Service
	logger      *slog.Logger
}

type ListToolsResponse struct {
	Tools []Tool `json:"tools"`
}

type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type CallToolResponse struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

func NewHandlers(authService *auth.Service, logger *slog.Logger) *Handlers {
	return &Handlers{
		authService: authService,
		logger:      logger,
	}
}

func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Get("/mcp/tools", h.HandleListTools)
	r.Post("/mcp/tools/call", h.HandleCallTool)
}

func (h *Handlers) HandleListTools(w http.ResponseWriter, r *http.Request) {
	user, decryptedToken, err := h.getUserFromAPIKey(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.FlyioToken.Valid || user.FlyioToken.String == "" {
		http.Error(w, "Fly.io credentials not configured", http.StatusBadRequest)
		return
	}

	client := flyio.NewClient(decryptedToken, user.FlyioOrg.String)
	server := NewServer(client)

	tools := server.ListTools()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListToolsResponse{
		Tools: tools,
	})
}

func (h *Handlers) HandleCallTool(w http.ResponseWriter, r *http.Request) {
	user, decryptedToken, err := h.getUserFromAPIKey(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.FlyioToken.Valid || user.FlyioToken.String == "" {
		http.Error(w, "Fly.io credentials not configured", http.StatusBadRequest)
		return
	}

	var req CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	client := flyio.NewClient(decryptedToken, user.FlyioOrg.String)
	server := NewServer(client)

	result, err := server.CallTool(r.Context(), req.Name, req.Arguments)
	if err != nil {
		h.logger.Error("Error calling tool", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handlers) getUserFromAPIKey(r *http.Request) (*users.User, string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, "", http.ErrNoCookie
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, "", http.ErrNoCookie
	}

	apiKey := authHeader[7:]

	userID, err := h.authService.ValidateAPIKey(r.Context(), apiKey)
	if err != nil {
		return nil, "", err
	}

	user, err := h.authService.GetUserByID(r.Context(), userID)
	if err != nil {
		return nil, "", err
	}

	decryptedToken := ""
	if user.FlyioToken.Valid && user.FlyioToken.String != "" {
		decryptedToken, err = h.authService.DecryptToken(user.FlyioToken.String)
		if err != nil {
			h.logger.Error("Failed to decrypt Fly.io token", "error", err)
			return nil, "", err
		}
	}

	return user, decryptedToken, nil
}
