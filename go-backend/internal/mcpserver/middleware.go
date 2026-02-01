package mcpserver

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/augustdev/autoclip/internal/auth"
)

func AuthMiddleware(authService *auth.Service, logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		token := parts[1]

		userID, err := authService.ValidateAPIKey(r.Context(), token)
		if err != nil {
			logger.Debug("invalid api key", "error", err)
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}

		user, err := authService.GetUserByID(r.Context(), userID)
		if err != nil {
			logger.Error("failed to get user", "error", err, "user_id", userID)
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		ctx := ContextWithUser(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
