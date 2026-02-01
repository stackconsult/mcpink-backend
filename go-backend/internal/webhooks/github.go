package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/apps"
)

type GitHubPushPayload struct {
	Ref        string               `json:"ref"`
	Repository GitHubPushRepository `json:"repository"`
}

type GitHubPushRepository struct {
	FullName string `json:"full_name"`
}

type WebhookResponse struct {
	Message     string           `json:"message"`
	Deployments []DeploymentInfo `json:"deployments,omitempty"`
}

type DeploymentInfo struct {
	AppID      string `json:"app_id"`
	WorkflowID string `json:"workflow_id"`
}

func (h *Handlers) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("received GitHub webhook",
		"method", r.Method,
		"event", r.Header.Get("X-GitHub-Event"),
		"delivery", r.Header.Get("X-GitHub-Delivery"),
		"signature_present", r.Header.Get("X-Hub-Signature-256") != "")

	// Only handle push events
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "push" {
		h.logger.Info("ignoring non-push event", "event", eventType)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(WebhookResponse{Message: "ignored event type: " + eventType})
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Verify signature
	signature := r.Header.Get("X-Hub-Signature-256")
	h.logger.Info("verifying webhook signature",
		"signature", signature,
		"body_length", len(body))
	if !h.verifySignature(body, signature) {
		h.logger.Warn("invalid webhook signature",
			"signature", signature)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	h.logger.Info("webhook signature verified")

	// Parse payload
	var payload GitHubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("failed to parse payload", "error", err)
		http.Error(w, "failed to parse payload", http.StatusBadRequest)
		return
	}

	// Extract repo and branch
	repo := payload.Repository.FullName
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	h.logger.Info("received push webhook",
		"repo", repo,
		"branch", branch)

	// Find apps for this repo/branch
	matchingApps, err := h.appsQ.GetAppsByRepoBranch(r.Context(), apps.GetAppsByRepoBranchParams{
		Repo:   repo,
		Branch: branch,
	})
	if err != nil {
		h.logger.Error("failed to query apps", "error", err)
		http.Error(w, "failed to query apps", http.StatusInternalServerError)
		return
	}

	h.logger.Info("found matching apps",
		"repo", repo,
		"branch", branch,
		"count", len(matchingApps))

	if len(matchingApps) == 0 {
		h.logger.Info("no apps found for repo/branch",
			"repo", repo,
			"branch", branch)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(WebhookResponse{Message: "no apps found for this repo/branch"})
		return
	}

	// Start redeploy workflow for each app
	var deployments []DeploymentInfo
	for _, app := range matchingApps {
		if app.CoolifyAppUuid == nil {
			h.logger.Warn("app has no coolify uuid, skipping",
				"appID", app.ID)
			continue
		}

		workflowID, err := h.deployService.RedeployApp(r.Context(), app.ID, *app.CoolifyAppUuid)
		if err != nil {
			h.logger.Error("failed to start redeploy workflow",
				"appID", app.ID,
				"error", err)
			continue
		}

		h.logger.Info("started redeploy workflow",
			"appID", app.ID,
			"workflowID", workflowID)

		deployments = append(deployments, DeploymentInfo{
			AppID:      app.ID,
			WorkflowID: workflowID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(WebhookResponse{
		Message:     "redeploy workflows started",
		Deployments: deployments,
	})
}

func (h *Handlers) verifySignature(body []byte, signature string) bool {
	if h.config.WebhookSecret == "" {
		h.logger.Warn("webhook secret not configured, skipping verification")
		return true
	}

	if signature == "" {
		h.logger.Warn("no signature provided in request")
		return false
	}

	// Remove "sha256=" prefix
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(h.config.WebhookSecret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	valid := hmac.Equal([]byte(signature), []byte(expectedMAC))
	if !valid {
		h.logger.Warn("signature mismatch",
			"expected", expectedMAC,
			"got", signature)
	}
	return valid
}
