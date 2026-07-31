package knowledgestore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"

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
// Three resolution paths, tried in order:
//  1. KMS decryption — credentials stored encrypted in the DB (production with KMS).
//  2. K8s Secret fallback — read from the store's credential Secret (local/no-KMS managed stores).
//  3. Error — no credentials available (external stores without KMS have no fallback).
//
// When KMS is required (EncryptedDataKey + dbCreds present) but no KMS client is
// available, managed stores fall through to the k8s Secret which holds plaintext
// credentials provisioned alongside the StatefulSet. External stores have no
// such Secret and surface the KMS error directly.
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
		if kmsClient != nil {
			return DecryptCredentials(ctx, kmsClient, store.EncryptedDataKey, dbCreds)
		}
		// No KMS available. External stores have nothing else to read from.
		if store.Mode == ModeExternal || secretReader == nil {
			return nil, fmt.Errorf("store %q requires KMS decryption but no KMS client is available", store.Name)
		}
		// Managed store: fall through to k8s Secret (path 2).
	}

	// Path 2: k8s Secret fallback — for managed stores without KMS (local dev).
	// External stores have no k8s Secret; this path will fail for them.
	if secretReader != nil {
		creds, err := secretReader.ReadCredentials(ctx, store.ID, namespace)
		if err != nil {
			if store.Mode == ModeExternal {
				return nil, fmt.Errorf("external store %q: credentials require KMS (not configured when store was created)", store.Name)
			}
			return nil, fmt.Errorf("store %q: %w", store.Name, err)
		}
		return creds, nil
	}

	return nil, fmt.Errorf("store %q: no credentials available (no KMS and no k8s Secret reader configured)", store.Name)
}

// HostCredentialKey is the credential key holding an external store's
// connection host.
const HostCredentialKey = "HOST"

// EncryptHostCredential builds the HOST credential row for a store, encrypting
// host under the store's existing data key (no re-key) so it decrypts alongside
// the store's other credentials.
func EncryptHostCredential(ctx context.Context, kmsClient envelope.KMSClient, store *KnowledgeStore, host string) (Credential, error) {
	plaintextKey, err := envelope.DecryptDataKey(ctx, kmsClient, store.EncryptedDataKey)
	if err != nil {
		return Credential{}, fmt.Errorf("decrypt data key: %w", err)
	}
	defer func() {
		for i := range plaintextKey {
			plaintextKey[i] = 0
		}
	}()

	enc, err := envelope.NewEncryptorFromPlaintext(plaintextKey, store.EncryptedDataKey, "")
	if err != nil {
		return Credential{}, fmt.Errorf("build encryptor: %w", err)
	}
	ciphertext, nonce, err := enc.Encrypt([]byte(host))
	if err != nil {
		return Credential{}, fmt.Errorf("encrypt host: %w", err)
	}
	return Credential{Key: HostCredentialKey, ValueEncrypted: ciphertext, Nonce: nonce}, nil
}

// RewriteHostCredential re-encrypts a store's HOST credential to host and
// upserts only that row, leaving the store's other credentials untouched.
// It is a no-op for a store with no encrypted data key (KMS off — such stores
// have no persisted external credentials to update).
func (s *Store) RewriteHostCredential(ctx context.Context, kmsClient envelope.KMSClient, store *KnowledgeStore, host string) error {
	return s.RewriteCredentials(ctx, kmsClient, store, map[string]string{HostCredentialKey: host})
}

// EncryptCredentialsForStore encrypts each value in updates under the store's
// existing data key (no re-key) so the rows decrypt alongside the store's other
// credentials. Keys are processed in sorted order for deterministic output.
func EncryptCredentialsForStore(ctx context.Context, kmsClient envelope.KMSClient, store *KnowledgeStore, updates map[string]string) ([]Credential, error) {
	plaintextKey, err := envelope.DecryptDataKey(ctx, kmsClient, store.EncryptedDataKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt data key: %w", err)
	}
	defer func() {
		for i := range plaintextKey {
			plaintextKey[i] = 0
		}
	}()
	enc, err := envelope.NewEncryptorFromPlaintext(plaintextKey, store.EncryptedDataKey, "")
	if err != nil {
		return nil, fmt.Errorf("build encryptor: %w", err)
	}
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	creds := make([]Credential, 0, len(updates))
	for _, k := range keys {
		ciphertext, nonce, err := enc.Encrypt([]byte(updates[k]))
		if err != nil {
			return nil, fmt.Errorf("encrypt %s: %w", k, err)
		}
		creds = append(creds, Credential{Key: k, ValueEncrypted: ciphertext, Nonce: nonce})
	}
	return creds, nil
}

// RewriteCredentials re-encrypts the given updates under the store's existing
// data key and upserts only those rows, leaving other credentials untouched.
// It is a no-op for a store with no encrypted data key (KMS off — such stores
// have no persisted external credentials to update) or an empty update.
func (s *Store) RewriteCredentials(ctx context.Context, kmsClient envelope.KMSClient, store *KnowledgeStore, updates map[string]string) error {
	if store == nil || len(store.EncryptedDataKey) == 0 || len(updates) == 0 {
		return nil
	}
	creds, err := EncryptCredentialsForStore(ctx, kmsClient, store, updates)
	if err != nil {
		return err
	}
	return s.SaveCredentials(store.ID, creds)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return hex.EncodeToString(b), nil
}
