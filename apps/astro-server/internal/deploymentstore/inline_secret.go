package deploymentstore

import (
	"context"
	"encoding/base64"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

func IsInlineSecret(v Variable) bool {
	return v.Secret && v.Ref == "" && v.Value != ""
}

func InlineSecretNames(stored []Variable) []string {
	names := make([]string, 0, len(stored))
	for _, v := range stored {
		if IsInlineSecret(v) {
			names = append(names, v.Name)
		}
	}
	return names
}

func PlaintextValue(dec *envelope.Decryptor, v Variable) (string, bool) {
	raw, err := base64.StdEncoding.DecodeString(v.Value)
	if err != nil {
		return "", false
	}
	plaintext, decErr := dec.Decrypt(raw, v.Nonce)
	if decErr != nil {
		return "", false
	}
	return string(plaintext), true
}

func NewDeploymentDecryptor(ctx context.Context, vault *envelope.Vault, encryptedDataKey []byte) (*envelope.Decryptor, error) {
	if len(encryptedDataKey) == 0 {
		return nil, nil
	}
	return vault.Decryptor(ctx, encryptedDataKey)
}
