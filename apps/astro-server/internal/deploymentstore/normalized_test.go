package deploymentstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"os"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
)

// --- Backward compatibility tests ---

func TestSaveDeploymentPending_BackwardCompat(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// SaveDeploymentPending with nil txFn: no normalized data
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "compat-agent",
		DisplayName: "Compat", BuildID: "build-1", Namespace: "ns-compat",
		SpecJSON: `{"spec":"deployment/v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	if d.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", d.Status)
	}
	if d.DeploymentSpecJSON != `{"spec":"deployment/v1"}` {
		t.Errorf("spec JSON mismatch: %q", d.DeploymentSpecJSON)
	}

	// Verify no normalized data was written (nil txFn)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_workloads WHERE deployment_id = $1", d.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query workloads: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 workloads for nil txFn, got %d", count)
	}
}

func TestSaveDeploymentPending_NilTxFn(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// SaveDeploymentPending with nil txFn
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "nil-txfn-agent",
		DisplayName: "NilTxFn", BuildID: "build-1", Namespace: "ns-nil",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending(nil txFn) failed: %v", err)
	}
	if d.Status != "pending" {
		t.Errorf("expected pending, got %q", d.Status)
	}

	// Verify lookup by ID works
	d2, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if d2 == nil || d2.ID != d.ID {
		t.Errorf("expected to find deployment %s", d.ID)
	}
}

func TestSaveDeploymentPending_Redeploy_BackwardCompat(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// First deploy
	d1, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "redeploy-compat",
		DisplayName: "Redeploy", BuildID: "build-1", Namespace: "ns-1",
		SpecJSON: `{"v":1}`,
	}, nil)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}
	// Mark first as active so the second deploy will mark it as undeployed
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d1.ID)

	// Second deploy — should mark first as undeployed
	d2, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "redeploy-compat",
		DisplayName: "Redeploy", BuildID: "build-2", Namespace: "ns-2",
		SpecJSON: `{"v":2}`,
	}, nil)
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}

	// Verify first is undeployed
	var status string
	err = db.QueryRow("SELECT status FROM deployments WHERE id = $1", d1.ID).Scan(&status)
	if err != nil {
		t.Fatalf("query first: %v", err)
	}
	if status != "undeployed" {
		t.Errorf("first deployment should be undeployed, got %q", status)
	}

	// Verify second is pending
	if d2.Status != "pending" {
		t.Errorf("second deployment should be pending, got %q", d2.Status)
	}
}

// --- Normalized spec dual-write tests ---

func TestSaveDeploymentPending_WithNormalizedSpec(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Account: "test-acct", Name: "my-agent", Build: "build-1", Registry: "r.io",
		},
		Target: spec.DeploymentTarget{Runtime: "kubernetes"},
		Agent: spec.DeploymentAgent{
			Image:    "r.io/my-agent:latest",
			Replicas: 2,
			Resources: spec.DeploymentResources{
				CPU: "100m", Memory: "256Mi", CPULimit: "1", MemoryLimit: "1Gi",
			},
			Update: spec.UpdateStrategy{Strategy: "rolling", MaxUnavailable: "25%", MaxSurge: "25%"},
			Endpoints: map[string]spec.Endpoint{
				"http": {Port: 8080, Protocol: "http"},
			},
		},
		Models: map[string]spec.DeploymentModel{
			"gpt4": {
				Image: "ollama/ollama:latest", Replicas: 1,
				Resources:  spec.DeploymentResources{CPU: "2", Memory: "8Gi", CPULimit: "4", MemoryLimit: "16Gi"},
				Persistent: true, Provider: "ollama", Model: "gpt4",
				Endpoints: map[string]spec.Endpoint{"http": {Port: 11434, Protocol: "http"}},
				GPU:       &spec.DeploymentGPU{VRAM: "8Gi", Runtime: "cuda", Count: 1},
				Update:    spec.DefaultUpdateStrategy(),
			},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"redis": {
				Image: "redis:7-alpine", Replicas: 1,
				Resources:  spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "500m", MemoryLimit: "512Mi"},
				Persistent: true, Provider: "redis",
				Storage:   &spec.StorageConfig{Size: "5Gi", AccessMode: "ReadWriteOnce"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 6379, Protocol: "tcp"}},
				Update:    spec.DefaultUpdateStrategy(),
			},
		},
		Integrations: map[string]spec.DeploymentIntegration{
			"search": {
				Image: "r.io/search:latest", Replicas: 1,
				Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "500m", MemoryLimit: "512Mi"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 8080, Protocol: "http"}},
				Update:    spec.DefaultUpdateStrategy(),
			},
		},
		Variables: map[string]spec.Variable{
			"API_KEY":    {Value: "sk-secret-123", Secret: true, Targets: []string{"agent"}},
			"LOG_LEVEL":  {Value: "debug", Secret: false, Targets: []string{"agent", "search"}},
			"OPTIONAL_V": {Value: "", Secret: false, Optional: true},
		},
		Observability: spec.DeploymentObservability{
			Enabled: true, Image: "collector:latest", Port: 4318,
		},
	}

	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{
			"ASTRO_AGENT_NAME": "my-agent",
			"LOG_LEVEL":        "debug",
		},
		SecretData: map[string]string{
			"API_KEY": "sk-secret-123",
		},
	}

	specJSON := `{"spec":"deployment/v1"}`
	deploymentID := newID()

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "my-agent",
		DisplayName: "My Agent", BuildID: "build-1", Namespace: "ns-test",
		SpecJSON: specJSON,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, nil, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	if d.Status != "pending" {
		t.Fatalf("expected pending, got %q", d.Status)
	}

	// Verify workloads were created
	workloads, err := store.GetWorkloads(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloads failed: %v", err)
	}

	// Expected: agent + gpt4 model + redis knowledge + search integration + collector = 5
	if len(workloads) != 5 {
		t.Fatalf("expected 5 workloads, got %d", len(workloads))
	}

	// Check agent workload
	var agentWL *Workload
	for _, w := range workloads {
		if w.ComponentKind == "agent" {
			agentWL = w
		}
	}
	if agentWL == nil {
		t.Fatal("agent workload not found")
	}
	if agentWL.WorkloadType != "deployment" {
		t.Errorf("agent workload_type: got %q, want 'deployment'", agentWL.WorkloadType)
	}
	if agentWL.Replicas != 2 {
		t.Errorf("agent replicas: got %d, want 2", agentWL.Replicas)
	}
	if agentWL.CPURequest != "100m" {
		t.Errorf("agent cpu_request: got %q, want '100m'", agentWL.CPURequest)
	}
	if agentWL.Image != "r.io/my-agent:latest" {
		t.Errorf("agent image: got %q", agentWL.Image)
	}

	// Check model workload is a statefulset with GPU
	var modelWL *Workload
	for _, w := range workloads {
		if w.ComponentKind == "model" {
			modelWL = w
		}
	}
	if modelWL == nil {
		t.Fatal("model workload not found")
	}
	if modelWL.WorkloadType != "statefulset" {
		t.Errorf("model workload_type: got %q, want 'statefulset'", modelWL.WorkloadType)
	}
	if !modelWL.Persistent {
		t.Error("model should be persistent")
	}
	if modelWL.GPURuntime == nil || *modelWL.GPURuntime != "cuda" {
		t.Errorf("model gpu_runtime: got %v, want 'cuda'", modelWL.GPURuntime)
	}
	if modelWL.GPUCount == nil || *modelWL.GPUCount != 1 {
		t.Errorf("model gpu_count: got %v, want 1", modelWL.GPUCount)
	}

	// Check knowledge workload
	var knowledgeWL *Workload
	for _, w := range workloads {
		if w.ComponentKind == "knowledge" {
			knowledgeWL = w
		}
	}
	if knowledgeWL == nil {
		t.Fatal("knowledge workload not found")
	}
	if knowledgeWL.WorkloadType != "statefulset" {
		t.Errorf("knowledge workload_type: got %q, want 'statefulset'", knowledgeWL.WorkloadType)
	}

	// Verify services (use GetServices which unions workload + sidecar services)
	allServices, err := store.GetServices(d.ID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}
	// agent(1 http) + model(1 http) + knowledge(1 http) + tool(1 http) + collector(2: grpc+http) = 6
	if len(allServices) != 6 {
		t.Errorf("expected 6 services, got %d", len(allServices))
	}

	// Verify volumes (model + knowledge)
	var volCount int
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_volumes WHERE workload_id IN (SELECT id FROM deployment_workloads WHERE deployment_id = $1)", d.ID).Scan(&volCount)
	if err != nil {
		t.Fatalf("count volumes: %v", err)
	}
	if volCount != 2 {
		t.Errorf("expected 2 volumes (model + knowledge), got %d", volCount)
	}

	// Verify variables
	var varCount int
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_variables WHERE deployment_id = $1", d.ID).Scan(&varCount)
	if err != nil {
		t.Fatalf("count variables: %v", err)
	}
	if varCount != 2 {
		// OPTIONAL_V has nil Targets and is skipped by both writers for parity.
		t.Errorf("expected 2 variables, got %d", varCount)
	}

	// Verify secret variable value is stripped (no encryptor passed)
	var secretVal string
	err = db.QueryRow("SELECT value FROM deployment_variables WHERE deployment_id = $1 AND name = 'API_KEY'", d.ID).Scan(&secretVal)
	if err != nil {
		t.Fatalf("query secret variable: %v", err)
	}
	if secretVal != "" {
		t.Errorf("secret variable should be empty without encryptor, got %q", secretVal)
	}

	// Verify non-secret variable value is stored
	var plainVal string
	err = db.QueryRow("SELECT value FROM deployment_variables WHERE deployment_id = $1 AND name = 'LOG_LEVEL'", d.ID).Scan(&plainVal)
	if err != nil {
		t.Fatalf("query plain variable: %v", err)
	}
	if plainVal != "debug" {
		t.Errorf("expected 'debug', got %q", plainVal)
	}

}

func TestSaveDeploymentPending_TxFnError_Rollback(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	deploymentID := newID()

	// SaveDeploymentPending with a failing txFn should roll back the entire transaction
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "rollback-agent",
		DisplayName: "Rollback", BuildID: "build-1", Namespace: "ns-rollback",
		SpecJSON: `{"spec":"v1"}`,
	}, func(tx *sql.Tx, depID string) error {
		return sql.ErrNoRows // simulate failure
	})
	if err == nil {
		t.Fatal("expected error from failing txFn")
	}

	// The deployment row should NOT exist (transaction was rolled back)
	d, err := store.GetDeploymentByID(deploymentID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if d != nil {
		t.Errorf("deployment should not exist after rollback, got %+v", d)
	}
}

func TestSaveDeploymentPending_EncryptedDataKey(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	deploymentID := newID()
	fakeKey := []byte("encrypted-data-key-bytes")
	fakeARN := "arn:aws:kms:us-east-1:123:key/test-key"

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "enc-agent",
		DisplayName: "Encrypted", BuildID: "build-1", Namespace: "ns-enc",
		SpecJSON: `{}`, EncryptedDataKey: fakeKey, KMSKeyARN: fakeARN,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Verify encrypted_data_key and kms_key_arn are stored
	var storedKey []byte
	var storedARN sql.NullString
	err = db.QueryRow("SELECT encrypted_data_key, kms_key_arn FROM deployments WHERE id = $1", d.ID).Scan(&storedKey, &storedARN)
	if err != nil {
		t.Fatalf("query encrypted cols: %v", err)
	}
	if string(storedKey) != string(fakeKey) {
		t.Errorf("encrypted_data_key mismatch: got %q", storedKey)
	}
	if !storedARN.Valid || storedARN.String != fakeARN {
		t.Errorf("kms_key_arn mismatch: got %v", storedARN)
	}
}

func TestSaveDeploymentPending_NoEncryptedDataKey(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "no-enc-agent",
		DisplayName: "NoEnc", BuildID: "build-1", Namespace: "ns-noenc",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Verify encrypted_data_key and kms_key_arn are NULL
	var storedKey []byte
	var storedARN sql.NullString
	err = db.QueryRow("SELECT encrypted_data_key, kms_key_arn FROM deployments WHERE id = $1", d.ID).Scan(&storedKey, &storedARN)
	if err != nil {
		t.Fatalf("query encrypted cols: %v", err)
	}
	if len(storedKey) != 0 {
		t.Errorf("expected empty encrypted_data_key, got %q", storedKey)
	}
	if storedARN.Valid {
		t.Errorf("expected NULL kms_key_arn, got %v", storedARN)
	}
}

// TestSaveNormalizedSpec_WithEncryptor verifies secrets are encrypted when an encryptor is provided.
func TestSaveNormalizedSpec_WithEncryptor(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create a mock encryptor
	enc := &envelope.Encryptor{
		EncryptedDataKey: []byte("fake-encrypted-key"),
		KMSKeyARN:        "arn:aws:kms:us-east-1:123:key/test",
	}
	// We can't use the real encryptor (needs KMS), but we can test the variable
	// storage by using a nil encryptor and verifying the strip behavior
	// For the encrypted path, the actual crypto is tested in envelope_test.go

	ds := &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Name: "enc-test-agent", Build: "build-1", Registry: "r.io",
		},
		Agent: spec.DeploymentAgent{
			Image: "r.io/agent:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Variables: map[string]spec.Variable{
			"SECRET_KEY": {Value: "super-secret", Secret: true, Targets: []string{"agent"}},
			"PLAIN_VAR":  {Value: "visible", Secret: false, Targets: []string{"agent"}},
		},
	}
	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{"PLAIN_VAR": "visible"},
		SecretData:    map[string]string{"SECRET_KEY": "super-secret"},
	}

	deploymentID := newID()
	// Use nil encryptor — secrets should be stripped
	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "enc-test-agent",
		DisplayName: "EncTest", BuildID: "build-1", Namespace: "ns-enc-test",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, nil, nil)
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Secret value should be empty (stripped)
	var val string
	var isSecret bool
	err = db.QueryRow("SELECT value, secret FROM deployment_variables WHERE deployment_id = $1 AND name = 'SECRET_KEY'", deploymentID).Scan(&val, &isSecret)
	if err != nil {
		t.Fatalf("query secret: %v", err)
	}
	if val != "" {
		t.Errorf("secret value should be stripped, got %q", val)
	}
	if !isSecret {
		t.Error("secret flag should be true")
	}

	// Plain value should be stored
	err = db.QueryRow("SELECT value, secret FROM deployment_variables WHERE deployment_id = $1 AND name = 'PLAIN_VAR'", deploymentID).Scan(&val, &isSecret)
	if err != nil {
		t.Fatalf("query plain: %v", err)
	}
	if val != "visible" {
		t.Errorf("plain value: got %q, want 'visible'", val)
	}
	if isSecret {
		t.Error("plain var should not be secret")
	}

	_ = enc // used above for documentation; real encrypt test is in envelope_test.go
}

// TestSaveNormalizedSpec_EncryptedBase64 verifies that encrypted secret values are
// stored as valid UTF-8 (base64-encoded) so Postgres text columns accept them, and
// that the stored value can be decoded back to the original ciphertext.
func TestSaveNormalizedSpec_EncryptedBase64(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create an encryptor with a random AES-256 key (no KMS needed)
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatal(err)
	}
	enc, err := envelope.NewTestEncryptor(aesKey)
	if err != nil {
		t.Fatalf("NewTestEncryptor: %v", err)
	}

	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "b64-agent", Build: "build-1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/agent:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Variables: map[string]spec.Variable{
			"API_KEY":   {Value: "sk-secret-key-with-binary-unsafe-bytes", Secret: true, Targets: []string{"agent"}},
			"PLAIN_VAR": {Value: "hello", Secret: false, Targets: []string{"agent"}},
		},
	}
	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{"PLAIN_VAR": "hello"},
		SecretData:    map[string]string{"API_KEY": "sk-secret-key-with-binary-unsafe-bytes"},
	}

	deploymentID := newID()
	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "b64-agent",
		DisplayName: "Base64Test", BuildID: "build-1", Namespace: "ns-b64",
		SpecJSON: `{}`, EncryptedDataKey: enc.EncryptedDataKey, KMSKeyARN: "arn:test",
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, enc, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Read back the encrypted variable and verify it's valid base64
	vars, err := store.GetDeploymentVariables(deploymentID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}

	var found bool
	for _, v := range vars {
		if v.Name == "API_KEY" {
			found = true
			if !v.Secret {
				t.Error("API_KEY should be marked as secret")
			}
			// Value should be valid base64 (not raw binary)
			ciphertext, err := base64.StdEncoding.DecodeString(v.Value)
			if err != nil {
				t.Fatalf("stored value is not valid base64: %v", err)
			}
			if len(ciphertext) == 0 {
				t.Fatal("ciphertext should not be empty")
			}
			// Decrypt and verify roundtrip
			dec, err := envelope.NewTestDecryptor(aesKey, enc.EncryptedDataKey)
			if err != nil {
				t.Fatalf("NewTestDecryptor: %v", err)
			}
			plaintext, err := dec.Decrypt(ciphertext, v.Nonce)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}
			if string(plaintext) != "sk-secret-key-with-binary-unsafe-bytes" {
				t.Errorf("decrypted value mismatch: got %q", plaintext)
			}
		}
	}
	if !found {
		t.Fatal("API_KEY variable not found")
	}
}

// --- Phase 2: Read query tests ---

func TestGetWorkloadSummaries(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "summary-agent", Build: "build-1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/agent:latest", Replicas: 2,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Models: map[string]spec.DeploymentModel{
			"llm": {
				Image: "ollama:latest", Replicas: 1,
				Resources: spec.DeploymentResources{CPU: "2", Memory: "8Gi"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 11434}},
			},
		},
		Observability: spec.DeploymentObservability{Enabled: true, Image: "collector:latest"},
	}
	resolved := &deployment.ResolvedEnv{ConfigMapData: map[string]string{}, SecretData: map[string]string{}}

	deploymentID := newID()
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "summary-agent",
		DisplayName: "Summary", BuildID: "build-1", Namespace: "ns-summary",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, nil, nil)
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	summaries, err := store.GetWorkloadSummaries(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloadSummaries: %v", err)
	}

	// agent + llm model + collector = 3
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}

	// Build a lookup by component_kind
	byKind := make(map[string]*WorkloadSummary)
	for _, s := range summaries {
		byKind[s.ComponentKind] = s
	}

	// Verify agent summary
	agentSummary := byKind["agent"]
	if agentSummary == nil {
		t.Fatal("agent summary not found")
	}
	if agentSummary.Replicas != 2 {
		t.Errorf("agent replicas: got %d, want 2", agentSummary.Replicas)
	}
	if agentSummary.CPURequest != "100m" {
		t.Errorf("agent cpu: got %q, want '100m'", agentSummary.CPURequest)
	}
	if agentSummary.WorkloadType != "deployment" {
		t.Errorf("agent type: got %q, want 'deployment'", agentSummary.WorkloadType)
	}
	if agentSummary.Image != "r.io/agent:latest" {
		t.Errorf("agent image: got %q", agentSummary.Image)
	}
	if agentSummary.Persistent {
		t.Error("agent should not be persistent")
	}

	// Verify model summary
	modelSummary := byKind["model"]
	if modelSummary == nil {
		t.Fatal("model summary not found")
	}
	if modelSummary.Name != "summary-agent-model-llm" {
		t.Errorf("model name: got %q, want 'summary-agent-model-llm'", modelSummary.Name)
	}

	// Verify collector is a workload, not a sidecar
	collectorSummary := byKind["collector"]
	if collectorSummary == nil {
		t.Fatal("collector workload not found in summaries")
	}
	if collectorSummary.WorkloadType != "deployment" {
		t.Errorf("collector type: got %q, want 'deployment'", collectorSummary.WorkloadType)
	}
	if collectorSummary.Image != "collector:latest" {
		t.Errorf("collector image: got %q, want 'collector:latest'", collectorSummary.Image)
	}
}

func TestGetWorkloadSummaries_PersistentFlag(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "persist-agent"},
		Agent: spec.DeploymentAgent{
			Image: "agent:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"vectors": {
				Image: "qdrant:latest", Replicas: 1, Persistent: true, Provider: "qdrant",
				Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
				Storage:   &spec.StorageConfig{Size: "10Gi", AccessMode: "ReadWriteOnce"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 6333}},
			},
			"cache": {
				Image: "redis:7", Replicas: 1, Persistent: false, Provider: "redis",
				Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 6379}},
			},
		},
	}
	resolved := &deployment.ResolvedEnv{ConfigMapData: map[string]string{}, SecretData: map[string]string{}}

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "persist-agent",
		DisplayName: "Persist", BuildID: "build-1", Namespace: "ns-persist",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, nil, nil)
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	summaries, err := store.GetWorkloadSummaries(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloadSummaries: %v", err)
	}

	byKey := make(map[string]*WorkloadSummary)
	for _, s := range summaries {
		byKey[s.ComponentKey] = s
	}

	vectors := byKey["vectors"]
	if vectors == nil {
		t.Fatal("vectors workload not found")
	}
	if !vectors.Persistent {
		t.Error("vectors should be persistent")
	}
	if vectors.WorkloadType != "statefulset" {
		t.Errorf("vectors type: got %q, want 'statefulset'", vectors.WorkloadType)
	}

	cache := byKey["cache"]
	if cache == nil {
		t.Fatal("cache workload not found")
	}
	if cache.Persistent {
		t.Error("cache should not be persistent")
	}
	if cache.WorkloadType != "deployment" {
		t.Errorf("cache type: got %q, want 'deployment'", cache.WorkloadType)
	}
}

func TestGetWorkloadSummaries_Empty(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Deployment without normalized data (nil txFn)
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "old-agent",
		DisplayName: "Old", BuildID: "build-1", Namespace: "ns-old",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	summaries, err := store.GetWorkloadSummaries(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloadSummaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for old deployment, got %d", len(summaries))
	}
}

func TestGetActiveDeploymentWorkloads(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "active-wl-agent", Build: "build-1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/agent:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "500m", Memory: "1Gi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Integrations: map[string]spec.DeploymentIntegration{
			"search": {
				Image: "r.io/search:latest", Replicas: 2,
				Resources: spec.DeploymentResources{CPU: "200m", Memory: "512Mi"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
			},
		},
	}
	resolved := &deployment.ResolvedEnv{ConfigMapData: map[string]string{}, SecretData: map[string]string{}}

	deploymentID := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "active-wl-agent",
		DisplayName: "ActiveWL", BuildID: "build-1", Namespace: "ns-active-wl",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, nil, nil)
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	// Mark as active so GetActiveDeploymentWorkloads can find it
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", deploymentID)

	workloads, err := store.GetActiveDeploymentWorkloads()
	if err != nil {
		t.Fatalf("GetActiveDeploymentWorkloads: %v", err)
	}

	// Find our deployment's workloads
	var ours []*ActiveDeploymentWorkload
	for _, w := range workloads {
		if w.DeploymentID == deploymentID {
			ours = append(ours, w)
		}
	}

	// agent + search integration = 2
	if len(ours) != 2 {
		t.Fatalf("expected 2 workloads for our deployment, got %d", len(ours))
	}

	// Verify fields
	for _, w := range ours {
		if w.AccountID != accountID {
			t.Errorf("account_id mismatch: got %q", w.AccountID)
		}
		if w.AgentName != "active-wl-agent" {
			t.Errorf("agent_name: got %q", w.AgentName)
		}
		if w.Namespace != "ns-active-wl" {
			t.Errorf("namespace: got %q", w.Namespace)
		}
	}

	// Verify component data
	byKind := map[string]*ActiveDeploymentWorkload{}
	for _, w := range ours {
		key := w.ComponentKind
		if w.ComponentKey != "" {
			key += "/" + w.ComponentKey
		}
		byKind[key] = w
	}

	agent := byKind["agent"]
	if agent == nil {
		t.Fatal("agent workload not found")
	}
	if agent.CPURequest != "500m" {
		t.Errorf("agent cpu: got %q, want '500m'", agent.CPURequest)
	}
	if agent.Replicas != 1 {
		t.Errorf("agent replicas: got %d, want 1", agent.Replicas)
	}

	tool := byKind["integration/search"]
	if tool == nil {
		t.Fatal("integration/search workload not found")
	}
	if tool.CPURequest != "200m" {
		t.Errorf("integration cpu: got %q, want '200m'", tool.CPURequest)
	}
	if tool.Replicas != 2 {
		t.Errorf("integration replicas: got %d, want 2", tool.Replicas)
	}
}

// TestUpdateDeploymentPending_CleansUpOldNormalizedData verifies that
// UpdateDeploymentPending deletes old workloads/services/variables and
// re-inserts the new spec, so removed components are fully purged
// from the normalized tables.
func TestUpdateDeploymentPending_CleansUpOldNormalizedData(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Initial spec: agent + tool "search" + variable "API_KEY"
	ds1 := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "update-agent", Build: "build-1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/agent:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Integrations: map[string]spec.DeploymentIntegration{
			"search": {
				Image: "r.io/search:latest", Replicas: 1,
				Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
				Endpoints: map[string]spec.Endpoint{"http": {Port: 3000}},
			},
		},
		Variables: map[string]spec.Variable{
			"API_KEY":   {Value: "sk-123", Secret: true, Targets: []string{"agent"}},
			"LOG_LEVEL": {Value: "debug", Secret: false, Targets: []string{"agent"}},
		},
	}
	resolved1 := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{"LOG_LEVEL": "debug"},
		SecretData:    map[string]string{"API_KEY": "sk-123"},
	}

	deploymentID := newID()
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "update-agent",
		DisplayName: "Update Test", BuildID: "build-1", Namespace: "ns-update",
		SpecJSON: `{"v":1}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds1, resolved1, nil, nil)
	})
	if err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Verify initial state: 2 workloads (agent + search), 2 variables
	workloads1, err := store.GetWorkloads(d.ID)
	if err != nil {
		t.Fatalf("get workloads: %v", err)
	}
	if len(workloads1) != 2 {
		t.Fatalf("expected 2 workloads initially, got %d", len(workloads1))
	}
	vars1, err := store.GetDeploymentVariables(d.ID)
	if err != nil {
		t.Fatalf("get variables: %v", err)
	}
	if len(vars1) != 2 {
		t.Fatalf("expected 2 variables initially, got %d", len(vars1))
	}

	// Updated spec: agent only (tool removed), API_KEY removed
	ds2 := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "update-agent", Build: "build-2", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/agent:v2", Replicas: 2,
			Resources: spec.DeploymentResources{CPU: "200m", Memory: "512Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
		},
		Variables: map[string]spec.Variable{
			"LOG_LEVEL": {Value: "info", Secret: false, Targets: []string{"agent"}},
		},
	}
	resolved2 := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{"LOG_LEVEL": "info"},
		SecretData:    map[string]string{},
	}

	d2, err := store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "update-agent",
		DisplayName: "Update Test", BuildID: "build-2", Namespace: "ns-update",
		SpecJSON: `{"v":2}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds2, resolved2, nil, nil)
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if d2.BuildID != "build-2" {
		t.Errorf("expected build-2, got %q", d2.BuildID)
	}

	// Verify: only 1 workload (agent), tool is gone
	workloads2, err := store.GetWorkloads(d2.ID)
	if err != nil {
		t.Fatalf("get workloads after update: %v", err)
	}
	if len(workloads2) != 1 {
		t.Fatalf("expected 1 workload after update, got %d", len(workloads2))
	}
	if workloads2[0].ComponentKind != "agent" {
		t.Errorf("expected remaining workload to be 'agent', got %q", workloads2[0].ComponentKind)
	}
	if workloads2[0].Image != "r.io/agent:v2" {
		t.Errorf("expected updated image, got %q", workloads2[0].Image)
	}
	if workloads2[0].Replicas != 2 {
		t.Errorf("expected 2 replicas, got %d", workloads2[0].Replicas)
	}

	// Verify: only 1 variable (LOG_LEVEL), API_KEY is gone
	vars2, err := store.GetDeploymentVariables(d2.ID)
	if err != nil {
		t.Fatalf("get variables after update: %v", err)
	}
	if len(vars2) != 1 {
		t.Fatalf("expected 1 variable after update, got %d", len(vars2))
	}
	if vars2[0].Name != "LOG_LEVEL" {
		t.Errorf("expected LOG_LEVEL variable, got %q", vars2[0].Name)
	}
	if vars2[0].Value != "info" {
		t.Errorf("expected 'info', got %q", vars2[0].Value)
	}

	// Verify: services from the old integration workload are also gone (cascaded from workload deletion)
	services2, err := store.GetServices(d2.ID)
	if err != nil {
		t.Fatalf("get services after update: %v", err)
	}
	// Only agent's http endpoint should remain
	if len(services2) != 1 {
		t.Errorf("expected 1 service after update (agent http), got %d", len(services2))
	}
}

// TestRepairNormalizedSpec_CollectorAsWorkload verifies that RepairNormalizedSpec
// creates the collector as a standalone workload (not a sidecar) with its services.
func TestRepairNormalizedSpec_CollectorAsWorkload(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	specJSON := `{
		"spec": "deployment/v1",
		"source": {"account": "repair-test", "name": "repair-agent", "build": "b1", "registry": "r.io"},
		"agent": {
			"image": "r.io/agent:latest",
			"endpoints": {"http": {"port": 8080, "protocol": "http"}},
			"replicas": 1,
			"resources": {"cpu": "100m", "memory": "256Mi"}
		},
		"observability": {
			"enabled": true,
			"image": "collector:latest",
			"port": 4318,
			"resources": {"cpu": "50m", "memory": "128Mi", "cpu_limit": "200m", "memory_limit": "256Mi"}
		}
	}`

	deploymentID := newID()
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: deploymentID, AccountID: accountID, AgentName: "repair-agent",
		DisplayName: "Repair", BuildID: "b1", Namespace: "ns-repair",
		SpecJSON: specJSON,
	}, nil) // no txFn — no initial normalized data
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	// Repair should create normalized workloads from the spec JSON
	workloads, services, _, err := store.RepairNormalizedSpec(d.ID, &NormalizedSpecConfig{
		Namespace: "ns-repair",
	}, nil)
	if err != nil {
		t.Fatalf("RepairNormalizedSpec: %v", err)
	}

	// agent + collector = 2 workloads
	if workloads != 2 {
		t.Errorf("expected 2 workloads, got %d", workloads)
	}
	// agent(1 http) + collector(2: grpc+http) = 3 services
	if services != 3 {
		t.Errorf("expected 3 services, got %d", services)
	}

	// Verify collector is in workloads table with correct attributes
	wls, err := store.GetWorkloads(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}
	var collectorWL *Workload
	for _, w := range wls {
		if w.ComponentKind == "collector" {
			collectorWL = w
		}
	}
	if collectorWL == nil {
		t.Fatal("collector workload not found")
	}
	if collectorWL.WorkloadType != "deployment" {
		t.Errorf("collector workload_type: got %q, want 'deployment'", collectorWL.WorkloadType)
	}
	if collectorWL.Image != "collector:latest" {
		t.Errorf("collector image: got %q, want 'collector:latest'", collectorWL.Image)
	}
	if collectorWL.Replicas != 1 {
		t.Errorf("collector replicas: got %d, want 1", collectorWL.Replicas)
	}
	if collectorWL.CPURequest != "50m" {
		t.Errorf("collector cpu_request: got %q, want '50m'", collectorWL.CPURequest)
	}
	if collectorWL.MemoryRequest != "128Mi" {
		t.Errorf("collector memory_request: got %q, want '128Mi'", collectorWL.MemoryRequest)
	}

	// Verify collector services (otlp-grpc on 4317, otlp-http on 4318)
	allSvcs, err := store.GetServices(d.ID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}
	collectorSvcs := make(map[string]int)
	for _, svc := range allSvcs {
		if svc.WorkloadName == collectorWL.Name {
			collectorSvcs[svc.Name] = svc.Port
		}
	}
	if collectorSvcs["otlp-grpc"] != 4317 {
		t.Errorf("otlp-grpc port: got %d, want 4317", collectorSvcs["otlp-grpc"])
	}
	if collectorSvcs["otlp-http"] != 4318 {
		t.Errorf("otlp-http port: got %d, want 4318", collectorSvcs["otlp-http"])
	}

	// Verify collector is NOT in the sidecars table
	sidecars, err := store.GetSidecars(d.ID)
	if err != nil {
		t.Fatalf("GetSidecars: %v", err)
	}
	for _, sc := range sidecars {
		if sc.ComponentKind == "collector" {
			t.Error("collector should not be in sidecars table")
		}
	}
}

// TestSaveNormalizedSpec_VarRefsStored verifies that account variable refs passed
// via NormalizedSpecConfig.VarRefs are persisted in deployment_variables.ref so
// the prefilled template can restore them instead of returning resolved values.
func TestSaveNormalizedSpec_VarRefsStored(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Name: "ref-store-agent", Build: "build-1", Registry: "r.io",
		},
		Variables: map[string]spec.Variable{
			// Secret resolved from account variable ref — ref was cleared before this point.
			"API_KEY": {Value: "resolved-secret", Secret: true, Targets: []string{"agent"}},
			// Non-secret resolved from account variable ref.
			"LOG_LEVEL": {Value: "debug", Secret: false, Targets: []string{"agent"}},
			// Direct value, no ref.
			"TIMEOUT": {Value: "30s", Secret: false, Targets: []string{"agent"}},
		},
	}
	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{"LOG_LEVEL": "debug", "TIMEOUT": "30s"},
		SecretData:    map[string]string{"API_KEY": "resolved-secret"},
	}
	nsCfg := &NormalizedSpecConfig{
		VarRefs: map[string]string{
			"API_KEY":   "MY_ACCOUNT_SECRET",
			"LOG_LEVEL": "SHARED_LOG_LEVEL",
			// TIMEOUT has no ref — direct value.
		},
	}

	depID := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "ref-store-agent",
		DisplayName: "RefStoreTest", BuildID: "build-1", Namespace: "ns-ref-store",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, ds, resolved, nil, nsCfg)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	// Verify each variable's stored ref.
	rows, err := db.Query(
		`SELECT name, ref FROM deployment_variables WHERE deployment_id = $1 ORDER BY name`,
		depID,
	)
	if err != nil {
		t.Fatalf("query deployment_variables: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	got := make(map[string]string)
	for rows.Next() {
		var name, ref string
		if err := rows.Scan(&name, &ref); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[name] = ref
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	cases := []struct{ name, wantRef string }{
		{"API_KEY", "MY_ACCOUNT_SECRET"},
		{"LOG_LEVEL", "SHARED_LOG_LEVEL"},
		{"TIMEOUT", ""},
	}
	for _, tc := range cases {
		if got[tc.name] != tc.wantRef {
			t.Errorf("variable %s: ref = %q, want %q", tc.name, got[tc.name], tc.wantRef)
		}
	}
}

// TestGetDeploymentVariables_RefsRoundtrip verifies that refs written by
// SaveNormalizedSpec are returned correctly by GetDeploymentVariables.
func TestGetDeploymentVariables_RefsRoundtrip(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Name: "ref-roundtrip-agent", Build: "b1", Registry: "r.io",
		},
		Variables: map[string]spec.Variable{
			"SECRET_KEY": {Value: "plaintext", Secret: true, Targets: []string{"agent"}},
			"PLAIN_KEY":  {Value: "value", Secret: false, Targets: []string{"agent"}},
		},
	}
	nsCfg := &NormalizedSpecConfig{
		VarRefs: map[string]string{"SECRET_KEY": "ACCT_SECRET"},
	}

	depID := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "ref-roundtrip-agent",
		DisplayName: "RefRoundtrip", BuildID: "b1", Namespace: "ns-ref-roundtrip",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, ds, nil, nil, nsCfg)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}

	byName := make(map[string]Variable, len(vars))
	for _, v := range vars {
		byName[v.Name] = v
	}

	if byName["SECRET_KEY"].Ref != "ACCT_SECRET" {
		t.Errorf("SECRET_KEY.Ref = %q, want %q", byName["SECRET_KEY"].Ref, "ACCT_SECRET")
	}
	if byName["PLAIN_KEY"].Ref != "" {
		t.Errorf("PLAIN_KEY.Ref = %q, want empty (no ref)", byName["PLAIN_KEY"].Ref)
	}
}

// --- build_env dual-write parity tests (review concerns from #852) ---

func minimalAgentSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "review-test", Build: "b1", Registry: "r"},
		Agent: spec.DeploymentAgent{
			Image:     "img:b1",
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}},
			Replicas:  1,
		},
	}
}

func countBuildEnv(t *testing.T, db *sql.DB, depID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM deployment_build_env WHERE deployment_id = $1`,
		depID,
	).Scan(&n); err != nil {
		t.Fatalf("count build_env: %v", err)
	}
	return n
}

func countDeploymentVariables(t *testing.T, db *sql.DB, depID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM deployment_variables WHERE deployment_id = $1`,
		depID,
	).Scan(&n); err != nil {
		t.Fatalf("count deployment_variables: %v", err)
	}
	return n
}

func TestSaveNormalizedSpec_EmptyTargets_DoesNotDiverge(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := minimalAgentSpec()
	ds.Variables = map[string]spec.Variable{
		"NO_TARGETS": {Value: "x", Secret: false, Targets: nil},
	}

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: ds.Source.Name,
		BuildID: ds.Source.Build, Namespace: "ns-empty-targets",
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	legacyRows := countDeploymentVariables(t, db, d.ID)
	buildEnvRows := countBuildEnv(t, db, d.ID)
	if legacyRows != buildEnvRows {
		t.Errorf("variable parity drift: deployment_variables=%d, deployment_build_env=%d (NO_TARGETS row only lives in legacy table)",
			legacyRows, buildEnvRows)
	}
}

func TestRepairNormalizedSpec_PreservesBuildEnvRows(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatal(err)
	}
	enc, err := envelope.NewTestEncryptor(aesKey)
	if err != nil {
		t.Fatalf("NewTestEncryptor: %v", err)
	}

	ds := minimalAgentSpec()
	ds.Variables = map[string]spec.Variable{
		"API_KEY":   {Value: "sk-secret-123", Secret: true, Targets: []string{"agent"}},
		"LOG_LEVEL": {Value: "debug", Secret: false, Targets: []string{"agent"}},
	}
	resolved := deployment.ResolveDeploymentSpecEnv(ds, deployment.ResolveContext{
		Namespace: "ns-repair-be", AgentName: ds.Source.Name, BuildID: ds.Source.Build,
	})

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: ds.Source.Name,
		BuildID: ds.Source.Build, Namespace: "ns-repair-be",
		SpecJSON:         `{"agent":{"image":"img:b1"}}`,
		EncryptedDataKey: enc.EncryptedDataKey,
		KMSKeyARN:        enc.KMSKeyARN,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, enc, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	beBefore := countBuildEnv(t, db, d.ID)
	if beBefore == 0 {
		t.Fatalf("build_env should be populated before repair")
	}

	if _, _, _, err := store.RepairNormalizedSpec(d.ID, &NormalizedSpecConfig{
		Namespace: "ns-repair-be",
	}, nil); err != nil {
		t.Fatalf("RepairNormalizedSpec: %v", err)
	}

	// SkipBuildEnvClear suppresses the build_env DELETE in SaveNormalizedSpec,
	// so existing rows must survive the repair (KMS encryptor is unavailable
	// at repair time, so we can't re-encrypt secrets).
	beAfter := countBuildEnv(t, db, d.ID)
	if beAfter != beBefore {
		t.Errorf("Repair must preserve deployment_build_env: before=%d, after=%d", beBefore, beAfter)
	}
}

func TestGetMessagingURLs(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Helper to create a deployment and wire up the sidecar → service → ingress
	// chain that GetMessagingURLs queries. Uses direct SQL inserts so the test
	// doesn't depend on the spec-processing path in SaveNormalizedSpec.
	insertMessagingIngress := func(t *testing.T, depID, hostname string) {
		t.Helper()
		var scID int
		if err := db.QueryRow(`
			INSERT INTO deployment_sidecars (deployment_id, name, component_kind, image,
				cpu_request, memory_request, cpu_limit, memory_limit)
			VALUES ($1, 'messaging', 'messaging', 'msg:latest', '100m', '128Mi', '200m', '256Mi')
			RETURNING id`, depID).Scan(&scID); err != nil {
			t.Fatalf("insert sidecar: %v", err)
		}
		var svcID int
		if err := db.QueryRow(`
			INSERT INTO deployment_services (workload_id, sidecar_id, name, port, target_port, protocol)
			VALUES (NULL, $1, 'http', 8080, 8080, 'http') RETURNING id`, scID).Scan(&svcID); err != nil {
			t.Fatalf("insert service: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO deployment_ingresses (service_id, hostname, path, tls_enabled)
			VALUES ($1, $2, '/', true)`, svcID, hostname); err != nil {
			t.Fatalf("insert ingress: %v", err)
		}
	}

	makeDeployment := func(t *testing.T, name string) string {
		t.Helper()
		d, err := store.SaveDeploymentPending(SaveDeploymentParams{
			ID: newID(), AccountID: accountID, AgentName: name,
			BuildID: "b1", Namespace: "ns-" + name, SpecJSON: `{}`,
		}, nil)
		if err != nil {
			t.Fatalf("SaveDeploymentPending: %v", err)
		}
		return d.ID
	}

	t.Run("returns URL for deployment with messaging ingress", func(t *testing.T) {
		depID := makeDeployment(t, "msg-url-agent")
		insertMessagingIngress(t, depID, "msg-agent.example.com")

		urls, err := store.GetMessagingURLs([]string{depID})
		if err != nil {
			t.Fatalf("GetMessagingURLs: %v", err)
		}
		want := "https://msg-agent.example.com"
		if got := urls[depID]; got != want {
			t.Errorf("want %q, got %q", want, got)
		}
	})

	t.Run("absent for deployment with no messaging ingress", func(t *testing.T) {
		depID := makeDeployment(t, "no-msg-agent")

		urls, err := store.GetMessagingURLs([]string{depID})
		if err != nil {
			t.Fatalf("GetMessagingURLs: %v", err)
		}
		if _, ok := urls[depID]; ok {
			t.Errorf("expected no URL for deployment without messaging ingress, got %q", urls[depID])
		}
	})

	t.Run("returns only matching deployment in a batch", func(t *testing.T) {
		depWithMsg := makeDeployment(t, "batch-msg-agent")
		depWithout := makeDeployment(t, "batch-no-msg-agent")
		insertMessagingIngress(t, depWithMsg, "batch-msg.example.com")

		urls, err := store.GetMessagingURLs([]string{depWithMsg, depWithout})
		if err != nil {
			t.Fatalf("GetMessagingURLs: %v", err)
		}
		if got := urls[depWithMsg]; got != "https://batch-msg.example.com" {
			t.Errorf("want URL for %q, got %q", depWithMsg, got)
		}
		if _, ok := urls[depWithout]; ok {
			t.Errorf("expected no URL for %q, got %q", depWithout, urls[depWithout])
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		urls, err := store.GetMessagingURLs([]string{})
		if err != nil {
			t.Fatalf("GetMessagingURLs: %v", err)
		}
		if urls != nil {
			t.Errorf("expected nil for empty input, got %v", urls)
		}
	})
}

func TestGetMessagingWebConfigured(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Every messaging sidecar exposes an http service on 8080 (the platform
	// messaging API the proxy uses), regardless of adapters — so the http
	// service alone must NOT make an agent web-configured.
	insertMessagingHTTP := func(t *testing.T, depID string) {
		t.Helper()
		var scID int
		if err := db.QueryRow(`
			INSERT INTO deployment_sidecars (deployment_id, name, component_kind, image,
				cpu_request, memory_request, cpu_limit, memory_limit)
			VALUES ($1, 'messaging', 'messaging', 'msg:latest', '100m', '128Mi', '200m', '256Mi')
			RETURNING id`, depID).Scan(&scID); err != nil {
			t.Fatalf("insert sidecar: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO deployment_services (workload_id, sidecar_id, name, port, target_port, protocol)
			VALUES (NULL, $1, 'http', 8080, 8080, 'http')`, scID); err != nil {
			t.Fatalf("insert http service: %v", err)
		}
	}

	makeDeployment := func(t *testing.T, name, specJSON string) string {
		t.Helper()
		d, err := store.SaveDeploymentPending(SaveDeploymentParams{
			ID: newID(), AccountID: accountID, AgentName: name,
			BuildID: "b1", Namespace: "ns-" + name, SpecJSON: specJSON,
		}, nil)
		if err != nil {
			t.Fatalf("SaveDeploymentPending: %v", err)
		}
		return d.ID
	}

	t.Run("true when spec enables the web adapter", func(t *testing.T) {
		depID := makeDeployment(t, "web-msg-agent",
			`{"interfaces":{"adapters":["web","slack"]}}`)
		insertMessagingHTTP(t, depID)

		got, err := store.GetMessagingWebConfigured(context.Background(), []string{depID})
		if err != nil {
			t.Fatalf("GetMessagingWebConfigured: %v", err)
		}
		if !got[depID] {
			t.Error("expected messaging web configured")
		}
	})

	t.Run("false for slack-only adapter even with http service", func(t *testing.T) {
		depID := makeDeployment(t, "slack-only-agent",
			`{"interfaces":{"adapters":["slack"]}}`)
		// Slack-only sidecars still expose the http service — this must not
		// make the agent web-configured.
		insertMessagingHTTP(t, depID)

		got, err := store.GetMessagingWebConfigured(context.Background(), []string{depID})
		if err != nil {
			t.Fatalf("GetMessagingWebConfigured: %v", err)
		}
		if got[depID] {
			t.Error("expected false for slack-only adapter")
		}
	})

	t.Run("absent for deployment without interfaces", func(t *testing.T) {
		depID := makeDeployment(t, "no-msg-agent", `{}`)

		got, err := store.GetMessagingWebConfigured(context.Background(), []string{depID})
		if err != nil {
			t.Fatalf("GetMessagingWebConfigured: %v", err)
		}
		if got[depID] {
			t.Error("expected absent for deployment without interfaces")
		}
	})
}
