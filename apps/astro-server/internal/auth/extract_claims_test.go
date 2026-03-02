package auth

import (
	"testing"
)

func TestExtractTokenClaims_RoleAndPermissions(t *testing.T) {
	token := createTestJWT(map[string]any{
		"sid":         "session_abc",
		"role":        "admin",
		"permissions": []string{"admin:view", "agents:deploy"},
	})

	claims := ExtractTokenClaims(token)

	if claims.SessionID != "session_abc" {
		t.Errorf("SessionID = %q, want %q", claims.SessionID, "session_abc")
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q, want %q", claims.Role, "admin")
	}
	if len(claims.Permissions) != 2 {
		t.Fatalf("Permissions length = %d, want 2", len(claims.Permissions))
	}
	if claims.Permissions[0] != "admin:view" {
		t.Errorf("Permissions[0] = %q, want %q", claims.Permissions[0], "admin:view")
	}
	if claims.Permissions[1] != "agents:deploy" {
		t.Errorf("Permissions[1] = %q, want %q", claims.Permissions[1], "agents:deploy")
	}
}

func TestExtractTokenClaims_NoRoleOrPermissions(t *testing.T) {
	token := createTestJWT(map[string]any{
		"sid": "session_xyz",
		"sub": "user_123",
	})

	claims := ExtractTokenClaims(token)

	if claims.SessionID != "session_xyz" {
		t.Errorf("SessionID = %q, want %q", claims.SessionID, "session_xyz")
	}
	if claims.Role != "" {
		t.Errorf("Role = %q, want empty string", claims.Role)
	}
	if claims.Permissions != nil {
		t.Errorf("Permissions = %v, want nil", claims.Permissions)
	}
}

func TestExtractTokenClaims_EmptyPermissions(t *testing.T) {
	token := createTestJWT(map[string]any{
		"sid":         "session_1",
		"role":        "member",
		"permissions": []string{},
	})

	claims := ExtractTokenClaims(token)

	if claims.Role != "member" {
		t.Errorf("Role = %q, want %q", claims.Role, "member")
	}
	if len(claims.Permissions) != 0 {
		t.Errorf("Permissions length = %d, want 0", len(claims.Permissions))
	}
}

func TestExtractTokenClaims_InvalidToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no dots", "not-a-jwt"},
		{"two parts", "header.payload"},
		{"invalid base64", "header.!!invalid!!.signature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := ExtractTokenClaims(tt.token)
			if claims.SessionID != "" {
				t.Errorf("SessionID = %q, want empty", claims.SessionID)
			}
			if claims.Role != "" {
				t.Errorf("Role = %q, want empty", claims.Role)
			}
			if claims.Permissions != nil {
				t.Errorf("Permissions = %v, want nil", claims.Permissions)
			}
		})
	}
}
