// Package envelope implements the decrypt half of AWS KMS envelope encryption,
// matching what astro-server's envelope package writes: a KMS-encrypted data
// key plus AES-256-GCM ciphertext + per-value nonce. astro-otel only ever
// decrypts (Langfuse secret keys), so the encrypt path is intentionally absent.
// Kept byte-compatible with apps/astro-server/internal/envelope.
package envelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSClient is the subset of the KMS API we use.
type KMSClient interface {
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// Decryptor holds a decrypted data key for decrypting multiple values.
type Decryptor struct {
	gcm cipher.AEAD
}

// NewDecryptor calls KMS Decrypt on the encrypted data key and returns a Decryptor.
func NewDecryptor(ctx context.Context, client KMSClient, encryptedDataKey []byte) (*Decryptor, error) {
	out, err := client.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encryptedDataKey,
	})
	if err != nil {
		return nil, fmt.Errorf("kms Decrypt: %w", err)
	}

	gcm, err := newGCM(out.Plaintext)
	if err != nil {
		return nil, err
	}

	// Zero out plaintext key.
	for i := range out.Plaintext {
		out.Plaintext[i] = 0
	}

	return &Decryptor{gcm: gcm}, nil
}

// Decrypt decrypts a ciphertext value using its nonce. Empty-nonce values are
// returned as-is — the convention astro-server uses for plaintext storage
// (non-secret rows, KMS-off dev mode).
func (d *Decryptor) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) == 0 {
		return ciphertext, nil
	}
	if d == nil || d.gcm == nil {
		return nil, fmt.Errorf("decryptor unavailable for ciphertext with nonce")
	}
	plaintext, err := d.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm decrypt: %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return gcm, nil
}
