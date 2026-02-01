package mcpserver

import (
	"context"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
)

type ctxKey string

const userCtxKey ctxKey = "mcp_user"

func ContextWithUser(ctx context.Context, user *users.User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

func UserFromContext(ctx context.Context) *users.User {
	user, _ := ctx.Value(userCtxKey).(*users.User)
	return user
}
