package authz

import (
	"context"
	"errors"
	"strings"

	"github.com/99designs/gqlgen/graphql"
)

type RoleChecker interface {
	GetUserRole(ctx context.Context, id string) (string, error)
}

type contextKey struct{}

var (
	ErrNoContext     = errors.New("no security context present")
	ErrNotAuthorized = errors.New("not authorized")
)

// SecurityContext represents the security context for the current request.
type SecurityContext interface {
	GetUserID() string
	GetRoles() []string
	HasUserID(string) bool
	HasRole(role string) bool
}

// JWTSecurityContext implements SecurityContext with JWT authentication.
type JWTSecurityContext struct {
	UserID string
	Roles  []string
}

func (b *JWTSecurityContext) GetUserID() string {
	return b.UserID
}

func (b *JWTSecurityContext) HasUserID(id string) bool {
	return b.UserID == id
}

func (b *JWTSecurityContext) GetRoles() []string {
	return b.Roles
}

func (b *JWTSecurityContext) HasRole(role string) bool {
	roleLower := strings.ToLower(role)
	for _, r := range b.Roles {
		if strings.ToLower(r) == roleLower {
			return true
		}
	}
	return false
}

// WalletSecurityContext implements SecurityContext with wallet-based authentication.
type WalletSecurityContext struct {
	UserID        string
	WalletAddress string
	Roles         []string
}

func (w *WalletSecurityContext) GetUserID() string {
	return w.UserID
}

func (w *WalletSecurityContext) HasUserID(id string) bool {
	return w.UserID == id
}

func (w *WalletSecurityContext) GetRoles() []string {
	return w.Roles
}

func (w *WalletSecurityContext) HasRole(role string) bool {
	roleLower := strings.ToLower(role)
	for _, r := range w.Roles {
		if strings.ToLower(r) == roleLower {
			return true
		}
	}
	return false
}

// GetWalletAddress returns the wallet address used for authentication.
func (w *WalletSecurityContext) GetWalletAddress() string {
	return w.WalletAddress
}

// AdminAPIKeySecurityContext implements SecurityContext for admin API key authentication.
type AdminAPIKeySecurityContext struct{}

func (a *AdminAPIKeySecurityContext) GetUserID() string {
	return "admin-api-key"
}

func (a *AdminAPIKeySecurityContext) HasUserID(_ string) bool {
	return false
}

func (a *AdminAPIKeySecurityContext) GetRoles() []string {
	return []string{"ADMIN"}
}

func (a *AdminAPIKeySecurityContext) HasRole(role string) bool {
	return strings.EqualFold(role, "ADMIN")
}

// To adds a security context to a context.
func To(ctx context.Context, sc SecurityContext) context.Context {
	return context.WithValue(ctx, contextKey{}, sc)
}

// For gets the security context from a context.
func For(ctx context.Context) SecurityContext {
	if sc, ok := ctx.Value(contextKey{}).(SecurityContext); ok {
		return sc
	}
	panic(ErrNoContext)
}

// ForErr gets the security context from a context, returning an error if not present.
func ForErr(ctx context.Context) (SecurityContext, error) {
	if sc, ok := ctx.Value(contextKey{}).(SecurityContext); ok {
		return sc, nil
	}
	return nil, ErrNoContext
}

// IsAuthenticatedDirective implements the @isAuthenticated directive.
func IsAuthenticatedDirective(ctx context.Context, _ any, next graphql.Resolver) (any, error) {
	if _, err := ForErr(ctx); err != nil {
		if err == ErrNoContext {
			return nil, errors.New("authentication required: please provide a valid bearer token")
		}
		return nil, err
	}
	return next(ctx)
}
