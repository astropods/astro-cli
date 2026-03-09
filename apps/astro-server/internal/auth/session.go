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

	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrInvalidSession   = errors.New("invalid session")
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrInvalidCookie    = errors.New("invalid cookie data")
)

// sessionKeySalt is a fixed salt for PBKDF2 key derivation.
var sessionKeySalt = []byte("astro-session-key-v1")

// pbkdf2Iterations is the number of iterations for PBKDF2.
// OWASP 2023 recommends 600,000 for PBKDF2-HMAC-SHA256.
const pbkdf2Iterations = 600000

// SessionManager handles session encryption and decryption
type SessionManager struct {
	cookiePassword []byte
	maxAge         time.Duration
}

// NewSessionManager creates a new session manager
func NewSessionManager(cookiePassword string, maxAge time.Duration) *SessionManager {
	// Derive a 32-byte key using PBKDF2-HMAC-SHA256
	key := pbkdf2.Key(
		[]byte(cookiePassword),
		sessionKeySalt,
		pbkdf2Iterations,
		32, // 32 bytes = 256 bits for AES-256
		sha256.New,
	)
	return &SessionManager{
		cookiePassword: key,
		maxAge:         maxAge,
	}
}

// SealSession encrypts and encodes session data for storage in a cookie
func (sm *SessionManager) SealSession(data *SessionData) (string, error) {
	// Serialize the session data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal session data: %w", ErrEncryptionFailed, err)
	}

	// Encrypt the JSON data
	encrypted, err := sm.encrypt(jsonData)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrEncryptionFailed, err)
	}

	// Use URL-safe base64 encoding for cookie storage (avoids issues with + and / chars)
	return base64.RawURLEncoding.EncodeToString(encrypted), nil
}

// UnsealSession decrypts and decodes session data from a cookie
func (sm *SessionManager) UnsealSession(sealedData string) (*SessionData, error) {
	if sealedData == "" {
		return nil, ErrInvalidCookie
	}

	// URL-safe base64 decode
	encrypted, err := base64.RawURLEncoding.DecodeString(sealedData)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 encoding", ErrDecryptionFailed)
	}

	// Decrypt the data
	jsonData, err := sm.decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
	}

	// Deserialize the JSON
	var data SessionData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal session data", ErrDecryptionFailed)
	}

	// Session expiry is NOT checked here so that callers (e.g. /me, /auth/refresh)
	// can read expired sessions and attempt a token refresh using the stored
	// refresh token. Callers that need to reject expired sessions should use
	// IsSessionValid after unsealing.

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
