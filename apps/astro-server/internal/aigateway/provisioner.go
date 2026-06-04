package aigateway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

// Provisioner mints, stores, and revokes per-deployment virtual keys
// against the AI Gateway (LiteLLM). KMS client + key ARN are passed in at
// method-call time (same shape as langfuse.Provisioner) so the deployer can
// supply a lazily-resolved KMS handle via its kmsClient() helper.
//
// Lifecycle: minted at first deploy, decrypted-and-returned on retries and
// redeploys, revoked on undeploy. No rotation today — a future
// deployment-template API will trigger explicit rotation.
type Provisioner struct {
	client *Client
}

// NewProvisioner constructs a Provisioner.
func NewProvisioner(client *Client) *Provisioner {
	return &Provisioner{client: client}
}

// Client returns the underlying LiteLLM client (used for the public base URL
// the deployer writes into the tenant Secret).
func (p *Provisioner) Client() *Client { return p.client }

// DeploymentKeyParams collects the inputs to EnsureDeploymentKey. Grouped
// to keep the signature readable.
type DeploymentKeyParams struct {
	AccountID    string
	DeploymentID string
	ClusterID    string
	AgentName    string
	AgentVersion string
}

// EnsureDeploymentKey returns the (plaintext API key, public base URL) for
// the deployment. Idempotent: if a row already exists, the stored ciphertext
// is decrypted and returned. Otherwise a new key is minted upstream via
// /key/generate, KMS-encrypted, and persisted.
//
// UserID and TeamID stay pinned to accountID — OpenMeter chargeback keys off
// metadata.user_api_key_user_id, which must remain stable per account or the
// ledger silently corrupts. Deployment scope lives in metadata.tags instead.
//
// No rotation: the key minted here lives for the lifetime of the deployment.
// Redeploys reuse it; Teardown revokes it. A future deployment-template API
// will trigger explicit rotation.
func (p *Provisioner) EnsureDeploymentKey(
	ctx context.Context,
	store *Store,
	kmsKeyARN string,
	kmsClient envelope.KMSClient,
	params DeploymentKeyParams,
) (apiKey, baseURL string, err error) {
	if params.AccountID == "" || params.DeploymentID == "" {
		return "", "", fmt.Errorf("EnsureDeploymentKey: AccountID and DeploymentID are required")
	}

	existing, err := store.Get(params.DeploymentID)
	if err != nil {
		return "", "", fmt.Errorf("check existing: %w", err)
	}
	if existing != nil {
		pk, err := decryptAPIKey(ctx, kmsClient, existing.EncryptedAPIKey, existing.EncryptedDataKey, existing.Nonce)
		if err != nil {
			return "", "", fmt.Errorf("decrypt existing key: %w", err)
		}
		return pk, p.client.URL(), nil
	}

	resp, err := p.client.GenerateKey(ctx, KeyRequest{
		UserID:   params.AccountID,
		TeamID:   params.AccountID,
		Metadata: deploymentKeyMetadata(params),
	})
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}

	ciphertext, encDataKey, nonce, err := encryptAPIKey(ctx, kmsClient, kmsKeyARN, resp.Key)
	if err != nil {
		if delErr := p.client.DeleteKey(ctx, resp.KeyID); delErr != nil {
			return "", "", errors.Join(fmt.Errorf("encrypt key (orphan upstream %s): %w", resp.KeyID, err), fmt.Errorf("revoke also failed: %w", delErr))
		}
		return "", "", fmt.Errorf("encrypt key: %w", err)
	}

	row := &DeploymentAIGateway{
		DeploymentID:     params.DeploymentID,
		AccountID:        params.AccountID,
		KeyID:            resp.KeyID,
		EncryptedAPIKey:  ciphertext,
		EncryptedDataKey: encDataKey,
		Nonce:            nonce,
		IssuedAt:         time.Now().UTC(),
	}
	if err := store.Save(row); err != nil {
		if delErr := p.client.DeleteKey(ctx, resp.KeyID); delErr != nil {
			return "", "", errors.Join(fmt.Errorf("save key (orphan upstream %s): %w", resp.KeyID, err), fmt.Errorf("revoke also failed: %w", delErr))
		}
		return "", "", fmt.Errorf("save key: %w", err)
	}
	return resp.Key, p.client.URL(), nil
}

// deploymentKeyMetadata builds the LiteLLM key metadata. Tags identify the
// deployment in the LiteLLM admin view and are forwarded onto Langfuse
// traces if (and only if) LiteLLM is configured with a Langfuse callback
// elsewhere; we do not embed per-tenant Langfuse credentials here.
func deploymentKeyMetadata(p DeploymentKeyParams) map[string]any {
	tags := []string{"deployment:" + p.DeploymentID}
	if p.AgentName != "" {
		tags = append(tags, "agent:"+p.AgentName)
	}
	if p.AgentVersion != "" {
		tags = append(tags, "version:"+p.AgentVersion)
	}
	return map[string]any{
		"cluster_id": p.ClusterID,
		"tags":       tags,
	}
}

// RevokeDeploymentKey deletes the key upstream and removes the local row.
// Called on undeploy. Idempotent.
func (p *Provisioner) RevokeDeploymentKey(ctx context.Context, store *Store, deploymentID string) error {
	row, err := store.Get(deploymentID)
	if err != nil {
		return fmt.Errorf("get for revoke: %w", err)
	}
	if row == nil {
		return nil
	}
	if err := p.client.DeleteKey(ctx, row.KeyID); err != nil {
		return fmt.Errorf("delete key %s: %w", row.KeyID, err)
	}
	if err := store.Delete(deploymentID); err != nil {
		return fmt.Errorf("delete row: %w", err)
	}
	return nil
}

// RevokeAccount sweeps every deployment_ai_gateway row under the account,
// revokes each key upstream, and deletes the rows. Called on account purge.
// Best-effort — if a single deployment's revoke fails, log and continue;
// the row would otherwise block account hard-delete unnecessarily.
func (p *Provisioner) RevokeAccount(ctx context.Context, store *Store, accountID string) error {
	depIDs, err := store.ListByAccount(accountID)
	if err != nil {
		return fmt.Errorf("list deployment keys for account: %w", err)
	}
	var errs []error
	for _, depID := range depIDs {
		if err := p.RevokeDeploymentKey(ctx, store, depID); err != nil {
			errs = append(errs, fmt.Errorf("revoke deployment %s: %w", depID, err))
		}
	}
	return errors.Join(errs...)
}

// RevokeAccountDevKeys deletes every account_ai_gateway_dev_keys row's
// upstream LiteLLM key and clears the rows locally. Called on account
// purge alongside RevokeAccount — the FK cascade on the eventual account
// hard-delete would clear the rows, but LiteLLM has no FK back to us, so
// without the explicit /key/delete the upstream keys would linger until
// their 8h TTL with a defunct user/team binding.
func (p *Provisioner) RevokeAccountDevKeys(ctx context.Context, devStore *DevStore, accountID string) error {
	keyIDs, err := devStore.ListKeyIDsByAccount(accountID)
	if err != nil {
		return fmt.Errorf("list dev keys for account: %w", err)
	}
	var errs []error
	for _, keyID := range keyIDs {
		if err := p.client.DeleteKey(ctx, keyID); err != nil {
			errs = append(errs, fmt.Errorf("delete dev key %s: %w", keyID, err))
		}
	}
	if err := devStore.DeleteByAccount(accountID); err != nil {
		errs = append(errs, fmt.Errorf("delete dev key rows: %w", err))
	}
	return errors.Join(errs...)
}

// DevKeyDuration is the LiteLLM-side TTL set when minting an `astro dev`
// key. Reused across CLI invocations while non-expired (per DevKey.IsUsable)
// so a developer running `ast dev` repeatedly in the same working day
// gets the same key each time.
const DevKeyDuration = 8 * time.Hour

// EnsureDevKey returns a usable dev key for (accountID, actorUserID),
// minting a fresh one only when the previous has expired (or doesn't
// exist yet). The LiteLLM-side metadata records actorUserID for audit.
//
// Independent of EnsureDeploymentKey — an account can run `ast dev` before
// ever having deployed; the per-deployment key is only minted at deploy.
//
// Returns (plaintext apiKey, baseURL, expiresAt).
func (p *Provisioner) EnsureDevKey(
	ctx context.Context,
	devStore *DevStore,
	kmsKeyARN string,
	kmsClient envelope.KMSClient,
	accountID, actorUserID string,
) (apiKey, baseURL string, expiresAt time.Time, err error) {
	existing, err := devStore.Get(accountID, actorUserID)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("get dev key row: %w", err)
	}
	if existing.IsUsable() {
		plaintext, err := decryptAPIKey(ctx, kmsClient, existing.EncryptedAPIKey, existing.EncryptedDataKey, existing.Nonce)
		if err != nil {
			return "", "", time.Time{}, fmt.Errorf("decrypt existing dev key: %w", err)
		}
		return plaintext, p.client.URL(), existing.ExpiresAt, nil
	}

	// Mint fresh.
	resp, err := p.client.GenerateKey(ctx, KeyRequest{
		UserID: accountID,
		TeamID: accountID,
		Metadata: map[string]any{
			// kind separates dev keys from deploy keys in LiteLLM /key/list.
			// actor_user_id is the only user identifier we store (no PII).
			"kind":          "dev",
			"actor_user_id": actorUserID,
		},
		Duration: DevKeyDuration.String(),
	})
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate dev key: %w", err)
	}

	ciphertext, encDataKey, nonce, err := encryptAPIKey(ctx, kmsClient, kmsKeyARN, resp.Key)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("encrypt dev key: %w", err)
	}

	expires := time.Now().UTC().Add(DevKeyDuration)
	prev, err := devStore.Upsert(&DevKey{
		AccountID:        accountID,
		UserID:           actorUserID,
		KeyID:            resp.KeyID,
		EncryptedAPIKey:  ciphertext,
		EncryptedDataKey: encDataKey,
		Nonce:            nonce,
		ExpiresAt:        expires,
	})
	if err != nil {
		if delErr := p.client.DeleteKey(ctx, resp.KeyID); delErr != nil {
			return "", "", time.Time{}, errors.Join(
				fmt.Errorf("upsert dev key (orphan upstream key %s): %w", resp.KeyID, err),
				fmt.Errorf("revoke also failed: %w", delErr),
			)
		}
		return "", "", time.Time{}, fmt.Errorf("upsert dev key: %w", err)
	}
	if prev != "" {
		// Best-effort revoke of the predecessor. Non-fatal — its own TTL
		// would expire it anyway; explicit cleanup just keeps LiteLLM's
		// DB tidy.
		_ = p.client.DeleteKey(ctx, prev)
	}

	return resp.Key, p.client.URL(), expires, nil
}

func encryptAPIKey(ctx context.Context, kmsClient envelope.KMSClient, kmsKeyARN, plaintext string) (ciphertext string, encDataKey, nonce []byte, err error) {
	if kmsKeyARN == "" || kmsClient == nil {
		// No KMS — store plaintext (dev/test). Matches Langfuse's fallback.
		return plaintext, nil, nil, nil
	}
	enc, err := envelope.NewEncryptor(ctx, kmsClient, kmsKeyARN)
	if err != nil {
		return "", nil, nil, fmt.Errorf("new encryptor: %w", err)
	}
	ct, n, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return "", nil, nil, fmt.Errorf("encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(ct), enc.EncryptedDataKey, n, nil
}

func decryptAPIKey(ctx context.Context, kmsClient envelope.KMSClient, ciphertext string, encDataKey, nonce []byte) (string, error) {
	if len(encDataKey) == 0 || len(nonce) == 0 {
		return ciphertext, nil
	}
	if kmsClient == nil {
		return "", fmt.Errorf("KMS client required to decrypt AI gateway key")
	}
	dec, err := envelope.NewDecryptor(ctx, kmsClient, encDataKey)
	if err != nil {
		return "", fmt.Errorf("new decryptor: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	pt, err := dec.Decrypt(ct, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}
