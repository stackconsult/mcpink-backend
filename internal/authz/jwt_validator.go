package authz

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"
)

// JWTTokenValidator is a concrete implementation of TokenValidator for JWT tokens.
type JWTTokenValidator struct {
	keySet  jwk.Set
	jwksURL string
	devMode bool
}

// NewTokenValidator creates a new JWT token validator with the given JWKS URL.
func NewTokenValidator(jwksURL string) (TokenValidator, error) {
	if jwksURL == "" {
		return &JWTTokenValidator{
			keySet:  nil,
			jwksURL: "",
			devMode: true,
		}, nil
	}

	keySet, err := jwk.Fetch(context.Background(), jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", jwksURL, err)
	}

	return &JWTTokenValidator{
		keySet:  keySet,
		jwksURL: jwksURL,
		devMode: false,
	}, nil
}

// RefreshKeys refreshes the JWKS from the URL.
func (v *JWTTokenValidator) RefreshKeys() error {
	if v.jwksURL == "" {
		return ErrNoJWKS
	}

	keySet, err := jwk.Fetch(context.Background(), v.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to refresh JWKS from %s: %w", v.jwksURL, err)
	}

	v.keySet = keySet
	return nil
}

// ValidateToken validates a JWT token and returns the user ID and roles.
func (v *JWTTokenValidator) ValidateToken(tokenString string) (string, []string, error) {
	if v.devMode {
		token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &StandardClaims{})
		if err != nil {
			return "", nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}

		if claims, ok := token.Claims.(*StandardClaims); ok {
			if claims.Subject == "" {
				return "", nil, fmt.Errorf("%w: no subject (sub) found in token claims", ErrInvalidToken)
			}
			return claims.Subject, claims.Roles, nil
		}
		return "", nil, ErrInvalidToken
	}

	if v.keySet == nil {
		return "", nil, ErrNoJWKS
	}

	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &StandardClaims{})
	if err != nil {
		return "", nil, fmt.Errorf("%w: failed to parse token header: %v", ErrInvalidToken, err)
	}

	kid, ok := token.Header["kid"].(string)
	if !ok {
		return "", nil, fmt.Errorf("%w: token header missing kid", ErrInvalidToken)
	}

	key, found := v.keySet.LookupKeyID(kid)
	if !found {
		if err := v.RefreshKeys(); err != nil {
			return "", nil, fmt.Errorf("%w: key with ID %s not found and failed to refresh keys: %v", ErrInvalidToken, kid, err)
		}

		key, found = v.keySet.LookupKeyID(kid)
		if !found {
			var availableKeys []string
			for i := 0; i < v.keySet.Len(); i++ {
				k, _ := v.keySet.Get(i)
				availableKeys = append(availableKeys, k.KeyID())
			}
			return "", nil, fmt.Errorf("%w: key with ID %s not found, available keys: %v", ErrInvalidToken, kid, availableKeys)
		}
	}

	var rawKey interface{}
	if err := key.Raw(&rawKey); err != nil {
		return "", nil, fmt.Errorf("%w: failed to get raw key: %v", ErrInvalidToken, err)
	}

	validatedToken, err := jwt.ParseWithClaims(
		tokenString,
		&StandardClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return rawKey, nil
		},
	)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := validatedToken.Claims.(*StandardClaims)
	if !ok || !validatedToken.Valid {
		return "", nil, ErrInvalidToken
	}

	if !claims.VerifyExpiresAt(time.Now(), true) {
		return "", nil, ErrExpiredToken
	}

	if claims.Subject == "" {
		return "", nil, fmt.Errorf("%w: no subject (sub) found in token claims", ErrInvalidToken)
	}

	return claims.Subject, claims.Roles, nil
}
