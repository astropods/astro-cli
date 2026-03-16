//go:build integration

package e2e

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
)

// minimalSlackSpec returns a minimal deployment spec with slack adapter enabled
// and two secret variables (SLACK_BOT_TOKEN, SLACK_APP_TOKEN) plus a non-secret.
func minimalSlackSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Name: "rehydrate-test", Build: "abc123", Registry: "test.ecr/repo",
		},
		Agent: spec.DeploymentAgent{
			Image:     "test.ecr/repo/agent:abc123",
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080, Protocol: "http"}},
			Replicas:  1,
		},
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"slack"},
			Image:    "test.ecr/repo/messaging:latest",
			Endpoints: map[string]spec.Endpoint{
				"grpc": {Port: 9090, Protocol: "grpc"},
			},
		},
		Variables: map[string]spec.Variable{
			"SLACK_BOT_TOKEN": {
				Value: "xoxb-test-token-value", Secret: true, Optional: false,
				Targets: []string{"interface.slack"},
			},
			"SLACK_APP_TOKEN": {
				Value: "xapp-test-token-value", Secret: true, Optional: false,
				Targets: []string{"interface.slack"},
			},
			"LOG_LEVEL": {
				Value: "debug", Secret: false, Optional: true,
				Targets: []string{"agent"},
			},
		},
	}
}

// saveDeploymentWithSecrets saves a deployment spec with secret values stored
// in deployment_variables. The revision spec has secrets stripped (as in prod).
// If enc is non-nil, secret values are encrypted before storage.
func saveDeploymentWithSecrets(
	t *testing.T, db *sql.DB, store *ds.Store,
	full *spec.AstroDeploymentSpec,
	enc *envelope.Encryptor,
) *ds.Deployment {
	t.Helper()
	accountID := ensureTestAccount(t, db)

	// Resolve environment (produces ConfigMapData + SecretData)
	rctx := deployment.ResolveContext{
		Namespace: "ns-rehydrate-test", AgentName: full.Source.Name, BuildID: full.Source.Build,
	}
	resolved := deployment.ResolveDeploymentSpecEnv(full, rctx)

	// Strip secrets from the spec JSON (mirrors deploy handler behaviour)
	stripped := spec.StripSecretVariableValues(full)
	specJSON, err := json.Marshal(stripped)
	if err != nil {
		t.Fatalf("marshal stripped spec: %v", err)
	}

	params := ds.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: full.Source.Name,
		DisplayName: "Rehydrate Test", BuildID: full.Source.Build,
		Namespace: "ns-rehydrate-test", SpecJSON: string(specJSON),
	}
	if enc != nil {
		params.EncryptedDataKey = enc.EncryptedDataKey
		params.KMSKeyARN = enc.KMSKeyARN
	}

	d, err := store.SaveDeploymentPending(params, func(tx *sql.Tx, depID string) error {
		return ds.SaveNormalizedSpec(tx, depID, full, resolved, enc, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	return d
}

// TestRehydrateSecrets_Plaintext verifies that secret variable values stored as
// plaintext (no KMS) are injected back into a stripped spec by RehydrateSecrets.
func TestRehydrateSecrets_Plaintext(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)

	full := minimalSlackSpec()
	dep := saveDeploymentWithSecrets(t, db, store, full, nil)

	// Load the revision — secrets should be stripped
	rev, err := store.GetCurrentRevision(dep.ID)
	if err != nil {
		t.Fatalf("GetCurrentRevision: %v", err)
	}
	var loaded spec.AstroDeploymentSpec
	if err := json.Unmarshal(rev.SpecJSON, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.Variables["SLACK_BOT_TOKEN"].Value != "" {
		t.Fatal("expected stripped spec to have empty SLACK_BOT_TOKEN value")
	}

	// Reload deployment with full columns (EncryptedDataKey, etc.)
	dep, err = store.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}

	d := &deployer.Deployer{
		Store: store,
		Cfg:   &config.Config{},
		Log:   logger.New("error", "text"),
	}

	if err := d.RehydrateSecrets(context.Background(), dep, &loaded); err != nil {
		t.Fatalf("RehydrateSecrets: %v", err)
	}

	// Verify secret values were restored
	if got := loaded.Variables["SLACK_BOT_TOKEN"].Value; got != "xoxb-test-token-value" {
		t.Errorf("SLACK_BOT_TOKEN: got %q, want %q", got, "xoxb-test-token-value")
	}
	if got := loaded.Variables["SLACK_APP_TOKEN"].Value; got != "xapp-test-token-value" {
		t.Errorf("SLACK_APP_TOKEN: got %q, want %q", got, "xapp-test-token-value")
	}

	// Non-secret should be unchanged
	if got := loaded.Variables["LOG_LEVEL"].Value; got != "debug" {
		t.Errorf("LOG_LEVEL: got %q, want %q", got, "debug")
	}
}

// TestRehydrateSecrets_Encrypted verifies end-to-end rehydration with KMS
// envelope encryption using a fake KMS key.
func TestRehydrateSecrets_Encrypted(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)

	// Create a test encryptor with a random AES-256 key
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatal(err)
	}
	enc, err := envelope.NewTestEncryptor(aesKey)
	if err != nil {
		t.Fatalf("NewTestEncryptor: %v", err)
	}

	full := minimalSlackSpec()
	dep := saveDeploymentWithSecrets(t, db, store, full, enc)

	// Verify secrets are actually encrypted in the DB (not plaintext)
	storedVars, err := store.GetDeploymentVariables(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	for _, sv := range storedVars {
		if sv.Secret && (sv.Value == "xoxb-test-token-value" || sv.Value == "xapp-test-token-value") {
			t.Fatalf("secret %q stored as plaintext — encryption failed", sv.Name)
		}
	}

	// Load the stripped spec from the revision
	rev, err := store.GetCurrentRevision(dep.ID)
	if err != nil {
		t.Fatalf("GetCurrentRevision: %v", err)
	}
	var loaded spec.AstroDeploymentSpec
	if err := json.Unmarshal(rev.SpecJSON, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Reload deployment to get EncryptedDataKey
	dep, err = store.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}

	d := &deployer.Deployer{
		Store:     store,
		Cfg:       &config.Config{Deployment: config.DeploymentConfig{KMSKeyARN: "arn:aws:kms:test:000:key/test"}},
		Log:       logger.New("error", "text"),
		KMSClient: &fakeKMS{key: aesKey},
	}

	if err := d.RehydrateSecrets(context.Background(), dep, &loaded); err != nil {
		t.Fatalf("RehydrateSecrets: %v", err)
	}

	// Verify decrypted values match originals
	if got := loaded.Variables["SLACK_BOT_TOKEN"].Value; got != "xoxb-test-token-value" {
		t.Errorf("SLACK_BOT_TOKEN: got %q, want %q", got, "xoxb-test-token-value")
	}
	if got := loaded.Variables["SLACK_APP_TOKEN"].Value; got != "xapp-test-token-value" {
		t.Errorf("SLACK_APP_TOKEN: got %q, want %q", got, "xapp-test-token-value")
	}
	if got := loaded.Variables["LOG_LEVEL"].Value; got != "debug" {
		t.Errorf("LOG_LEVEL: got %q, want %q", got, "debug")
	}
}

// TestRehydrateSecrets_NoVariables verifies that rehydration is a no-op when
// the deployment has no stored variables.
func TestRehydrateSecrets_NoVariables(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	// Save a deployment with no variables
	noVarSpec := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "no-vars", Build: "b1", Registry: "r"},
		Agent: spec.DeploymentAgent{
			Image:     "img:b1",
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
			Replicas:  1,
		},
	}
	specJSON, _ := json.Marshal(noVarSpec)
	dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: "no-vars",
		DisplayName: "No Vars", BuildID: "b1",
		Namespace: "ns-no-vars", SpecJSON: string(specJSON),
	}, func(tx *sql.Tx, depID string) error {
		return ds.SaveNormalizedSpec(tx, depID, noVarSpec, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	dep, _ = store.GetDeploymentByID(dep.ID)

	loaded := *noVarSpec
	d := &deployer.Deployer{
		Store: store, Cfg: &config.Config{}, Log: logger.New("error", "text"),
	}

	if err := d.RehydrateSecrets(context.Background(), dep, &loaded); err != nil {
		t.Fatalf("RehydrateSecrets: %v", err)
	}
	// Should be a no-op — no variables to rehydrate
}

// fakeKMS implements envelope.KMSClient for testing. It treats the encrypted
// data key as the raw AES key (the "Decrypt" call returns it as-is).
type fakeKMS struct {
	key []byte
}

func (f *fakeKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return nil, nil // unused in rehydrate path
}

func (f *fakeKMS) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	// NewTestEncryptor stores the raw AES key as EncryptedDataKey, so
	// "decrypting" it just means returning the same bytes.
	plain := make([]byte, len(params.CiphertextBlob))
	copy(plain, params.CiphertextBlob)
	return &kms.DecryptOutput{Plaintext: plain}, nil
}
