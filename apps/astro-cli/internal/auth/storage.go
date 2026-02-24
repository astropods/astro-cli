package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	// KeyringService is the service name for keyring storage
	KeyringService = "astro-cli"

	// KeyringAccessTokenKey is the key for the access token in keyring
	KeyringAccessTokenKey = "access_token"

	// KeyringRefreshTokenKey is the key for the refresh token in keyring
	KeyringRefreshTokenKey = "refresh_token"
)

// Credentials represents stored authentication credentials
type Credentials struct {
	Profiles       map[string]*Profile `json:"profiles"`
	CurrentProfile string              `json:"current_profile"`
}

// Profile represents a single authentication profile
type Profile struct {
	AccessToken  string           `json:"access_token,omitempty"`  //nolint:gosec
	RefreshToken string           `json:"refresh_token,omitempty"` //nolint:gosec
	ExpiresAt    time.Time        `json:"expires_at,omitempty"`
	User         *StoredUser      `json:"user,omitempty"`
	Accounts     []StoredAccount  `json:"accounts,omitempty"`
}

// StoredUser represents user info stored with credentials
type StoredUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	AccountName string `json:"account_name,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
}

// StoredAccount represents an account stored with the profile
type StoredAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role,omitempty"`
}

// Storage handles secure credential storage
type Storage struct {
	binaryName string
	useKeyring bool
}

// NewStorage creates a new storage instance.
// binaryName controls the config directory (e.g. "ast" → ~/.ast, "ast-preview" → ~/.ast-preview).
func NewStorage(binaryName string) *Storage {
	useKeyring := isKeyringAvailable()
	return &Storage{binaryName: binaryName, useKeyring: useKeyring}
}

// isKeyringAvailable tests if the system keyring is accessible
func isKeyringAvailable() bool {
	testKey := "astro-cli-test"
	testValue := "test"

	// Try to set and delete a test value
	err := keyring.Set(KeyringService, testKey, testValue)
	if err != nil {
		return false
	}

	_ = keyring.Delete(KeyringService, testKey)
	return true
}

// LoadCredentials loads credentials from storage
func (s *Storage) LoadCredentials() (*Credentials, error) {
	path, err := CredentialsPath(s.binaryName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return &Credentials{
				Profiles:       make(map[string]*Profile),
				CurrentProfile: "default",
			}, nil
		}
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	if creds.Profiles == nil {
		creds.Profiles = make(map[string]*Profile)
	}
	if creds.CurrentProfile == "" {
		creds.CurrentProfile = "default"
	}

	// Load tokens from keyring if available
	if s.useKeyring {
		for name, profile := range creds.Profiles {
			if accessToken, err := keyring.Get(KeyringService, fmt.Sprintf("%s_%s", name, KeyringAccessTokenKey)); err == nil {
				profile.AccessToken = accessToken
			}
			if refreshToken, err := keyring.Get(KeyringService, fmt.Sprintf("%s_%s", name, KeyringRefreshTokenKey)); err == nil {
				profile.RefreshToken = refreshToken
			}
		}
	}

	return &creds, nil
}

// SaveCredentials saves credentials to storage
func (s *Storage) SaveCredentials(creds *Credentials) error {
	path, err := CredentialsPath(s.binaryName)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// If using keyring, store tokens there and remove from file
	credsToSave := *creds
	if s.useKeyring {
		credsToSave.Profiles = make(map[string]*Profile)
		for name, profile := range creds.Profiles {
			// Store tokens in keyring
			if profile.AccessToken != "" {
				if err := keyring.Set(KeyringService, fmt.Sprintf("%s_%s", name, KeyringAccessTokenKey), profile.AccessToken); err != nil {
					return fmt.Errorf("failed to store access token in keyring: %w", err)
				}
			}
			if profile.RefreshToken != "" {
				if err := keyring.Set(KeyringService, fmt.Sprintf("%s_%s", name, KeyringRefreshTokenKey), profile.RefreshToken); err != nil {
					return fmt.Errorf("failed to store refresh token in keyring: %w", err)
				}
			}

			// Copy profile without tokens for file storage
			profileCopy := *profile
			profileCopy.AccessToken = ""
			profileCopy.RefreshToken = ""
			credsToSave.Profiles[name] = &profileCopy
		}
	} else {
		credsToSave.Profiles = creds.Profiles
	}

	data, err := json.MarshalIndent(credsToSave, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// GetCurrentProfile returns the current profile
func (s *Storage) GetCurrentProfile() (*Profile, error) {
	creds, err := s.LoadCredentials()
	if err != nil {
		return nil, err
	}

	profile, ok := creds.Profiles[creds.CurrentProfile]
	if !ok {
		return nil, errors.New("no current profile found")
	}

	return profile, nil
}

// SaveProfile saves a profile to storage
func (s *Storage) SaveProfile(name string, profile *Profile) error {
	creds, err := s.LoadCredentials()
	if err != nil {
		return err
	}

	creds.Profiles[name] = profile
	return s.SaveCredentials(creds)
}

// DeleteProfile deletes a profile from storage
func (s *Storage) DeleteProfile(name string) error {
	creds, err := s.LoadCredentials()
	if err != nil {
		return err
	}

	// Delete tokens from keyring if using it
	if s.useKeyring {
		_ = keyring.Delete(KeyringService, fmt.Sprintf("%s_%s", name, KeyringAccessTokenKey))
		_ = keyring.Delete(KeyringService, fmt.Sprintf("%s_%s", name, KeyringRefreshTokenKey))
	}

	delete(creds.Profiles, name)
	return s.SaveCredentials(creds)
}

// DeleteAllProfiles deletes all profiles from storage
func (s *Storage) DeleteAllProfiles() error {
	creds, err := s.LoadCredentials()
	if err != nil {
		return err
	}

	// Delete tokens from keyring if using it
	if s.useKeyring {
		for name := range creds.Profiles {
			_ = keyring.Delete(KeyringService, fmt.Sprintf("%s_%s", name, KeyringAccessTokenKey))
			_ = keyring.Delete(KeyringService, fmt.Sprintf("%s_%s", name, KeyringRefreshTokenKey))
		}
	}

	creds.Profiles = make(map[string]*Profile)

	// Also delete the credentials file
	path, err := CredentialsPath(s.binaryName)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// SetCurrentProfile sets the current profile
func (s *Storage) SetCurrentProfile(name string) error {
	creds, err := s.LoadCredentials()
	if err != nil {
		return err
	}

	if _, ok := creds.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	creds.CurrentProfile = name
	return s.SaveCredentials(creds)
}

// HasValidCredentials checks if there are valid credentials stored
func (s *Storage) HasValidCredentials() bool {
	profile, err := s.GetCurrentProfile()
	if err != nil {
		return false
	}

	return profile.AccessToken != "" && time.Now().Before(profile.ExpiresAt)
}
