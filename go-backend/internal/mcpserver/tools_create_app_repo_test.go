package mcpserver

import (
	"context"
	"testing"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
)

func TestNormalizeServiceRepo(t *testing.T) {
	ghUsername := "gluonfield"
	u := &users.User{ID: "test-user", GithubUsername: &ghUsername}

	// Server with nil internalGitSvc — only github.com and validation tests work
	s := &Server{}

	cases := []struct {
		name     string
		input    CreateServiceInput
		wantHost string
		wantRepo string
		wantErr  bool
	}{
		{
			name:     "github.com host expands repo name",
			input:    CreateServiceInput{Repo: "exp20", Host: "github.com"},
			wantHost: "github.com",
			wantRepo: "gluonfield/exp20",
		},
		{
			name:    "rejects owner/repo format",
			input:   CreateServiceInput{Repo: "gluonfield/exp20", Host: "ml.ink"},
			wantErr: true,
		},
		{
			name:    "rejects url",
			input:   CreateServiceInput{Repo: "https://git.ml.ink/gluonfield/exp20.git"},
			wantErr: true,
		},
		{
			name:    "rejects prefixed repo",
			input:   CreateServiceInput{Repo: "ml.ink/gluonfield/exp20", Host: "ml.ink"},
			wantErr: true,
		},
		{
			name:    "rejects embedded creds",
			input:   CreateServiceInput{Repo: "gluonfield:token@git.ml.ink/gluonfield/exp20"},
			wantErr: true,
		},
		{
			name:    "rejects paths with slashes",
			input:   CreateServiceInput{Repo: "a/b/c"},
			wantErr: true,
		},
		{
			name:    "invalid host",
			input:   CreateServiceInput{Repo: "exp20", Host: "gitlab"},
			wantErr: true,
		},
		{
			name:    "ml.ink without internalGitSvc returns error",
			input:   CreateServiceInput{Repo: "exp20", Host: "ml.ink"},
			wantErr: true,
		},
		{
			name:    "github.com without github username returns error",
			input:   CreateServiceInput{Repo: "exp20", Host: "github.com"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testUser := u
			if tc.name == "github.com without github username returns error" {
				testUser = &users.User{ID: "no-gh-user"}
			}

			gotHost, gotRepo, err := s.normalizeServiceRepo(context.Background(), testUser, tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (host=%q repo=%q)", gotHost, gotRepo)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotHost != tc.wantHost {
				t.Fatalf("host: got %q, want %q", gotHost, tc.wantHost)
			}
			if gotRepo != tc.wantRepo {
				t.Fatalf("repo: got %q, want %q", gotRepo, tc.wantRepo)
			}
		})
	}
}
