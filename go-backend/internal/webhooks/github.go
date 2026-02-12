package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/augustdev/autoclip/internal/helpers"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/githubcreds"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
)

type GitHubPushPayload struct {
	Ref        string               `json:"ref"`
	After      string               `json:"after"`
	Repository GitHubPushRepository `json:"repository"`
}

type GitHubPushRepository struct {
	FullName string `json:"full_name"`
}

type GitHubInstallationPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Sender struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"sender"`
}

type WebhookResponse struct {
	Message     string           `json:"message"`
	Deployments []DeploymentInfo `json:"deployments,omitempty"`
}

type DeploymentInfo struct {
	ServiceID  string `json:"service_id"`
	WorkflowID string `json:"workflow_id"`
}

func (h *Handlers) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	eventType := r.Header.Get("X-GitHub-Event")

	// Read body first so we can log it
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	h.logger.Info("received GitHub webhook",
		"method", r.Method,
		"event", eventType,
		"delivery", r.Header.Get("X-GitHub-Delivery"),
		"signature_present", r.Header.Get("X-Hub-Signature-256") != "")

	// Verify signature first
	signature := r.Header.Get("X-Hub-Signature-256")
	if !h.verifySignature(body, signature) {
		h.logger.Warn("invalid webhook signature", "signature", signature)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	switch eventType {
	case "installation":
		h.handleInstallationWebhook(w, r, body)
		return
	case "push":
		h.handlePushWebhook(w, r, body)
		return
	default:
		h.logger.Info("ignoring unhandled event", "event", eventType)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(WebhookResponse{Message: "ignored event type: " + eventType})
		return
	}
}

func (h *Handlers) handleInstallationWebhook(w http.ResponseWriter, r *http.Request, body []byte) {
	var payload GitHubInstallationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("failed to parse installation payload", "error", err)
		http.Error(w, "failed to parse payload", http.StatusBadRequest)
		return
	}

	h.logger.Info("processing installation webhook",
		"action", payload.Action,
		"installation_id", payload.Installation.ID,
		"sender_id", payload.Sender.ID,
		"sender_login", payload.Sender.Login)

	// Find user by GitHub ID
	creds, err := h.githubCredsQ.GetGitHubCredsByGitHubID(r.Context(), payload.Sender.ID)
	if err != nil {
		h.logger.Warn("no user found for github id", "github_id", payload.Sender.ID)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(WebhookResponse{Message: "no user found"})
		return
	}

	var installationID int64
	switch payload.Action {
	case "created":
		installationID = payload.Installation.ID
	case "deleted":
		installationID = 0
	default:
		h.logger.Info("ignoring installation action", "action", payload.Action)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(WebhookResponse{Message: "ignored action: " + payload.Action})
		return
	}

	if installationID == 0 {
		if _, err := h.githubCredsQ.ClearGitHubAppInstallation(r.Context(), creds.UserID); err != nil {
			h.logger.Error("failed to clear installation", "error", err, "user_id", creds.UserID)
			http.Error(w, fmt.Sprintf("failed to clear installation: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := h.githubCredsQ.SetGitHubAppInstallation(r.Context(), githubcreds.SetGitHubAppInstallationParams{
			UserID:                  creds.UserID,
			GithubAppInstallationID: helpers.Ptr(installationID),
		}); err != nil {
			h.logger.Error("failed to set installation", "error", err, "user_id", creds.UserID)
			http.Error(w, fmt.Sprintf("failed to set installation: %v", err), http.StatusInternalServerError)
			return
		}
	}

	h.logger.Info("installation synced", "user_id", creds.UserID, "installation_id", installationID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(WebhookResponse{Message: "installation synced"})
}

func (h *Handlers) handlePushWebhook(w http.ResponseWriter, r *http.Request, body []byte) {
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
	delivery := r.Header.Get("X-GitHub-Delivery")
	after := strings.TrimSpace(payload.After)

	h.logger.Info("received push webhook",
		"repo", repo,
		"branch", branch,
		"after", after,
		"delivery", delivery)

	// Find services for this repo/branch
	matchingServices, err := h.servicesQ.GetServicesByRepoBranch(r.Context(), services.GetServicesByRepoBranchParams{
		Repo:   repo,
		Branch: branch,
	})
	if err != nil {
		h.logger.Error("failed to query services", "error", err)
		http.Error(w, "failed to query services", http.StatusInternalServerError)
		return
	}

	h.logger.Info("found matching services",
		"repo", repo,
		"branch", branch,
		"count", len(matchingServices))

	if len(matchingServices) == 0 {
		h.logger.Info("no services found for repo/branch",
			"repo", repo,
			"branch", branch)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(WebhookResponse{Message: "no services found for this repo/branch"})
		return
	}

	// Start redeploy workflow for each service
	var deploys []DeploymentInfo
	for _, svc := range matchingServices {
		workflowID, err := h.deployService.RedeployFromGitHubPush(r.Context(), svc.ID, after, delivery)
		if err != nil {
			h.logger.Error("failed to start redeploy workflow",
				"serviceID", svc.ID,
				"error", err)
			continue
		}

		h.logger.Info("started redeploy workflow",
			"serviceID", svc.ID,
			"workflowID", workflowID)

		deploys = append(deploys, DeploymentInfo{
			ServiceID:  svc.ID,
			WorkflowID: workflowID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(WebhookResponse{
		Message:     "redeploy workflows started",
		Deployments: deploys,
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
