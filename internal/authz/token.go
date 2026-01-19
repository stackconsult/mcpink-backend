package authz

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
	ErrNoJWKS       = errors.New("no JWKS URL provided")
)

// StandardClaims represents the standard claims in a JWT token.
type StandardClaims struct {
	Roles []string `json:"roles,omitempty"` // Custom claim for user roles
	jwt.RegisteredClaims
}

// TokenValidator validates JWT tokens and extracts user ID and roles.
type TokenValidator interface {
	ValidateToken(tokenString string) (userID string, roles []string, err error)
}

// ExtractBearerToken extracts the bearer token from the Authorization header.
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("authorization header is required")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", errors.New("authorization header format must be 'Bearer {token}'")
	}

	return parts[1], nil
}

// NewAuthMiddleware creates a clean middleware function for chi router.
func NewAuthMiddleware(validator TokenValidator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return Middleware(next, validator.ValidateToken, logger)
	}
}

// CookieConfig holds configuration for cookie-based authentication.
type CookieConfig struct {
	CookieName   string
	ValidateFunc func(token string) (userID string, err error)
}

// MiddlewareConfig holds configuration for the auth middleware.
type MiddlewareConfig struct {
	Cookie *CookieConfig
}

// Middleware creates a middleware that validates bearer tokens and adds security context.
func Middleware(next http.Handler, validateToken func(string) (string, []string, error), logger *slog.Logger) http.Handler {
	return MiddlewareWithConfig(next, validateToken, logger, nil)
}

// MiddlewareWithConfig creates a middleware with additional configuration options.
func MiddlewareWithConfig(next http.Handler, validateToken func(string) (string, []string, error), logger *slog.Logger, config *MiddlewareConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID string
		var roles []string
		var authenticated bool

		// Try Bearer token first
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token, err := ExtractBearerToken(authHeader)
			if err == nil {
				userID, roles, err = validateToken(token)
				if err == nil && userID != "" {
					authenticated = true
				}
			}
		}

		// Try cookie if Bearer token not present or invalid
		if !authenticated && config != nil && config.Cookie != nil {
			cookie, err := r.Cookie(config.Cookie.CookieName)
			if err == nil && cookie.Value != "" {
				userID, err = config.Cookie.ValidateFunc(cookie.Value)
				if err == nil && userID != "" {
					authenticated = true
					roles = []string{}
				}
			}
		}

		if authenticated {
			sc := &JWTSecurityContext{
				UserID: userID,
				Roles:  roles,
			}
			ctx := To(r.Context(), sc)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func maskAuthHeader(header string) string {
	parts := strings.Split(header, " ")
	if len(parts) != 2 {
		return "[invalid format]"
	}

	token := parts[1]
	if len(token) <= 10 {
		return parts[0] + " [masked]"
	}

	return parts[0] + " " + token[:5] + "..." + token[len(token)-5:]
}
