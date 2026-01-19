package graph

import (
	"context"
	"fmt"
	"strconv"

	"github.com/augustdev/autoclip/internal/authz"
)

func getUserIDFromContext(ctx context.Context) (int64, error) {
	securityCtx, err := authz.ForErr(ctx)
	if err != nil {
		return 0, fmt.Errorf("unauthorized: %w", err)
	}

	userIDStr := securityCtx.GetUserID()
	if userIDStr == "" {
		return 0, fmt.Errorf("unauthorized: no user ID in security context")
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid user ID format: %w", err)
	}

	return userID, nil
}
