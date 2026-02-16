package internalgit

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	petname "github.com/dustinkirkland/golang-petname"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
)

const (
	maxUsernameAttempts = 10
	suffixCharset       = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffixLen           = 2
)

// ResolveUsername returns the user's git username, generating and
// persisting one if it doesn't exist yet. The generated name is a two-word
// petname (e.g. "funky-beaver"). On collision a short random suffix is appended.
func (s *Service) ResolveUsername(ctx context.Context, user users.User) (string, error) {
	if user.GiteaUsername != nil && *user.GiteaUsername != "" {
		return *user.GiteaUsername, nil
	}

	candidate := petname.Generate(2, "-")

	for attempt := 0; attempt < maxUsernameAttempts; attempt++ {
		name := candidate
		if attempt > 0 {
			suffix, err := randomSuffix()
			if err != nil {
				return "", fmt.Errorf("generate suffix: %w", err)
			}
			name = candidate + "-" + suffix
		}

		_, err := s.userQueries.GetUserByGiteaUsername(ctx, &name)
		if err != nil {
			// Not found — name is available
			_, setErr := s.userQueries.SetGiteaUsername(ctx, users.SetGiteaUsernameParams{
				ID:            user.ID,
				GiteaUsername: &name,
			})
			if setErr != nil {
				return "", fmt.Errorf("persist username: %w", setErr)
			}
			return name, nil
		}
		// Name taken — retry with suffix
	}

	return "", fmt.Errorf("failed to find unique username after %d attempts", maxUsernameAttempts)
}


func randomSuffix() (string, error) {
	b := make([]byte, suffixLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(suffixCharset))))
		if err != nil {
			return "", err
		}
		b[i] = suffixCharset[n.Int64()]
	}
	return string(b), nil
}
