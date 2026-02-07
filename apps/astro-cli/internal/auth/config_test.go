package auth

import "testing"

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty returns default", "", DefaultServerURL},
		{"whitespace returns default", "  ", DefaultServerURL},
		{"bare hostname adds https", "example.com", "https://example.com"},
		{"hostname with path adds https", "example.com/foo", "https://example.com/foo"},
		{"https already present", "https://example.com", "https://example.com"},
		{"https with trailing slash", "https://example.com/", "https://example.com"},
		{"http preserved", "http://localhost:8080", "http://localhost:8080"},
		{"trimmed input", "  https://example.com  ", "https://example.com"},
		{"with port", "https://example.com:443", "https://example.com:443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeServerURL(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeServerURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

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

func TestNormalizeThenRegistry(t *testing.T) {
	// Typical login flow: host flag -> normalize -> registry
	host := "example.com"
	server := NormalizeServerURL(host)
	registry := RegistryURLFromServerURL(server)
	if server != "https://example.com" {
		t.Errorf("server = %q, want https://example.com", server)
	}
	if registry != "https://registry.example.com" {
		t.Errorf("registry = %q, want https://registry.example.com", registry)
	}
}
