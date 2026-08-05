package knowledgestore

import (
	"context"
	"fmt"
	"sort"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

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

// ResolveCredentials returns plaintext credentials for a knowledge store by
// decrypting the rows held in the DB. Connected stores are credential-only: the
// platform never holds a plaintext copy anywhere else, so KMS is the sole path.
func ResolveCredentials(
	ctx context.Context,
	store *KnowledgeStore,
	dbCreds []Credential,
	kmsClient envelope.KMSClient,
) (map[string]string, error) {
	if len(store.EncryptedDataKey) == 0 || len(dbCreds) == 0 {
		return nil, fmt.Errorf("store %q has no stored credentials", store.Name)
	}
	if kmsClient == nil {
		return nil, fmt.Errorf("store %q requires KMS decryption but no KMS client is available", store.Name)
	}
	return DecryptCredentials(ctx, kmsClient, store.EncryptedDataKey, dbCreds)
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
