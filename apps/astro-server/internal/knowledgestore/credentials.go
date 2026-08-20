package knowledgestore

import (
	"context"
	"fmt"
	"sort"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

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

func DecryptCredentials(ctx context.Context, vault *envelope.Vault, encryptedDataKey []byte, creds []Credential) (map[string]string, error) {
	dec, err := vault.Decryptor(ctx, encryptedDataKey)
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

func ResolveCredentials(
	ctx context.Context,
	store *KnowledgeStore,
	dbCreds []Credential,
	vault *envelope.Vault,
) (map[string]string, error) {
	if len(store.EncryptedDataKey) == 0 || len(dbCreds) == 0 {
		return nil, fmt.Errorf("store %q has no stored credentials", store.Name)
	}
	return DecryptCredentials(ctx, vault, store.EncryptedDataKey, dbCreds)
}

const HostCredentialKey = "HOST"

func EncryptHostCredential(ctx context.Context, vault *envelope.Vault, store *KnowledgeStore, host string) (Credential, error) {
	enc, err := vault.EncryptorFor(ctx, store.EncryptedDataKey)
	if err != nil {
		return Credential{}, fmt.Errorf("build encryptor: %w", err)
	}
	ciphertext, nonce, err := enc.Encrypt([]byte(host))
	if err != nil {
		return Credential{}, fmt.Errorf("encrypt host: %w", err)
	}
	return Credential{Key: HostCredentialKey, ValueEncrypted: ciphertext, Nonce: nonce}, nil
}

func (s *Store) RewriteHostCredential(ctx context.Context, vault *envelope.Vault, store *KnowledgeStore, host string) error {
	return s.RewriteCredentials(ctx, vault, store, map[string]string{HostCredentialKey: host})
}

func EncryptCredentialsForStore(ctx context.Context, vault *envelope.Vault, store *KnowledgeStore, updates map[string]string) ([]Credential, error) {
	enc, err := vault.EncryptorFor(ctx, store.EncryptedDataKey)
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

func (s *Store) RewriteCredentials(ctx context.Context, vault *envelope.Vault, store *KnowledgeStore, updates map[string]string) error {
	if store == nil || len(store.EncryptedDataKey) == 0 || len(updates) == 0 {
		return nil
	}
	creds, err := EncryptCredentialsForStore(ctx, vault, store, updates)
	if err != nil {
		return err
	}
	return s.SaveCredentials(store.ID, creds)
}
