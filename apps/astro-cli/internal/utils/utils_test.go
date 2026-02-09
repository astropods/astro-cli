package utils

import "testing"

func TestImageNameForLocal(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		isLocal  bool
		expected string
	}{
		{"not local, full image", "ghcr.io/org/astro-playground:latest", false, "ghcr.io/org/astro-playground:latest"},
		{"local, full image", "ghcr.io/org/astro-playground:latest", true, "astro-playground:latest"},
		{"local, single segment", "astro-playground:latest", true, "astro-playground:latest"},
		{"not local, single segment", "astro-playground:latest", false, "astro-playground:latest"},
		{"local, registry with port", "myreg.io:5000/repo/img:v1", true, "img:v1"},
		{"local, empty string", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImageNameForLocal(tt.image, tt.isLocal)
			if got != tt.expected {
				t.Errorf("ImageNameForLocal(%q, %v) = %q, want %q", tt.image, tt.isLocal, got, tt.expected)
			}
		})
	}
}
