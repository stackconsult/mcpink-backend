package internalgit

import "time"

type Config struct {
	BaseURL       string
	AdminToken    string
	UserPrefix    string
	WebhookSecret string
}

const (
	DefaultTimeout       = 30 * time.Second
	DefaultTokenDuration = 1 * time.Hour
)
