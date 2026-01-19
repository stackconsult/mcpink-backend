package auth

import "time"

type Config struct {
	JWTSecret            string
	JWTExpiry            time.Duration
	APIKeyEncryptionKey  string
	SessionCookieName    string
	SessionCookieSecure  bool
	SessionCookieDomain  string
	FrontendURL          string
}
