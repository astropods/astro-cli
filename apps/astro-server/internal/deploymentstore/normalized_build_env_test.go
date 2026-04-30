package deploymentstore

// normalized_build_env_test.go — integration tests for SaveNormalizedSpec,
// GetBuildEnv, and GetDeploymentVariables covering S1–S17, R1–R4, and V1–V3
// from the deployment behaviour test plan.
//
// All tests require DATABASE_URL and are skipped when it is unset.

import (
	"crypto/rand"
	"database/sql"
	"sort"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// fullSpecDeploymentSpec returns a realistic DeploymentSpec that exercises
// agent, model, persistent knowledge, non-persistent knowledge, ingestion
// (schedule), and observability (collector) — the same mix as TestTemplate_FullSpec.
func fullSpecDeploymentSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "research-assistant", Build: "build1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/research-assistant:build1", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "1", MemoryLimit: "1Gi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080, Protocol: "http"}},
			Update:    spec.DefaultUpdateStrategy(),
		},
		Models: map[string]spec.DeploymentModel{
			"local": {
				Image: "r.io/ollama:latest", Replicas: 1, Provider: "ollama",
				Resources: spec.StandardResources,
				Endpoints: map[string]spec.Endpoint{"http": {Port: 11434, Protocol: "http"}},
				Update:    spec.DefaultUpdateStrategy(),
			},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"db": {
				Image: "r.io/pgvector:latest", Replicas: 1, Provider: "postgres",
				Persistent: true, Storage: &spec.StorageConfig{Size: "10Gi", AccessMode: "ReadWriteOnce"},
				Resources: spec.StandardResources,
				Endpoints: map[string]spec.Endpoint{"http": {Port: 5432, Protocol: "tcp"}},
				Update:    spec.UpdateStrategy{Strategy: "recreate"},
			},
			"cache": {
				Image: "r.io/redis:latest", Replicas: 1, Provider: "redis",
				Persistent: false,
				Resources:  spec.StandardResources,
				Endpoints:  map[string]spec.Endpoint{"http": {Port: 6379, Protocol: "tcp"}},
				Update:     spec.DefaultUpdateStrategy(),
			},
		},
		Ingestion: map[string]spec.DeploymentIngestion{
			"nightly": {Image: "r.io/sync:latest", Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"}, Resources: spec.StandardResources},
		},
		Observability: spec.DeploymentObservability{
			Enabled: true, Image: "r.io/collector:latest", Port: 4318,
			Resources: spec.CollectorResources,
		},
	}
}

// targetingSpec returns a spec whose variables exercise every target type:
// agent, interface.slack, bare ingestion fan-out, named ingestion, multi-target.
func targetingSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "target-test", Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Ingestion: map[string]spec.DeploymentIngestion{
			"nightly": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "schedule"}},
			"hook":    {Image: "hook:b1", Trigger: spec.DeploymentTrigger{Type: "webhook"}},
		},
		Variables: map[string]spec.Variable{
			"AGENT_VAR":    {Value: "va", Targets: []string{"agent"}},
			"SLACK_TOKEN":  {Value: "xoxb-1", Secret: true, Targets: []string{"interface.slack"}},
			"ALL_INGEST":   {Value: "vi", Targets: []string{"ingestion"}},
			"NIGHTLY_ONLY": {Value: "vn", Targets: []string{"ingestion.nightly"}},
			"MULTI":        {Value: "vm", Targets: []string{"agent", "ingestion.hook"}},
			"OPTIONAL_VAR": {Value: "vo", Optional: true, Targets: []string{"agent"}},
			"NO_TARGETS":   {Value: "vx", Targets: nil},
		},
	}
}

func saveSpec(t *testing.T, store *Store, accountID string, name string, ds *spec.AstroDeploymentSpec, enc *envelope.Encryptor) string {
	t.Helper()
	depID := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: name,
		BuildID: "b1", Namespace: "ns-" + name, SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, ds, nil, enc, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending(%s): %v", name, err)
	}
	return depID
}

type buildEnvRow struct {
	role, envName, userVar, acctRef string
	isSecret, optional              bool
}

// buildEnvRows reads all deployment_build_env rows for a deployment and
// returns them keyed by "role|env_name" for easy lookup.
func buildEnvRows(t *testing.T, db *sql.DB, depID string) map[string]buildEnvRow {
	t.Helper()
	rows, err := db.Query(`
		SELECT role, env_name, is_secret, COALESCE(optional, false),
		       COALESCE(user_var_name,''), COALESCE(account_var_ref,'')
		FROM deployment_build_env WHERE deployment_id = $1`, depID)
	if err != nil {
		t.Fatalf("query build_env: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	out := make(map[string]buildEnvRow)
	for rows.Next() {
		var r buildEnvRow
		if err := rows.Scan(&r.role, &r.envName, &r.isSecret, &r.optional, &r.userVar, &r.acctRef); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[r.role+"|"+r.envName] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// ── S1–S3: workloads, volumes, services ──────────────────────────────────────

func TestSaveNormalizedSpec_FullSpec_Workloads(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := fullSpecDeploymentSpec()
	depID := saveSpec(t, store, accountID, "ws-full", ds, nil)

	// S1: workload types
	workloads, err := store.GetWorkloads(depID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}

	byKind := map[string]*Workload{}
	byName := map[string]*Workload{}
	for _, w := range workloads {
		byKind[w.ComponentKind] = w
		byName[w.Name] = w
	}

	// agent → Deployment
	if byKind["agent"] == nil {
		t.Fatal("agent workload missing")
	}
	if byKind["agent"].WorkloadType != "deployment" {
		t.Errorf("agent.workload_type: want deployment, got %q", byKind["agent"].WorkloadType)
	}
	if byKind["agent"].Replicas != 1 {
		t.Errorf("agent.replicas: want 1, got %d", byKind["agent"].Replicas)
	}

	// model (non-persistent) → Deployment
	if byKind["model"] == nil {
		t.Fatal("model workload missing")
	}
	if byKind["model"].WorkloadType != "deployment" {
		t.Errorf("model.workload_type: want deployment, got %q", byKind["model"].WorkloadType)
	}

	// persistent knowledge → StatefulSet
	dbWL := byName["research-assistant-knowledge-db"]
	if dbWL == nil {
		t.Fatal("knowledge db workload missing")
	}
	if dbWL.WorkloadType != "statefulset" {
		t.Errorf("knowledge.db.workload_type: want statefulset, got %q", dbWL.WorkloadType)
	}
	if !dbWL.Persistent {
		t.Error("knowledge.db: expected persistent=true")
	}

	// non-persistent knowledge → Deployment
	cacheWL := byName["research-assistant-knowledge-cache"]
	if cacheWL == nil {
		t.Fatal("knowledge cache workload missing")
	}
	if cacheWL.WorkloadType != "deployment" {
		t.Errorf("knowledge.cache.workload_type: want deployment, got %q", cacheWL.WorkloadType)
	}
	if cacheWL.Persistent {
		t.Error("knowledge.cache: expected persistent=false")
	}

	// ingestion schedule trigger → stored on workload row
	ingWL := byName["research-assistant-ingestion-nightly"]
	if ingWL == nil {
		t.Fatal("ingestion nightly workload missing")
	}
	if ingWL.TriggerType == nil || *ingWL.TriggerType != "schedule" {
		t.Errorf("ingestion.nightly.trigger_type: want schedule, got %v", ingWL.TriggerType)
	}

	// collector → Deployment
	if byKind["collector"] == nil {
		t.Fatal("collector workload missing")
	}
	if byKind["collector"].WorkloadType != "deployment" {
		t.Errorf("collector.workload_type: want deployment, got %q", byKind["collector"].WorkloadType)
	}

	// S2: persistent knowledge gets a volume row; non-persistent does not
	var volCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM deployment_volumes v
		JOIN deployment_workloads w ON v.workload_id = w.id
		WHERE w.deployment_id = $1`, depID).Scan(&volCount); err != nil {
		t.Fatalf("count volumes: %v", err)
	}
	if volCount != 1 {
		t.Errorf("volumes: want 1 (persistent db only), got %d", volCount)
	}

	// S3: services — each workload has at least one service
	services, err := store.GetServices(depID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}
	// agent(1) + model(1) + db(1) + cache(1) + ingestion(0, no port) + collector(2 grpc+http)
	if len(services) < 5 {
		t.Errorf("services: want at least 5, got %d", len(services))
	}
}

// ── S8–S13: variable targeting ────────────────────────────────────────────────

func TestSaveNormalizedSpec_BuildEnv_Targeting(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := targetingSpec()
	depID := saveSpec(t, store, accountID, "be-target", ds, nil)

	rows := buildEnvRows(t, db, depID)

	// S8: agent target → role='agent'
	if _, ok := rows["agent|AGENT_VAR"]; !ok {
		t.Error("S8: expected row (role=agent, env=AGENT_VAR)")
	}
	if _, ok := rows["messaging|AGENT_VAR"]; ok {
		t.Error("S8: AGENT_VAR must not appear in messaging role")
	}

	// S9: interface.slack target → role='messaging'
	if _, ok := rows["messaging|SLACK_TOKEN"]; !ok {
		t.Error("S9: expected row (role=messaging, env=SLACK_TOKEN)")
	}
	if r := rows["messaging|SLACK_TOKEN"]; !r.isSecret {
		t.Error("S9: SLACK_TOKEN row should have is_secret=true")
	}
	if _, ok := rows["agent|SLACK_TOKEN"]; ok {
		t.Error("S9: SLACK_TOKEN must not appear in agent role")
	}

	// S10: bare ingestion target fans out to both ingestions
	if _, ok := rows["ingestion:nightly|ALL_INGEST"]; !ok {
		t.Error("S10: expected row (role=ingestion:nightly, env=ALL_INGEST)")
	}
	if _, ok := rows["ingestion:hook|ALL_INGEST"]; !ok {
		t.Error("S10: expected row (role=ingestion:hook, env=ALL_INGEST)")
	}

	// S11: named ingestion target → only that ingestion, not the other
	if _, ok := rows["ingestion:nightly|NIGHTLY_ONLY"]; !ok {
		t.Error("S11: expected row (role=ingestion:nightly, env=NIGHTLY_ONLY)")
	}
	if _, ok := rows["ingestion:hook|NIGHTLY_ONLY"]; ok {
		t.Error("S11: NIGHTLY_ONLY must not appear in ingestion:hook role")
	}

	// S12: multi-target → one row per resolved role
	if _, ok := rows["agent|MULTI"]; !ok {
		t.Error("S12: expected row (role=agent, env=MULTI)")
	}
	if _, ok := rows["ingestion:hook|MULTI"]; !ok {
		t.Error("S12: expected row (role=ingestion:hook, env=MULTI)")
	}
	if _, ok := rows["ingestion:nightly|MULTI"]; ok {
		t.Error("S12: MULTI must not appear in ingestion:nightly (not in targets)")
	}

	// S13: optional flag stored on row
	if r, ok := rows["agent|OPTIONAL_VAR"]; !ok {
		t.Error("S13: expected row (role=agent, env=OPTIONAL_VAR)")
	} else if !r.optional {
		t.Error("S13: OPTIONAL_VAR row should have optional=true")
	}
	// Non-optional variable should have optional=false
	if r, ok := rows["agent|AGENT_VAR"]; ok && r.optional {
		t.Error("S13: AGENT_VAR row should have optional=false")
	}

	// NO_TARGETS variable must not produce any rows
	for k := range rows {
		if strings.HasSuffix(k, "NO_TARGETS") {
			t.Errorf("NO_TARGETS should produce no rows, found: %s", k)
		}
	}
}

// ── S15–S16: update-deploy clears + no duplicates ────────────────────────────

func TestSaveNormalizedSpec_BuildEnv_UpdateDeployClears(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	baseSpec := func(varName, varValue string) *spec.AstroDeploymentSpec {
		return &spec.AstroDeploymentSpec{
			Spec:   "deployment/v1",
			Source: spec.DeploymentSource{Name: "update-test", Build: "b1"},
			Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
			Variables: map[string]spec.Variable{
				varName: {Value: varValue, Targets: []string{"agent"}},
			},
		}
	}

	// First deploy: FIRST_VAR
	depID := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "update-test",
		BuildID: "b1", Namespace: "ns-update", SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, baseSpec("FIRST_VAR", "v1"), nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	if n := countBuildEnv(t, db, depID); n != 1 {
		t.Fatalf("after first save: want 1 row, got %d", n)
	}

	// S15: second deploy with SECOND_VAR — first save's rows must be cleared
	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "update-test",
		BuildID: "b2", Namespace: "ns-update", SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, baseSpec("SECOND_VAR", "v2"), nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}

	rows := buildEnvRows(t, db, depID)

	if _, ok := rows["agent|FIRST_VAR"]; ok {
		t.Error("S15: FIRST_VAR should have been cleared by the second deploy")
	}
	if _, ok := rows["agent|SECOND_VAR"]; !ok {
		t.Error("S15: SECOND_VAR should be present after second deploy")
	}

	// S16: no duplicates — exactly one row for SECOND_VAR
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM deployment_build_env WHERE deployment_id = $1 AND env_name = 'SECOND_VAR'`,
		depID,
	).Scan(&count); err != nil {
		t.Fatalf("count SECOND_VAR: %v", err)
	}
	if count != 1 {
		t.Errorf("S16: want exactly 1 row for SECOND_VAR, got %d", count)
	}
}

// ── R1–R4: GetBuildEnv ────────────────────────────────────────────────────────

func TestGetBuildEnv_ReturnsAllRows(t *testing.T) {
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

	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "get-be-test", Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Ingestion: map[string]spec.DeploymentIngestion{
			"sync": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "startup"}},
		},
		Variables: map[string]spec.Variable{
			// R2: non-secret stored as plaintext
			"PLAIN": {Value: "hello", Secret: false, Targets: []string{"agent"}},
			// R3: secret stored as ciphertext + nonce
			"SECRET": {Value: "top-secret", Secret: true, Targets: []string{"agent"}},
			// R4: multi-role variable — same user_var_name appears in two roles
			"SHARED": {Value: "shared-val", Targets: []string{"agent", "ingestion.sync"}},
		},
	}

	depID := newID()
	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "get-be-test",
		BuildID: "b1", Namespace: "ns-get-be", SpecJSON: `{}`,
		EncryptedDataKey: enc.EncryptedDataKey, KMSKeyARN: "arn:test",
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, ds, nil, enc, nil)
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	// R1: GetBuildEnv returns all rows
	rows, err := store.GetBuildEnv(depID)
	if err != nil {
		t.Fatalf("GetBuildEnv: %v", err)
	}
	// PLAIN(agent) + SECRET(agent) + SHARED(agent) + SHARED(ingestion:sync) = 4
	if len(rows) != 4 {
		t.Errorf("R1: want 4 rows, got %d", len(rows))
	}

	byKey := map[string]BuildEnvRow{}
	for _, r := range rows {
		byKey[r.Role+"|"+r.EnvName] = r
	}

	// R2: non-secret stored as plaintext bytes, no nonce
	plain := byKey["agent|PLAIN"]
	if string(plain.ValueEncrypted) != "hello" {
		t.Errorf("R2: PLAIN value: want \"hello\", got %q", string(plain.ValueEncrypted))
	}
	if len(plain.Nonce) != 0 {
		t.Error("R2: PLAIN nonce should be nil for non-secret")
	}
	if plain.IsSecret {
		t.Error("R2: PLAIN is_secret should be false")
	}

	// R3: secret stored as ciphertext + nonce; decrypts correctly
	secret := byKey["agent|SECRET"]
	if len(secret.Nonce) == 0 {
		t.Fatal("R3: SECRET nonce should be set")
	}
	if !secret.IsSecret {
		t.Error("R3: SECRET is_secret should be true")
	}
	dec, err := envelope.NewTestDecryptor(aesKey, enc.EncryptedDataKey)
	if err != nil {
		t.Fatalf("NewTestDecryptor: %v", err)
	}
	plaintext, err := dec.Decrypt(secret.ValueEncrypted, secret.Nonce)
	if err != nil {
		t.Fatalf("R3: decrypt SECRET: %v", err)
	}
	if string(plaintext) != "top-secret" {
		t.Errorf("R3: decrypted value: want \"top-secret\", got %q", string(plaintext))
	}

	// R4: multi-role variable appears in both roles; values are identical
	agentShared := byKey["agent|SHARED"]
	ingShared := byKey["ingestion:sync|SHARED"]
	if agentShared.UserVarName != "SHARED" {
		t.Errorf("R4: agent|SHARED user_var_name: want SHARED, got %q", agentShared.UserVarName)
	}
	if ingShared.UserVarName != "SHARED" {
		t.Errorf("R4: ingestion:sync|SHARED user_var_name: want SHARED, got %q", ingShared.UserVarName)
	}
	if string(agentShared.ValueEncrypted) != string(ingShared.ValueEncrypted) {
		t.Error("R4: multi-role variable values must be identical across roles")
	}
}

// ── V1–V3: GetDeploymentVariables role reconstruction ────────────────────────

func TestGetDeploymentVariables_RoleReconstruction(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "role-recon", Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Ingestion: map[string]spec.DeploymentIngestion{
			"nightly": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "startup"}},
		},
		Variables: map[string]spec.Variable{
			// V1: agent role → Targets=["agent"]
			"AGENT_VAR": {Value: "va", Targets: []string{"agent"}},
			// V2: messaging role → Targets=["interface.slack"]
			"SLACK_TOKEN": {Value: "xoxb-1", Targets: []string{"interface.slack"}},
			// V3: ingestion:nightly role → Targets=["ingestion.nightly"]
			"NIGHTLY_VAR": {Value: "vn", Targets: []string{"ingestion.nightly"}},
		},
	}

	depID := saveSpec(t, store, accountID, "role-recon", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}

	byName := map[string]Variable{}
	for _, v := range vars {
		byName[v.Name] = v
	}

	// V1: agent row → Targets=["agent"]
	agentVar, ok := byName["AGENT_VAR"]
	if !ok {
		t.Fatal("V1: AGENT_VAR not returned")
	}
	if !containsTarget(agentVar.Targets, "agent") {
		t.Errorf("V1: AGENT_VAR.Targets: want [agent], got %v", agentVar.Targets)
	}

	// V2: messaging row → Targets contains "interface.slack" or equivalent
	slackVar, ok := byName["SLACK_TOKEN"]
	if !ok {
		t.Fatal("V2: SLACK_TOKEN not returned")
	}
	hasInterface := false
	for _, tgt := range slackVar.Targets {
		if tgt == "interface.slack" || tgt == "interface" {
			hasInterface = true
		}
	}
	if !hasInterface {
		t.Errorf("V2: SLACK_TOKEN.Targets should include an interface target, got %v", slackVar.Targets)
	}

	// V3: ingestion:nightly row → Targets contains "ingestion.nightly"
	nightlyVar, ok := byName["NIGHTLY_VAR"]
	if !ok {
		t.Fatal("V3: NIGHTLY_VAR not returned")
	}
	if !containsTarget(nightlyVar.Targets, "ingestion.nightly") {
		t.Errorf("V3: NIGHTLY_VAR.Targets: want [ingestion.nightly], got %v", nightlyVar.Targets)
	}
}

func containsTarget(targets []string, want string) bool {
	for _, t := range targets {
		if t == want {
			return true
		}
	}
	return false
}

// sortStrings is a convenience for deterministic comparisons.
func sortStrings(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}
