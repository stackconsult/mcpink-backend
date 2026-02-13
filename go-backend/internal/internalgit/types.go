package internalgit

// CreateRepoResult is returned after creating a repo
type CreateRepoResult struct {
	Repo      string `json:"repo"`
	GitRemote string `json:"git_remote"`
	ExpiresAt string `json:"expires_at"`
	Message   string `json:"message"`
}

// GetPushTokenResult is returned when getting a push token
type GetPushTokenResult struct {
	GitRemote string `json:"git_remote"`
	ExpiresAt string `json:"expires_at"`
}
