package authz

import (
	"testing"
)

func TestJWTSecurityContext_HasRole(t *testing.T) {
	tests := []struct {
		name      string
		roles     []string
		checkRole string
		want      bool
	}{
		{
			name:      "exact match",
			roles:     []string{"admin", "user"},
			checkRole: "admin",
			want:      true,
		},
		{
			name:      "case insensitive",
			roles:     []string{"ADMIN", "USER"},
			checkRole: "admin",
			want:      true,
		},
		{
			name:      "mixed case",
			roles:     []string{"AdMiN", "UsEr"},
			checkRole: "admin",
			want:      true,
		},
		{
			name:      "not found",
			roles:     []string{"user", "editor"},
			checkRole: "admin",
			want:      false,
		},
		{
			name:      "empty roles",
			roles:     []string{},
			checkRole: "admin",
			want:      false,
		},
		{
			name:      "middle of list",
			roles:     []string{"user", "moderator", "editor"},
			checkRole: "MODERATOR",
			want:      true,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := &JWTSecurityContext{
				UserID: "test-user-id",
				Roles:  tt.roles,
			}
			if got := ctx.HasRole(tt.checkRole); got != tt.want {
				t.Errorf("HasRole() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJWTSecurityContext_GetUserID(t *testing.T) {
	t.Parallel()
	ctx := &JWTSecurityContext{
		UserID: "test-user-123",
		Roles:  []string{"admin"},
	}

	if got := ctx.GetUserID(); got != "test-user-123" {
		t.Errorf("GetUserID() = %v, want %v", got, "test-user-123")
	}
}

func TestJWTSecurityContext_HasUserID(t *testing.T) {
	ctx := &JWTSecurityContext{
		UserID: "test-user-123",
		Roles:  []string{"admin"},
	}

	tests := []struct {
		name   string
		userID string
		want   bool
	}{
		{
			name:   "match",
			userID: "test-user-123",
			want:   true,
		},
		{
			name:   "no match",
			userID: "different-user",
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ctx.HasUserID(tt.userID); got != tt.want {
				t.Errorf("HasUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJWTSecurityContext_GetRoles(t *testing.T) {
	t.Parallel()
	roles := []string{"admin", "user", "moderator"}
	ctx := &JWTSecurityContext{
		UserID: "test-user-123",
		Roles:  roles,
	}

	got := ctx.GetRoles()
	if len(got) != len(roles) {
		t.Errorf("GetRoles() length = %v, want %v", len(got), len(roles))
	}

	for i, role := range roles {
		if got[i] != role {
			t.Errorf("GetRoles()[%d] = %v, want %v", i, got[i], role)
		}
	}
}
