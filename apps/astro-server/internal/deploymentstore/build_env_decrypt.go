package deploymentstore

import (
	"context"
	"fmt"
)

// RedactedSecretValue is the placeholder returned in place of a decrypted
// secret value when surfacing env to clients.
const RedactedSecretValue = "••••••••" //nolint:gosec // not a credential, it's a UI placeholder

// DecryptedEnvVar is one entry of deployment_build_env resolved for display:
// non-secret values are decrypted plaintext, secrets are replaced with
// RedactedSecretValue. Source and IsSecret are carried through from the row.
type DecryptedEnvVar struct {
	Name     string
	Value    string
	IsSecret bool
	Source   string
}

// LoadDecryptedBuildEnv reads deployment_build_env for the given deployment,
// decrypts non-secret values using the deployment's envelope key (when KMS is
// configured), redacts secrets, and returns the env grouped by role. The role
// keys match the values stored in the role column (deployment.RoleAgent,
// RoleMessaging, RoleCollector, KnowledgeRole(name), IngestionRole(name)).
//
// Returns nil with nil error when no rows exist — pre-cutover deployments
// have no build_env, so callers should render an empty env list rather than
// erroring.
//
// Decryption failures fall back to RedactedSecretValue rather than surfacing
// raw ciphertext or aborting; the env panel stays usable even if a single key
// can't be decrypted.
func (s *Store) LoadDecryptedBuildEnv(ctx context.Context, dep *Deployment, kmsKeyARN string) (map[string][]DecryptedEnvVar, error) {
	rows, err := s.GetBuildEnv(dep.ID)
	if err != nil {
		return nil, fmt.Errorf("get build env: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// dec may be nil when KMS is off; envelope.Decrypt is nil-safe and
	// passes ciphertext through when the row was stored plaintext (empty
	// nonce) or when no decryptor is available, so there's no branching
	// here on KMS state.
	dec, _ := NewDeploymentDecryptor(ctx, dep.EncryptedDataKey, kmsKeyARN)

	out := make(map[string][]DecryptedEnvVar, len(rows))
	for _, r := range rows {
		ev := DecryptedEnvVar{
			Name:     r.EnvName,
			IsSecret: r.IsSecret,
			Source:   r.Source,
		}
		if r.IsSecret {
			// Surface secrets as a redaction placeholder regardless of
			// whether we can decrypt — clients should never see the
			// plaintext on the API.
			ev.Value = RedactedSecretValue
		} else {
			pt, decErr := dec.Decrypt(r.ValueEncrypted, r.Nonce)
			if decErr != nil {
				// Decryption failure on a non-secret is unexpected
				// (non-secrets aren't encrypted in the first place); fall
				// back to redaction rather than surfacing raw bytes.
				ev.Value = RedactedSecretValue
			} else {
				ev.Value = string(pt)
			}
		}
		out[r.Role] = append(out[r.Role], ev)
	}
	return out, nil
}
