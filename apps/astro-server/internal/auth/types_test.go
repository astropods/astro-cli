package auth

import (
	"testing"

	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

func TestUserFromWorkOS(t *testing.T) {
	wosUser := usermanagement.User{
		ID:                "user_123",
		Email:             "test@example.com",
		FirstName:         "Test",
		LastName:          "User",
		EmailVerified:     true,
		ProfilePictureURL: "https://example.com/pic.jpg",
		Metadata: map[string]string{
			"key": "value",
		},
		CreatedAt: "2024-01-15T10:00:00Z",
		UpdatedAt: "2024-01-15T11:00:00Z",
	}

	user := UserFromWorkOS(wosUser)

	if user.ID != wosUser.ID {
		t.Errorf("ID mismatch: got %s, want %s", user.ID, wosUser.ID)
	}
	if user.Email != wosUser.Email {
		t.Errorf("Email mismatch: got %s, want %s", user.Email, wosUser.Email)
	}
	if user.FirstName != wosUser.FirstName {
		t.Errorf("FirstName mismatch: got %s, want %s", user.FirstName, wosUser.FirstName)
	}
	if user.LastName != wosUser.LastName {
		t.Errorf("LastName mismatch: got %s, want %s", user.LastName, wosUser.LastName)
	}
	if user.EmailVerified != wosUser.EmailVerified {
		t.Errorf("EmailVerified mismatch: got %v, want %v", user.EmailVerified, wosUser.EmailVerified)
	}
	if user.ProfilePictureURL != wosUser.ProfilePictureURL {
		t.Errorf("ProfilePictureURL mismatch: got %s, want %s", user.ProfilePictureURL, wosUser.ProfilePictureURL)
	}
	if user.Metadata["key"] != "value" {
		t.Errorf("Metadata mismatch: got %v, want %v", user.Metadata, wosUser.Metadata)
	}
	if user.CreatedAt != wosUser.CreatedAt {
		t.Errorf("CreatedAt mismatch: got %s, want %s", user.CreatedAt, wosUser.CreatedAt)
	}
	if user.UpdatedAt != wosUser.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %s, want %s", user.UpdatedAt, wosUser.UpdatedAt)
	}
}

func TestUserFromWorkOS_EmptyFields(t *testing.T) {
	wosUser := usermanagement.User{
		ID:    "user_123",
		Email: "test@example.com",
		// All other fields empty
	}

	user := UserFromWorkOS(wosUser)

	if user.ID != wosUser.ID {
		t.Errorf("ID mismatch: got %s, want %s", user.ID, wosUser.ID)
	}
	if user.Email != wosUser.Email {
		t.Errorf("Email mismatch: got %s, want %s", user.Email, wosUser.Email)
	}
	if user.FirstName != "" {
		t.Errorf("expected empty FirstName, got %s", user.FirstName)
	}
	if user.LastName != "" {
		t.Errorf("expected empty LastName, got %s", user.LastName)
	}
	if user.EmailVerified != false {
		t.Errorf("expected EmailVerified false, got %v", user.EmailVerified)
	}
}

func TestContextKey(t *testing.T) {
	// Ensure context keys are distinct
	if UserContextKey == SessionContextKey {
		t.Error("UserContextKey and SessionContextKey should be different")
	}

	// Ensure they have expected values
	if string(UserContextKey) != "user" {
		t.Errorf("UserContextKey = %s, want 'user'", UserContextKey)
	}
	if string(SessionContextKey) != "session" {
		t.Errorf("SessionContextKey = %s, want 'session'", SessionContextKey)
	}
}
