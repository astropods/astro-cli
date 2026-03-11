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
func (e *Encryptor) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
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
func (d *Decryptor) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
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
