// Package envelope implements AWS KMS envelope encryption for secret values.
// A single data key is generated per deployment via KMS GenerateDataKey,
// and individual values are encrypted locally with AES-256-GCM using per-value nonces.
package envelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// KMSClient is the subset of the KMS API we use.
type KMSClient interface {
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// Encryptor holds a KMS-generated data key for encrypting multiple values.
type Encryptor struct {
	gcm              cipher.AEAD
	EncryptedDataKey []byte // Store this in the DB alongside the deployment
	KMSKeyARN        string
}

// NewEncryptor calls KMS GenerateDataKey and returns an Encryptor ready to encrypt values.
func NewEncryptor(ctx context.Context, client KMSClient, keyARN string) (*Encryptor, error) {
	out, err := client.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(keyARN),
		KeySpec: types.DataKeySpecAes256,
	})
	if err != nil {
		return nil, fmt.Errorf("kms GenerateDataKey: %w", err)
	}

	gcm, err := newGCM(out.Plaintext)
	if err != nil {
		return nil, err
	}

	// Zero out plaintext key in the KMS response struct
	for i := range out.Plaintext {
		out.Plaintext[i] = 0
	}

	return &Encryptor{
		gcm:              gcm,
		EncryptedDataKey: out.CiphertextBlob,
		KMSKeyARN:        keyARN,
	}, nil
}

// Encrypt encrypts a plaintext value, returning (ciphertext, nonce).
//
// Nil-safe: callers may invoke Encrypt on a nil receiver to express
// "KMS is not configured / passthrough" without branching themselves.
// In that case the returned ciphertext is the plaintext bytes and the
// nonce is nil. Storing values that way is the local-dev convention
// (also used by deployment_variables for non-secret rows); the
// corresponding Decrypt call below restores them as-is.
func (e *Encryptor) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	if e == nil || e.gcm == nil {
		return plaintext, nil, nil
	}
	nonce = make([]byte, e.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext = e.gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
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

	// Zero out plaintext key
	for i := range out.Plaintext {
		out.Plaintext[i] = 0
	}

	return &Decryptor{gcm: gcm}, nil
}

// Decrypt decrypts a ciphertext value using its nonce.
//
// Empty-nonce values are returned as-is — that's the convention Encrypt
// uses for plaintext storage (non-secret rows, KMS-off mode). This
// removes the "is KMS configured?" check from every caller; they just
// invoke Decrypt with whatever pair the store gave them.
//
// A nil receiver is only valid when every input row is plaintext
// (nonce==0). Calling Decrypt(ct, non-empty-nonce) on a nil receiver
// returns an error rather than leaking ciphertext bytes as a "plaintext"
// value — that combination signals KMS misconfiguration on a row that
// was actually encrypted.
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

// DecryptDataKey returns the plaintext data key behind an encrypted data key.
// Pair it with NewEncryptorFromPlaintext to encrypt new values under a store's
// existing data key (so they decrypt alongside its other values) without
// re-keying. The caller is responsible for zeroing the returned slice after use.
func DecryptDataKey(ctx context.Context, client KMSClient, encryptedDataKey []byte) ([]byte, error) {
	out, err := client.Decrypt(ctx, &kms.DecryptInput{CiphertextBlob: encryptedDataKey})
	if err != nil {
		return nil, fmt.Errorf("kms Decrypt: %w", err)
	}
	return out.Plaintext, nil
}

// NewEncryptorFromPlaintext creates an Encryptor from an already-decrypted data key.
// Use this when you already have the plaintext key (e.g. from a KMS Decrypt call)
// and the corresponding encrypted data key stored in the DB.
// The caller is responsible for zeroing the plaintext key after this call.
func NewEncryptorFromPlaintext(plaintextKey, encryptedDataKey []byte, kmsKeyARN string) (*Encryptor, error) {
	gcm, err := newGCM(plaintextKey)
	if err != nil {
		return nil, err
	}
	return &Encryptor{
		gcm:              gcm,
		EncryptedDataKey: encryptedDataKey,
		KMSKeyARN:        kmsKeyARN,
	}, nil
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
