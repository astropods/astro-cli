package handlers

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-registry/internal/auth"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
)

func TestParseClusterPullCredential(t *testing.T) {
	tests := []struct {
		name          string
		password      string
		wantOK        bool
		wantClusterID string
		wantSecret    string
	}{
		{"primary", "astrocp_primary_abc123", true, "primary", "abc123"},
		{"additional with hyphens", "astrocp_eu-west-1-managed_s3cr3t", true, "eu-west-1-managed", "s3cr3t"},
		{"secret containing underscores", "astrocp_primary_a_b_c", true, "primary", "a_b_c"},
		{"workos token (not a CPC)", "eyJhbGciOi...", false, "", ""},
		{"prefix only", "astrocp_", false, "", ""},
		{"empty cluster id", "astrocp__secret", false, "", ""},
		{"empty secret", "astrocp_primary_", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterID, secret, ok := parseClusterPullCredential(tt.password)
			if ok != tt.wantOK || clusterID != tt.wantClusterID || secret != tt.wantSecret {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					clusterID, secret, ok, tt.wantClusterID, tt.wantSecret, tt.wantOK)
			}
		})
	}
}

// fakeClusterAuthorizer reports homing from a fixed set; Authenticate always ok.
type fakeClusterAuthorizer struct {
	// namespace (name or id) → resolved account id; absence means "not homed here".
	resolved map[string]string
}

func (f fakeClusterAuthorizer) Authenticate(context.Context, string, string) (bool, error) {
	return true, nil
}

func (f fakeClusterAuthorizer) ResolveHomedAccount(_ context.Context, namespace, _ string) (string, bool, error) {
	id, ok := f.resolved[namespace]
	return id, ok, nil
}

func TestAuthorizeClusterScope(t *testing.T) {
	log := logger.New("error", "text")
	// "acme" (a name) is homed and resolves to account id "01acme"; "other" is not homed here.
	az := fakeClusterAuthorizer{resolved: map[string]string{"acme": "01acme"}}
	ctx := context.Background()

	repo := func(name string, actions ...string) auth.ResourceAccess {
		return auth.ResourceAccess{Type: "repository", Name: name, Actions: actions}
	}

	t.Run("homed tenant granted pull; scope carries resolved account id", func(t *testing.T) {
		got := authorizeClusterScope(ctx, repo("acme/agent", "pull"), "cluster-x", az, log)
		if len(got.Actions) != 1 || got.Actions[0] != "pull" || got.AccountID != "01acme" {
			t.Errorf("want pull-only resolving acme→01acme, got %+v", got)
		}
	})

	t.Run("tenant homed elsewhere is dropped (isolation)", func(t *testing.T) {
		got := authorizeClusterScope(ctx, repo("other/agent", "pull"), "cluster-x", az, log)
		if len(got.Actions) != 0 {
			t.Errorf("want no actions for non-homed tenant, got %+v", got)
		}
	})

	t.Run("push/delete downgraded to pull-only", func(t *testing.T) {
		got := authorizeClusterScope(ctx, repo("acme/agent", "push", "pull", "delete"), "cluster-x", az, log)
		if len(got.Actions) != 1 || got.Actions[0] != "pull" {
			t.Errorf("want pull-only, got %+v", got.Actions)
		}
	})

	t.Run("non-repository scope yields nothing", func(t *testing.T) {
		got := authorizeClusterScope(ctx, auth.ResourceAccess{Type: "registry", Name: "catalog", Actions: []string{"*"}}, "cluster-x", az, log)
		if len(got.Actions) != 0 {
			t.Errorf("want no actions for non-repository scope, got %+v", got)
		}
	})
}
