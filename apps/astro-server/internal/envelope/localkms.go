package envelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// localDevMasterKey is a compiled-in AES-256 key (exactly 32 bytes) used ONLY by
// LocalKMSClient to wrap per-row data keys when running against a local cluster
// without real KMS. It is published in the source, so it provides no
// confidentiality — callers MUST gate its use on local mode.
var localDevMasterKey = []byte("astro-local-dev-kms-master-key!!")

// LocalKMSClient is a dev-only KMSClient that emulates KMS envelope encryption
// without AWS. GenerateDataKey returns a fresh random data key wrapped under the
// static master key with AES-256-GCM; Decrypt unwraps it. This keeps the exact
// same at-rest shape (ciphertext + nonce + wrapped data key) as production so no
// other code needs to branch.
//
// NOT for production: the master key is in the public source. Use only when
// K8S_CLIENT_MODE=local and no real KMS_KEY_ARN is configured.
type LocalKMSClient struct{ aead cipher.AEAD }

// NewLocalKMSClient returns a LocalKMSClient backed by the compiled-in dev key.
func NewLocalKMSClient() (*LocalKMSClient, error) {
	block, err := aes.NewCipher(localDevMasterKey)
	if err != nil {
		return nil, fmt.Errorf("local kms: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("local kms: new gcm: %w", err)
	}
	return &LocalKMSClient{aead: aead}, nil
}

// GenerateDataKey returns a fresh 32-byte data key and its wrapped form. The
// caller (envelope.NewEncryptor) zeroes the returned Plaintext after use, so it
// is a standalone slice.
func (c *LocalKMSClient) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	dataKey := make([]byte, 32)
	if _, err := rand.Read(dataKey); err != nil {
		return nil, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// CiphertextBlob = nonce || sealed(dataKey).
	blob := c.aead.Seal(append([]byte(nil), nonce...), nonce, dataKey, nil)
	return &kms.GenerateDataKeyOutput{Plaintext: dataKey, CiphertextBlob: blob}, nil
}

// Decrypt unwraps a data key produced by GenerateDataKey.
func (c *LocalKMSClient) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	blob := params.CiphertextBlob
	ns := c.aead.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("local kms: ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("local kms: decrypt data key: %w", err)
	}
	return &kms.DecryptOutput{Plaintext: plain}, nil
}
