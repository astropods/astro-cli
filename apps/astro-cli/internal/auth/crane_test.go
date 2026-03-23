package auth

import (
	"testing"
)

func TestGetCraneAuthWithToken_ReturnsStaticToken(t *testing.T) {
	token := "org-scoped-test-token-abc123"
	authenticator := GetCraneAuthWithToken(token)

	authConfig, err := authenticator.Authorization()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if authConfig.RegistryToken != token {
		t.Errorf("expected RegistryToken %q, got %q", token, authConfig.RegistryToken)
	}
}

func TestGetCraneAuthWithToken_StableAcrossCalls(t *testing.T) {
	token := "stable-token"
	authenticator := GetCraneAuthWithToken(token)

	// Call multiple times — should always return the same token
	for i := 0; i < 3; i++ {
		authConfig, err := authenticator.Authorization()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if authConfig.RegistryToken != token {
			t.Errorf("call %d: expected %q, got %q", i, token, authConfig.RegistryToken)
		}
	}
}

func TestGetCraneAuthWithToken_DifferentInstances(t *testing.T) {
	auth1 := GetCraneAuthWithToken("token-a")
	auth2 := GetCraneAuthWithToken("token-b")

	config1, _ := auth1.Authorization()
	config2, _ := auth2.Authorization()

	if config1.RegistryToken == config2.RegistryToken {
		t.Error("different authenticators should return different tokens")
	}
}
