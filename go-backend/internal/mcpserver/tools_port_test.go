package mcpserver

import "testing"

func TestResolveServicePort(t *testing.T) {
	tests := []struct {
		name          string
		buildPack     string
		publishDir    string
		requestedPort int
		want          string
	}{
		{
			name:      "railpack default",
			buildPack: "railpack",
			want:      "3000",
		},
		{
			name:          "railpack custom requested port",
			buildPack:     "railpack",
			requestedPort: 4000,
			want:          "4000",
		},
		{
			name:       "railpack publish directory forces static port",
			buildPack:  "railpack",
			publishDir: "dist",
			want:       "8080",
		},
		{
			name:          "railpack publish directory overrides explicit port",
			buildPack:     "railpack",
			publishDir:    "dist",
			requestedPort: 3000,
			want:          "8080",
		},
		{
			name:      "static default",
			buildPack: "static",
			want:      "80",
		},
		{
			name:          "static explicit port",
			buildPack:     "static",
			requestedPort: 9000,
			want:          "9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveServicePort(tt.buildPack, tt.publishDir, tt.requestedPort)
			if got != tt.want {
				t.Fatalf("resolveServicePort(%q, %q, %d) = %q, want %q",
					tt.buildPack, tt.publishDir, tt.requestedPort, got, tt.want)
			}
		})
	}
}
