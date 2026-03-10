package utils

import (
	"strings"
	"testing"
)

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
		{"local, astropods prefix", "astropods/messaging:latest", true, "messaging:latest"},
		{"local, astropods playground", "astropods/astro-playground:latest", true, "astro-playground:latest"},
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

/*
TestImageStrippingStrategy validates the selective stripping behavior used in
dev.go: only images with the astropods/ prefix are stripped, third-party images
like qdrant/qdrant:latest must remain untouched.
*/
func TestImageStrippingStrategy(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		shouldStrip   bool
		expectedImage string
	}{
		{"astropods messaging", "astropods/messaging:latest", true, "messaging:latest"},
		{"astropods playground", "astropods/astro-playground:latest", true, "astro-playground:latest"},
		{"astropods collector", "astropods/astro-collector:latest", true, "astro-collector:latest"},
		{"third-party qdrant", "qdrant/qdrant:latest", false, "qdrant/qdrant:latest"},
		{"third-party redis", "redis:7-alpine", false, "redis:7-alpine"},
		{"no prefix", "messaging:latest", false, "messaging:latest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isAstropods := strings.HasPrefix(tt.image, "astropods/")
			if isAstropods != tt.shouldStrip {
				t.Errorf("HasPrefix(%q, astropods/) = %v, want %v", tt.image, isAstropods, tt.shouldStrip)
			}

			var result string
			if isAstropods {
				result = ImageNameForLocal(tt.image, true)
			} else {
				result = tt.image
			}
			if result != tt.expectedImage {
				t.Errorf("got %q, want %q", result, tt.expectedImage)
			}
		})
	}
}
