package auth

import (
	"errors"
	"testing"
	"time"
)

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	if sm == nil {
		t.Fatal("expected non-nil SessionManager")
	}
	if len(sm.cookiePassword) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(sm.cookiePassword))
	}
	if sm.maxAge != 24*time.Hour {
		t.Errorf("expected maxAge of 24h, got %v", sm.maxAge)
	}
}

func TestSealAndUnsealSession(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	originalData := &SessionData{
		Session: &Session{
			ID:             "session_123",
			UserID:         "user_456",
			OrganizationID: "org_789",
			AccessToken:    "access_token_abc",
			RefreshToken:   "refresh_token_xyz",
			ExpiresAt:      time.Now().Add(1 * time.Hour),
			CreatedAt:      time.Now(),
		},
		User: &User{
			ID:            "user_456",
			Email:         "test@example.com",
			FirstName:     "Test",
			LastName:      "User",
			EmailVerified: true,
		},
	}

	// Seal the session
	sealed, err := sm.SealSession(originalData)
	if err != nil {
		t.Fatalf("SealSession failed: %v", err)
	}
	if sealed == "" {
		t.Fatal("expected non-empty sealed data")
	}

	// Unseal the session
	unsealed, err := sm.UnsealSession(sealed)
	if err != nil {
		t.Fatalf("UnsealSession failed: %v", err)
	}

	// Verify the data matches
	if unsealed.Session.ID != originalData.Session.ID {
		t.Errorf("session ID mismatch: got %s, want %s", unsealed.Session.ID, originalData.Session.ID)
	}
	if unsealed.Session.UserID != originalData.Session.UserID {
		t.Errorf("user ID mismatch: got %s, want %s", unsealed.Session.UserID, originalData.Session.UserID)
	}
	if unsealed.Session.OrganizationID != originalData.Session.OrganizationID {
		t.Errorf("org ID mismatch: got %s, want %s", unsealed.Session.OrganizationID, originalData.Session.OrganizationID)
	}
	if unsealed.User.Email != originalData.User.Email {
		t.Errorf("email mismatch: got %s, want %s", unsealed.User.Email, originalData.User.Email)
	}
}

func TestSealAndUnsealSession_WithPermissions(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	originalData := &SessionData{
		Session: &Session{
			ID:          "session_perm",
			UserID:      "user_perm",
			Role:        "admin",
			Permissions: []string{"admin:view", "deployments:write"},
			AccessToken: "token",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
		},
		User: &User{
			ID:    "user_perm",
			Email: "admin@example.com",
		},
	}

	sealed, err := sm.SealSession(originalData)
	if err != nil {
		t.Fatalf("SealSession failed: %v", err)
	}

	unsealed, err := sm.UnsealSession(sealed)
	if err != nil {
		t.Fatalf("UnsealSession failed: %v", err)
	}

	if unsealed.Session.Role != "admin" {
		t.Errorf("Role = %q, want %q", unsealed.Session.Role, "admin")
	}
	if len(unsealed.Session.Permissions) != 2 {
		t.Fatalf("Permissions length = %d, want 2", len(unsealed.Session.Permissions))
	}
	if unsealed.Session.Permissions[0] != "admin:view" {
		t.Errorf("Permissions[0] = %q, want %q", unsealed.Session.Permissions[0], "admin:view")
	}
	if unsealed.Session.Permissions[1] != "deployments:write" {
		t.Errorf("Permissions[1] = %q, want %q", unsealed.Session.Permissions[1], "deployments:write")
	}
}

func TestUnsealSession_EmptyData(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	_, err := sm.UnsealSession("")
	if !errors.Is(err, ErrInvalidCookie) {
		t.Errorf("expected ErrInvalidCookie, got %v", err)
	}
}

func TestUnsealSession_InvalidBase64(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	_, err := sm.UnsealSession("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestUnsealSession_WrongPassword(t *testing.T) {
	sm1 := NewSessionManager("password-one-that-is-32-chars!!", 24*time.Hour)
	sm2 := NewSessionManager("password-two-that-is-32-chars!!", 24*time.Hour)

	data := &SessionData{
		Session: &Session{
			ID:        "session_123",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	sealed, err := sm1.SealSession(data)
	if err != nil {
		t.Fatalf("SealSession failed: %v", err)
	}

	// Try to unseal with different password
	_, err = sm2.UnsealSession(sealed)
	if err == nil {
		t.Error("expected error when unsealing with wrong password")
	}
}

func TestUnsealSession_ExpiredSession(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	data := &SessionData{
		Session: &Session{
			ID:        "session_123",
			ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
		},
	}

	sealed, err := sm.SealSession(data)
	if err != nil {
		t.Fatalf("SealSession failed: %v", err)
	}

	// UnsealSession should succeed for expired sessions so that callers
	// (e.g. /me, /auth/refresh) can read the refresh token and attempt
	// a transparent re-authentication. Callers use IsSessionValid to
	// check expiry where needed.
	unsealed, err := sm.UnsealSession(sealed)
	if err != nil {
		t.Fatalf("UnsealSession should succeed for expired sessions, got %v", err)
	}
	if unsealed.Session.ID != "session_123" {
		t.Errorf("session ID mismatch: got %s, want session_123", unsealed.Session.ID)
	}
	if sm.IsSessionValid(unsealed.Session) {
		t.Error("expected IsSessionValid to return false for expired session")
	}
}

func TestIsSessionValid(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	tests := []struct {
		name    string
		session *Session
		want    bool
	}{
		{
			name:    "nil session",
			session: nil,
			want:    false,
		},
		{
			name: "valid session",
			session: &Session{
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			want: true,
		},
		{
			name: "expired session",
			session: &Session{
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			want: false,
		},
		{
			name: "session expiring now",
			session: &Session{
				ExpiresAt: time.Now().Add(-1 * time.Second),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.IsSessionValid(tt.session)
			if got != tt.want {
				t.Errorf("IsSessionValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateSession(t *testing.T) {
	maxAge := 1 * time.Hour
	sm := NewSessionManager("test-password-that-is-32-chars!!", maxAge)

	session := sm.CreateSession(
		"session_123",
		"user_456",
		"org_789",
		"access_token",
		"refresh_token",
		3600, // 1 hour in seconds
	)

	if session.ID != "session_123" {
		t.Errorf("expected session ID 'session_123', got %s", session.ID)
	}
	if session.UserID != "user_456" {
		t.Errorf("expected user ID 'user_456', got %s", session.UserID)
	}
	if session.OrganizationID != "org_789" {
		t.Errorf("expected org ID 'org_789', got %s", session.OrganizationID)
	}
	if session.AccessToken != "access_token" {
		t.Errorf("expected access token 'access_token', got %s", session.AccessToken)
	}
	if session.RefreshToken != "refresh_token" {
		t.Errorf("expected refresh token 'refresh_token', got %s", session.RefreshToken)
	}

	// Verify expiry is within expected range
	expectedExpiry := time.Now().Add(maxAge)
	if session.ExpiresAt.After(expectedExpiry.Add(1 * time.Second)) {
		t.Errorf("session expiry %v is after max allowed %v", session.ExpiresAt, expectedExpiry)
	}
}

func TestCreateSession_CapsAtMaxAge(t *testing.T) {
	maxAge := 30 * time.Minute
	sm := NewSessionManager("test-password-that-is-32-chars!!", maxAge)

	// Request session with 2 hour expiry, but max age is 30 minutes
	session := sm.CreateSession(
		"session_123",
		"user_456",
		"",
		"access_token",
		"refresh_token",
		7200, // 2 hours in seconds
	)

	// Expiry should be capped at maxAge
	maxExpected := time.Now().Add(maxAge).Add(1 * time.Second)
	if session.ExpiresAt.After(maxExpected) {
		t.Errorf("session expiry %v exceeds max age cap %v", session.ExpiresAt, maxExpected)
	}
}

func TestEncryptDecrypt_Deterministic(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	plaintext := []byte("hello world")

	// Encrypt twice
	cipher1, err := sm.encrypt(plaintext)
	if err != nil {
		t.Fatalf("first encrypt failed: %v", err)
	}

	cipher2, err := sm.encrypt(plaintext)
	if err != nil {
		t.Fatalf("second encrypt failed: %v", err)
	}

	// Ciphertexts should be different (due to random nonce)
	if string(cipher1) == string(cipher2) {
		t.Error("expected different ciphertexts due to random nonce")
	}

	// But both should decrypt to the same plaintext
	decrypted1, err := sm.decrypt(cipher1)
	if err != nil {
		t.Fatalf("first decrypt failed: %v", err)
	}

	decrypted2, err := sm.decrypt(cipher2)
	if err != nil {
		t.Fatalf("second decrypt failed: %v", err)
	}

	if string(decrypted1) != string(plaintext) {
		t.Errorf("decrypted1 mismatch: got %s, want %s", decrypted1, plaintext)
	}
	if string(decrypted2) != string(plaintext) {
		t.Errorf("decrypted2 mismatch: got %s, want %s", decrypted2, plaintext)
	}
}

func TestDecrypt_TamperedData(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	plaintext := []byte("hello world")
	ciphertext, err := sm.encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Tamper with the ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err = sm.decrypt(ciphertext)
	if err == nil {
		t.Error("expected error when decrypting tampered data")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	sm := NewSessionManager("test-password-that-is-32-chars!!", 24*time.Hour)

	// AES-GCM nonce is 12 bytes, so anything shorter should fail
	shortData := []byte("short")

	_, err := sm.decrypt(shortData)
	if err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}
