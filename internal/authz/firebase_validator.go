package authz

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
)

type FirebaseTokenValidator struct {
	authClient *auth.Client
	projectID  string
}

func NewFirebaseTokenValidator(ctx context.Context, projectID string) (TokenValidator, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Firebase Auth client: %w", err)
	}

	return &FirebaseTokenValidator{
		authClient: authClient,
		projectID:  projectID,
	}, nil
}

func (f *FirebaseTokenValidator) ValidateToken(tokenString string) (string, []string, error) {
	ctx := context.Background()

	token, err := f.authClient.VerifyIDToken(ctx, tokenString)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if token.UID == "" {
		return "", nil, fmt.Errorf("%w: no user ID found in Firebase token", ErrInvalidToken)
	}

	var roles []string
	if token.Claims != nil {
		if rolesInterface, ok := token.Claims["roles"]; ok {
			if rolesArray, ok := rolesInterface.([]interface{}); ok {
				for _, r := range rolesArray {
					if roleStr, ok := r.(string); ok {
						roles = append(roles, roleStr)
					}
				}
			}
		} else if roleInterface, ok := token.Claims["role"]; ok {
			if roleStr, ok := roleInterface.(string); ok {
				roles = []string{roleStr}
			}
		}
	}

	return token.UID, roles, nil
}
