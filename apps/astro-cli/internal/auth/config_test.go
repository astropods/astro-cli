package auth

import "testing"

func TestRegistryURLFromServerURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		want      string
	}{
		{"https default host", "https://example.com", "https://registry.example.com"},
		{"https with trailing slash", "https://example.com/", "https://registry.example.com"},
		{"http scheme", "http://example.com", "http://registry.example.com"},
		{"with port strips port", "https://example.com:443", "https://registry.example.com"},
		{"localhost", "https://localhost", "https://registry.localhost"},
		{"localhost with port", "http://localhost:8080", "http://registry.localhost"},
		{"empty returns empty", "", ""},
		{"invalid URL returns empty", "://bad", ""},
		{"no scheme defaults to https", "example.com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegistryURLFromServerURL(tt.serverURL)
			if got != tt.want {
				t.Errorf("RegistryURLFromServerURL(%q) = %q, want %q", tt.serverURL, got, tt.want)
			}
		})
	}
}
