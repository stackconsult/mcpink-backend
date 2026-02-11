package k8sdeployments

import "testing"

func TestEffectiveAppPort(t *testing.T) {
	dist := "dist"

	tests := []struct {
		name       string
		buildPack  string
		appPort    string
		publishDir *string
		want       string
	}{
		{
			name:      "default railpack port",
			buildPack: "railpack",
			want:      "3000",
		},
		{
			name:      "respects explicit port",
			buildPack: "railpack",
			appPort:   "4200",
			want:      "4200",
		},
		{
			name:       "railpack publish directory forces 8080",
			buildPack:  "railpack",
			appPort:    "3000",
			publishDir: &dist,
			want:       "8080",
		},
		{
			name:      "static buildpack forces 8080",
			buildPack: "static",
			appPort:   "80",
			want:      "8080",
		},
		{
			name:       "nixpacks with publish directory forces 8080",
			buildPack:  "nixpacks",
			appPort:    "3000",
			publishDir: &dist,
			want:       "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveAppPort(tt.buildPack, tt.appPort, tt.publishDir)
			if got != tt.want {
				t.Fatalf("effectiveAppPort(%q, %q) = %q, want %q", tt.buildPack, tt.appPort, got, tt.want)
			}
		})
	}
}
