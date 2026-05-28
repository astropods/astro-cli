package deploymentstore

import (
	"context"
	"encoding/base64"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

// IsInlineSecret reports whether v is a deployment-local inline secret (not a vault ref).
func IsInlineSecret(v Variable) bool {
	return v.Secret && v.Ref == "" && v.Value != ""
}

// InlineSecretNames returns user_var names for stored inline secrets.
func InlineSecretNames(stored []Variable) []string {
	names := make([]string, 0, len(stored))
	for _, v := range stored {
		if IsInlineSecret(v) {
			names = append(names, v.Name)
		}
	}
	return names
}

// PlaintextValue decodes a variable Value from GetDeploymentVariables.
// Secret payloads are base64-encoded ciphertext (or plaintext bytes when KMS is off).
func PlaintextValue(dec *envelope.Decryptor, v Variable) (string, bool) {
	raw, err := base64.StdEncoding.DecodeString(v.Value)
	if err != nil {
		return "", false
	}
	if len(v.Nonce) > 0 {
		if dec == nil {
			return "", false
		}
		plaintext, decErr := dec.Decrypt(raw, v.Nonce)
		if decErr != nil {
			return "", false
		}
		return string(plaintext), true
	}
	return string(raw), true
}

// NewDeploymentDecryptor returns a decryptor for deployment user variables, or nil.
func NewDeploymentDecryptor(ctx context.Context, encryptedDataKey []byte, kmsKeyARN string) (*envelope.Decryptor, error) {
	if len(encryptedDataKey) == 0 || kmsKeyARN == "" {
		return nil, nil
	}
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return envelope.NewDecryptor(ctx, kms.NewFromConfig(awsCfg), encryptedDataKey)
}
