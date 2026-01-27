package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	ErrInvalidSession   = errors.New("invalid session")
	ErrSessionExpired   = errors.New("session expired")
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrInvalidCookie    = errors.New("invalid cookie data")
)

// SessionManager handles session encryption and decryption
type SessionManager struct {
	cookiePassword []byte
	maxAge         time.Duration
}

// NewSessionManager creates a new session manager
func NewSessionManager(cookiePassword string, maxAge time.Duration) *SessionManager {
	// Derive a 32-byte key from the password using SHA-256
	hash := sha256.Sum256([]byte(cookiePassword))
	return &SessionManager{
		cookiePassword: hash[:],
		maxAge:         maxAge,
	}
}

// SealSession encrypts and encodes session data for storage in a cookie
func (sm *SessionManager) SealSession(data *SessionData) (string, error) {
	// Serialize the session data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal session data: %v", ErrEncryptionFailed, err)
	}

	// Encrypt the JSON data
	encrypted, err := sm.encrypt(jsonData)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	// Base64 encode for cookie storage
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// UnsealSession decrypts and decodes session data from a cookie
func (sm *SessionManager) UnsealSession(sealedData string) (*SessionData, error) {
	if sealedData == "" {
		return nil, ErrInvalidCookie
	}

	// Base64 decode
	encrypted, err := base64.StdEncoding.DecodeString(sealedData)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 encoding", ErrDecryptionFailed)
	}

	// Decrypt the data
	jsonData, err := sm.decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	// Deserialize the JSON
	var data SessionData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal session data", ErrDecryptionFailed)
	}

	// Validate session expiry
	if data.Session != nil && time.Now().After(data.Session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	return &data, nil
}

// encrypt encrypts data using AES-GCM
func (sm *SessionManager) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.cookiePassword)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Create a nonce
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt and prepend nonce
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-GCM
func (sm *SessionManager) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.cookiePassword)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	// Extract nonce and decrypt
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// IsSessionValid checks if a session is still valid
func (sm *SessionManager) IsSessionValid(session *Session) bool {
	if session == nil {
		return false
	}
	return time.Now().Before(session.ExpiresAt)
}

// CreateSession creates a new session from authentication response
func (sm *SessionManager) CreateSession(
	sessionID string,
	userID string,
	organizationID string,
	accessToken string,
	refreshToken string,
	expiresIn int,
) *Session {
	now := time.Now()
	expiresAt := now.Add(time.Duration(expiresIn) * time.Second)

	// Cap expiry at session max age
	maxExpiry := now.Add(sm.maxAge)
	if expiresAt.After(maxExpiry) {
		expiresAt = maxExpiry
	}

	return &Session{
		ID:             sessionID,
		UserID:         userID,
		OrganizationID: organizationID,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
	}
}
