package handlers

import "testing"

func TestRegistrableDomainOf(t *testing.T) {
	cases := []struct {
		name string
		peer string
		want string
	}{
		{"subdomain", "api.slack.com", "slack.com"},
		{"deep subdomain", "shard-0.edge.acme-cdn.io", "acme-cdn.io"},
		{"apex", "acme.io", "acme.io"},
		{"strips port", "api.acme.io:443", "acme.io"},
		{"strips trailing dot", "api.acme.io.", "acme.io"},
		{"uppercase folds", "API.Acme.IO", "acme.io"},

		// The cases a label-count heuristic gets wrong.
		{"multi-label suffix", "api.example.co.uk", "example.co.uk"},
		{"multi-label suffix apex", "example.co.uk", "example.co.uk"},
		{"com.au", "cdn.acme.com.au", "acme.com.au"},
		// github.io is itself a public suffix, so these are unrelated parties
		// and must not collapse onto a shared "github.io".
		{"private suffix keeps the owner label", "alice.github.io", "alice.github.io"},
		{"private suffix, other owner", "bob.github.io", "bob.github.io"},
		{"vercel.app", "myapp.vercel.app", "myapp.vercel.app"},

		// Not registrable domains — the client treats these as standalone.
		{"ipv4", "10.0.14.22", ""},
		{"ipv4 with port", "10.0.14.22:5432", ""},
		{"ipv6", "::1", ""},
		{"single label", "internal-svc", ""},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := registrableDomainOf(tc.peer); got != tc.want {
				t.Errorf("registrableDomainOf(%q) = %q, want %q", tc.peer, got, tc.want)
			}
		})
	}
}
