package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/augustdev/autoclip/internal/auth"
)

type APIKeyValidator struct {
	authService *auth.Service
}

func NewAPIKeyValidator(authService *auth.Service) *APIKeyValidator {
	return &APIKeyValidator{
		authService: authService,
	}
}

func (v *APIKeyValidator) ValidateToken(tokenString string) (string, []string, error) {
	if !strings.HasPrefix(tokenString, "dk_live_") {
		return "", nil, fmt.Errorf("invalid api key format")
	}

	userID, err := v.authService.ValidateAPIKey(context.Background(), tokenString)
	if err != nil {
		return "", nil, fmt.Errorf("invalid api key: %w", err)
	}

	return fmt.Sprintf("%d", userID), []string{"user"}, nil
}
