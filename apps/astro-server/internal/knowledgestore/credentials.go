package knowledgestore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

// GenerateCredentials returns a map of env-var key → plaintext value for the given provider.
// These are injected into the K8s Secret and used to initialize the database process.
func GenerateCredentials(provider string) (map[string]string, error) {
	switch provider {
	case "postgres":
		pass, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"POSTGRES_USER":     "astro",
			"POSTGRES_PASSWORD": pass,
			"POSTGRES_DB":       "astro",
		}, nil

	case "qdrant":
		key, err := randomHex(24)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"QDRANT__SERVICE__API_KEY": key,
		}, nil

	case "redis":
		pass, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"REDIS_PASSWORD": pass,
		}, nil

	case "neo4j":
		pass, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			// neo4j uses NEO4J_AUTH=username/password to set credentials on first boot
			"NEO4J_AUTH": "neo4j/" + pass,
		}, nil

	default:
		return map[string]string{}, nil
	}
}

// EncryptCredentials encrypts each key-value pair using the given encryptor and returns
// the results ready for DB storage.
func EncryptCredentials(enc *envelope.Encryptor, creds map[string]string) ([]Credential, error) {
	result := make([]Credential, 0, len(creds))
	for k, v := range creds {
		ciphertext, nonce, err := enc.Encrypt([]byte(v))
		if err != nil {
			return nil, fmt.Errorf("encrypt %s: %w", k, err)
		}
		result = append(result, Credential{
			Key:            k,
			ValueEncrypted: ciphertext,
			Nonce:          nonce,
		})
	}
	return result, nil
}

// DecryptCredentials decrypts stored credentials using the given KMS client and the
// store's encrypted data key. Returns a map of env-var key → plaintext value.
func DecryptCredentials(ctx context.Context, kmsClient envelope.KMSClient, encryptedDataKey []byte, creds []Credential) (map[string]string, error) {
	dec, err := envelope.NewDecryptor(ctx, kmsClient, encryptedDataKey)
	if err != nil {
		return nil, fmt.Errorf("create decryptor: %w", err)
	}

	result := make(map[string]string, len(creds))
	for _, c := range creds {
		plain, err := dec.Decrypt(c.ValueEncrypted, c.Nonce)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s: %w", c.Key, err)
		}
		result[c.Key] = string(plain)
	}
	return result, nil
}

// SecretReader reads plaintext credentials from an external source (e.g. a k8s Secret).
// Used as the fallback when KMS is not configured.
type SecretReader interface {
	ReadCredentials(ctx context.Context, storeID, namespace string) (map[string]string, error)
}

// ResolveCredentials returns plaintext credentials for a knowledge store.
// It tries KMS decryption first (when EncryptedDataKey is set), then falls
// back to reading from the k8s Secret via the SecretReader.
func ResolveCredentials(
	ctx context.Context,
	store *KnowledgeStore,
	dbCreds []Credential,
	kmsClient envelope.KMSClient,
	secretReader SecretReader,
	namespace string,
) (map[string]string, error) {
	// Path 1: KMS decryption — credentials stored encrypted in the DB.
	if len(store.EncryptedDataKey) > 0 && len(dbCreds) > 0 {
		if kmsClient == nil {
			return nil, fmt.Errorf("KMS client required but not available")
		}
		return DecryptCredentials(ctx, kmsClient, store.EncryptedDataKey, dbCreds)
	}

	// Path 2: no KMS — read directly from the k8s Secret.
	if secretReader != nil {
		return secretReader.ReadCredentials(ctx, store.ID, namespace)
	}

	return nil, fmt.Errorf("no credentials available (KMS not configured and no secret reader)")
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return hex.EncodeToString(b), nil
}
