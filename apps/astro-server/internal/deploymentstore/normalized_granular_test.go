package deploymentstore

// normalized_granular_test.go — one test per S/R/V case from the deployment
// behaviour plan. Each test is minimal and self-contained so a failure names
// exactly which behaviour broke.
//
// All tests require DATABASE_URL and skip when it is unset.

import (
	"crypto/rand"
	"database/sql"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// ── workload helpers ──────────────────────────────────────────────────────────

func agentOnlySpec(name string) *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: name, Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1, Resources: spec.StandardResources, Update: spec.DefaultUpdateStrategy()},
	}
}

func workloadByKind(t *testing.T, store *Store, depID, kind string) *Workload {
	t.Helper()
	wls, err := store.GetWorkloads(depID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}
	for _, w := range wls {
		if w.ComponentKind == kind {
			return w
		}
	}
	return nil
}

func workloadByName(t *testing.T, store *Store, depID, name string) *Workload {
	t.Helper()
	wls, err := store.GetWorkloads(depID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}
	for _, w := range wls {
		if w.Name == name {
			return w
		}
	}
	return nil
}

// ── S1: workload types ────────────────────────────────────────────────────────

func TestSaveNormalizedSpec_S1_AgentIsDeployment(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s1-agent", agentOnlySpec("s1-agent"), nil)

	w := workloadByKind(t, store, depID, "agent")
	if w == nil {
		t.Fatal("agent workload not found")
	}
	if w.WorkloadType != "deployment" {
		t.Errorf("want deployment, got %q", w.WorkloadType)
	}
	if w.Image != "img:b1" {
		t.Errorf("image: want img:b1, got %q", w.Image)
	}
	if w.Replicas != 1 {
		t.Errorf("replicas: want 1, got %d", w.Replicas)
	}
}

func TestSaveNormalizedSpec_S1_NonPersistentKnowledgeIsDeployment(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s1-k-depl")
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"cache": {Image: "redis:7", Replicas: 1, Persistent: false, Endpoints: map[string]spec.Endpoint{"http": {Port: 6379}}, Update: spec.DefaultUpdateStrategy(), Resources: spec.StandardResources},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s1-k-depl", ds, nil)

	w := workloadByKind(t, store, depID, "knowledge")
	if w == nil {
		t.Fatal("knowledge workload not found")
	}
	if w.WorkloadType != "deployment" {
		t.Errorf("non-persistent knowledge: want deployment, got %q", w.WorkloadType)
	}
	if w.Persistent {
		t.Error("non-persistent knowledge: Persistent should be false")
	}
}

func TestSaveNormalizedSpec_S1_PersistentKnowledgeIsStatefulSet(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s1-k-sts")
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"db": {Image: "pgvector:17", Replicas: 1, Persistent: true, Storage: &spec.StorageConfig{Size: "10Gi", AccessMode: "ReadWriteOnce"}, Endpoints: map[string]spec.Endpoint{"http": {Port: 5432}}, Update: spec.UpdateStrategy{Strategy: "recreate"}, Resources: spec.StandardResources},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s1-k-sts", ds, nil)

	w := workloadByKind(t, store, depID, "knowledge")
	if w == nil {
		t.Fatal("knowledge workload not found")
	}
	if w.WorkloadType != "statefulset" {
		t.Errorf("persistent knowledge: want statefulset, got %q", w.WorkloadType)
	}
	if !w.Persistent {
		t.Error("persistent knowledge: Persistent should be true")
	}
}

func TestSaveNormalizedSpec_S1_IngestionTriggerTypeStored(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s1-ing")
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"nightly": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"}, Resources: spec.StandardResources},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s1-ing", ds, nil)

	w := workloadByKind(t, store, depID, "ingestion")
	if w == nil {
		t.Fatal("ingestion workload not found")
	}
	if w.TriggerType == nil || *w.TriggerType != "schedule" {
		t.Errorf("trigger_type: want schedule, got %v", w.TriggerType)
	}
	if w.TriggerSchedule == nil || *w.TriggerSchedule != "0 0 * * *" {
		t.Errorf("trigger_schedule: want '0 0 * * *', got %v", w.TriggerSchedule)
	}
}

func TestSaveNormalizedSpec_S1_CollectorIsDeployment(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s1-col")
	ds.Observability = spec.DeploymentObservability{Enabled: true, Image: "collector:latest", Port: 4318, Resources: spec.CollectorResources}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s1-col", ds, nil)

	w := workloadByKind(t, store, depID, "collector")
	if w == nil {
		t.Fatal("collector workload not found")
	}
	if w.WorkloadType != "deployment" {
		t.Errorf("collector: want deployment, got %q", w.WorkloadType)
	}
}

// ── S2: volumes ───────────────────────────────────────────────────────────────

func TestSaveNormalizedSpec_S2_PersistentKnowledgeGetsVolume(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s2-vol")
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"db": {Image: "pgvector:17", Replicas: 1, Persistent: true, Storage: &spec.StorageConfig{Size: "10Gi", AccessMode: "ReadWriteOnce"}, Endpoints: map[string]spec.Endpoint{"http": {Port: 5432}}, Update: spec.UpdateStrategy{Strategy: "recreate"}, Resources: spec.StandardResources},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s2-vol", ds, nil)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM deployment_volumes v JOIN deployment_workloads w ON v.workload_id = w.id WHERE w.deployment_id = $1`, depID).Scan(&n); err != nil {
		t.Fatalf("count volumes: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 volume for persistent knowledge, got %d", n)
	}
}

func TestSaveNormalizedSpec_S2_NonPersistentKnowledgeHasNoVolume(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s2-novol")
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"cache": {Image: "redis:7", Replicas: 1, Persistent: false, Endpoints: map[string]spec.Endpoint{"http": {Port: 6379}}, Update: spec.DefaultUpdateStrategy(), Resources: spec.StandardResources},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s2-novol", ds, nil)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM deployment_volumes v JOIN deployment_workloads w ON v.workload_id = w.id WHERE w.deployment_id = $1`, depID).Scan(&n); err != nil {
		t.Fatalf("count volumes: %v", err)
	}
	if n != 0 {
		t.Errorf("want 0 volumes for non-persistent knowledge, got %d", n)
	}
}

// ── S3: services ──────────────────────────────────────────────────────────────

func TestSaveNormalizedSpec_S3_ServiceCreatedPerEndpoint(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("s3-svc")
	ds.Observability = spec.DeploymentObservability{Enabled: true, Image: "collector:latest", Port: 4318, Resources: spec.CollectorResources}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s3-svc", ds, nil)

	services, err := store.GetServices(depID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}
	// agent(1 http) + collector(2 grpc+http) = 3
	if len(services) < 3 {
		t.Errorf("want at least 3 services (agent + collector grpc+http), got %d", len(services))
	}
}

// ── S8–S13: variable targeting (granular) ─────────────────────────────────────

func specWithIngestions(name string, vars map[string]spec.Variable) *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: name, Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Ingestion: map[string]spec.DeploymentIngestion{
			"nightly": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "schedule"}},
			"hook":    {Image: "hook:b1", Trigger: spec.DeploymentTrigger{Type: "webhook"}},
		},
		Variables: vars,
	}
}

func TestSaveNormalizedSpec_S8_AgentTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("s8", map[string]spec.Variable{
		"MY_VAR": {Value: "v", Targets: []string{"agent"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s8", ds, nil)
	rows := buildEnvRows(t, db, depID)

	if _, ok := rows["agent|MY_VAR"]; !ok {
		t.Error("S8: expected row with role=agent")
	}
	if _, ok := rows["messaging|MY_VAR"]; ok {
		t.Error("S8: MY_VAR must not appear in messaging role")
	}
}

func TestSaveNormalizedSpec_S9_MessagingTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("s9", map[string]spec.Variable{
		"SLACK_TOKEN": {Value: "xoxb", Secret: true, Targets: []string{"interface.slack"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s9", ds, nil)
	rows := buildEnvRows(t, db, depID)

	r, ok := rows["messaging|SLACK_TOKEN"]
	if !ok {
		t.Fatal("S9: expected row with role=messaging")
	}
	if !r.isSecret {
		t.Error("S9: is_secret should be true")
	}
	if _, ok := rows["agent|SLACK_TOKEN"]; ok {
		t.Error("S9: SLACK_TOKEN must not appear in agent role")
	}
}

func TestSaveNormalizedSpec_S10_IngestionFanOut(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("s10", map[string]spec.Variable{
		"SHARED": {Value: "v", Targets: []string{"ingestion"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s10", ds, nil)
	rows := buildEnvRows(t, db, depID)

	if _, ok := rows["ingestion:nightly|SHARED"]; !ok {
		t.Error("S10: expected row role=ingestion:nightly")
	}
	if _, ok := rows["ingestion:hook|SHARED"]; !ok {
		t.Error("S10: expected row role=ingestion:hook")
	}
	if _, ok := rows["agent|SHARED"]; ok {
		t.Error("S10: SHARED must not fan out to agent")
	}
}

func TestSaveNormalizedSpec_S11_NamedIngestionTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("s11", map[string]spec.Variable{
		"NIGHTLY_VAR": {Value: "v", Targets: []string{"ingestion.nightly"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s11", ds, nil)
	rows := buildEnvRows(t, db, depID)

	if _, ok := rows["ingestion:nightly|NIGHTLY_VAR"]; !ok {
		t.Error("S11: expected row role=ingestion:nightly")
	}
	if _, ok := rows["ingestion:hook|NIGHTLY_VAR"]; ok {
		t.Error("S11: must not appear in ingestion:hook")
	}
}

func TestSaveNormalizedSpec_S12_MultiTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("s12", map[string]spec.Variable{
		"MULTI": {Value: "v", Targets: []string{"agent", "ingestion.hook"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s12", ds, nil)
	rows := buildEnvRows(t, db, depID)

	if _, ok := rows["agent|MULTI"]; !ok {
		t.Error("S12: expected row role=agent")
	}
	if _, ok := rows["ingestion:hook|MULTI"]; !ok {
		t.Error("S12: expected row role=ingestion:hook")
	}
	if _, ok := rows["ingestion:nightly|MULTI"]; ok {
		t.Error("S12: must not appear in ingestion:nightly (not in targets)")
	}
}

func TestSaveNormalizedSpec_S13_OptionalFlagStoredOnRow(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("s13", map[string]spec.Variable{
		"OPT_VAR": {Value: "v1", Optional: true, Targets: []string{"agent"}},
		"REQ_VAR": {Value: "v2", Optional: false, Targets: []string{"agent"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "s13", ds, nil)
	rows := buildEnvRows(t, db, depID)

	if r, ok := rows["agent|OPT_VAR"]; !ok {
		t.Fatal("S13: OPT_VAR row missing")
	} else if !r.optional {
		t.Error("S13: OPT_VAR optional should be true")
	}
	if r, ok := rows["agent|REQ_VAR"]; !ok {
		t.Fatal("S13: REQ_VAR row missing")
	} else if r.optional {
		t.Error("S13: REQ_VAR optional should be false")
	}
}

// ── S15–S16: update-deploy (granular) ────────────────────────────────────────

func TestSaveNormalizedSpec_S15_UpdateDeployClears(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	save := func(varName string) string {
		depID := newID()
		_, err := store.SaveDeploymentPending(SaveDeploymentParams{
			ID: depID, AccountID: accountID, AgentName: "s15",
			BuildID: "b1", Namespace: "ns-s15", SpecJSON: `{}`,
		}, func(tx *sql.Tx, id string) error {
			return SaveNormalizedSpec(tx, id, &spec.AstroDeploymentSpec{
				Spec:   "deployment/v1",
				Source: spec.DeploymentSource{Name: "s15", Build: "b1"},
				Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
				Variables: map[string]spec.Variable{
					varName: {Value: "v", Targets: []string{"agent"}},
				},
			}, nil, nil, nil)
		})
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		return depID
	}

	depID := save("FIRST_VAR")
	depID = save("SECOND_VAR") // same deployment ID not possible via SaveDeploymentPending for a new spec, so save separately
	_ = depID

	// Use the targeting spec approach: save, then save again with same depID via direct tx
	depID2 := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID2, AccountID: accountID, AgentName: "s15b",
		BuildID: "b1", Namespace: "ns-s15b", SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, &spec.AstroDeploymentSpec{
			Spec:   "deployment/v1",
			Source: spec.DeploymentSource{Name: "s15b", Build: "b1"},
			Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
			Variables: map[string]spec.Variable{
				"BEFORE": {Value: "v1", Targets: []string{"agent"}},
			},
		}, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Second call to SaveNormalizedSpec on same deployment (simulating re-deploy)
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	err = SaveNormalizedSpec(tx, depID2, &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "s15b", Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Variables: map[string]spec.Variable{
			"AFTER": {Value: "v2", Targets: []string{"agent"}},
		},
	}, nil, nil, nil)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("second SaveNormalizedSpec: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	rows := buildEnvRows(t, db, depID2)
	if _, ok := rows["agent|BEFORE"]; ok {
		t.Error("S15: BEFORE should have been cleared")
	}
	if _, ok := rows["agent|AFTER"]; !ok {
		t.Error("S15: AFTER should be present")
	}
}

func TestSaveNormalizedSpec_S16_NoDuplicateRows(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	depID := newID()
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "s16",
		BuildID: "b1", Namespace: "ns-s16", SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, &spec.AstroDeploymentSpec{
			Spec:   "deployment/v1",
			Source: spec.DeploymentSource{Name: "s16", Build: "b1"},
			Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
			Variables: map[string]spec.Variable{
				"MY_VAR": {Value: "v", Targets: []string{"agent"}},
			},
		}, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM deployment_build_env WHERE deployment_id = $1 AND env_name = 'MY_VAR'`,
		depID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("S16: want exactly 1 row for MY_VAR, got %d", count)
	}
}

// ── R1–R4: GetBuildEnv (granular) ────────────────────────────────────────────

func TestGetBuildEnv_R1_AllRowsReturned(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("r1", map[string]spec.Variable{
		"A": {Value: "1", Targets: []string{"agent"}},
		"B": {Value: "2", Targets: []string{"ingestion.nightly"}},
		"C": {Value: "3", Targets: []string{"ingestion.hook"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "r1", ds, nil)

	rows, err := store.GetBuildEnv(depID)
	if err != nil {
		t.Fatalf("GetBuildEnv: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("R1: want 3 rows, got %d", len(rows))
	}
}

func TestGetBuildEnv_R2_PlaintextStoredWithoutNonce(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("r2")
	ds.Variables = map[string]spec.Variable{
		"PLAIN": {Value: "hello", Secret: false, Targets: []string{"agent"}},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "r2", ds, nil)

	rows, err := store.GetBuildEnv(depID)
	if err != nil {
		t.Fatalf("GetBuildEnv: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if string(r.ValueEncrypted) != "hello" {
		t.Errorf("R2: value: want hello, got %q", string(r.ValueEncrypted))
	}
	if len(r.Nonce) != 0 {
		t.Error("R2: nonce should be nil for non-secret")
	}
}

func TestGetBuildEnv_R3_SecretStoredAsCiphertextWithNonce(t *testing.T) {
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

	ds := agentOnlySpec("r3")
	ds.Variables = map[string]spec.Variable{
		"SECRET": {Value: "top-secret", Secret: true, Targets: []string{"agent"}},
	}

	depID := newID()
	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "r3",
		BuildID: "b1", Namespace: "ns-r3", SpecJSON: `{}`,
		EncryptedDataKey: enc.EncryptedDataKey, KMSKeyARN: "arn:test",
	}, func(tx *sql.Tx, id string) error {
		return SaveNormalizedSpec(tx, id, ds, nil, enc, nil)
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	rows, err := store.GetBuildEnv(depID)
	if err != nil {
		t.Fatalf("GetBuildEnv: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if len(r.Nonce) == 0 {
		t.Fatal("R3: nonce must be set for encrypted secret")
	}
	if !r.IsSecret {
		t.Error("R3: is_secret should be true")
	}

	dec, err := envelope.NewTestDecryptor(aesKey, enc.EncryptedDataKey)
	if err != nil {
		t.Fatalf("NewTestDecryptor: %v", err)
	}
	plain, err := dec.Decrypt(r.ValueEncrypted, r.Nonce)
	if err != nil {
		t.Fatalf("R3: decrypt: %v", err)
	}
	if string(plain) != "top-secret" {
		t.Errorf("R3: decrypted: want top-secret, got %q", string(plain))
	}
}

func TestGetBuildEnv_R4_MultiRoleRowsHaveIdenticalValues(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := specWithIngestions("r4", map[string]spec.Variable{
		"SHARED": {Value: "same-val", Targets: []string{"agent", "ingestion.nightly"}},
	})
	depID := saveSpec(t, store, ensureTestAccount(t, db), "r4", ds, nil)

	rows, err := store.GetBuildEnv(depID)
	if err != nil {
		t.Fatalf("GetBuildEnv: %v", err)
	}

	byRole := map[string]BuildEnvRow{}
	for _, r := range rows {
		if r.EnvName == "SHARED" {
			byRole[r.Role] = r
		}
	}
	if len(byRole) != 2 {
		t.Fatalf("R4: want 2 rows for SHARED (agent + ingestion:nightly), got %d", len(byRole))
	}
	agentVal := string(byRole["agent"].ValueEncrypted)
	ingVal := string(byRole["ingestion:nightly"].ValueEncrypted)
	if agentVal != ingVal {
		t.Errorf("R4: values must be identical across roles: agent=%q ingestion=%q", agentVal, ingVal)
	}
}

// ── V1–V3: GetDeploymentVariables role reconstruction (granular) ──────────────

func TestGetDeploymentVariables_V1_AgentRoleReconstructsTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("v1")
	ds.Variables = map[string]spec.Variable{
		"AGENT_VAR": {Value: "v", Targets: []string{"agent"}},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "v1", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	for _, v := range vars {
		if v.Name == "AGENT_VAR" {
			if !containsTarget(v.Targets, "agent") {
				t.Errorf("V1: Targets=%v, want to contain agent", v.Targets)
			}
			return
		}
	}
	t.Fatal("V1: AGENT_VAR not returned")
}

func TestGetDeploymentVariables_V2_MessagingRoleReconstructsTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("v2")
	ds.Variables = map[string]spec.Variable{
		"SLACK_TOKEN": {Value: "xoxb", Targets: []string{"interface.slack"}},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "v2", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	for _, v := range vars {
		if v.Name == "SLACK_TOKEN" {
			found := false
			for _, tgt := range v.Targets {
				if tgt == "interface.slack" || tgt == "interface" {
					found = true
				}
			}
			if !found {
				t.Errorf("V2: Targets=%v, want to contain interface target", v.Targets)
			}
			return
		}
	}
	t.Fatal("V2: SLACK_TOKEN not returned")
}

func TestGetDeploymentVariables_V3_IngestionRoleReconstructsTarget(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "v3", Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Ingestion: map[string]spec.DeploymentIngestion{
			"nightly": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "startup"}},
		},
		Variables: map[string]spec.Variable{
			"NIGHTLY_VAR": {Value: "v", Targets: []string{"ingestion.nightly"}},
		},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "v3", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	for _, v := range vars {
		if v.Name == "NIGHTLY_VAR" {
			if !containsTarget(v.Targets, "ingestion.nightly") {
				t.Errorf("V3: Targets=%v, want ingestion.nightly", v.Targets)
			}
			return
		}
	}
	t.Fatal("V3: NIGHTLY_VAR not returned")
}

// ── Collector workload ────────────────────────────────────────────────────────

func TestSaveNormalizedSpec_CollectorWorkloadCreated(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("col-created")
	ds.Observability = spec.DeploymentObservability{
		Enabled:   true,
		Image:     "collector:latest",
		Port:      4318,
		Resources: spec.CollectorResources,
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "col-created", ds, nil)

	w := workloadByKind(t, store, depID, "collector")
	if w == nil {
		t.Fatal("collector workload not found in deployment_workloads")
	}
	if w.WorkloadType != "deployment" {
		t.Errorf("collector workload_type: want deployment, got %q", w.WorkloadType)
	}
	if w.Image != "collector:latest" {
		t.Errorf("collector image: want collector:latest, got %q", w.Image)
	}
	if w.CPURequest != spec.CollectorResources.CPU {
		t.Errorf("collector cpu_request: want %s, got %s", spec.CollectorResources.CPU, w.CPURequest)
	}
}

func TestSaveNormalizedSpec_NoCollectorWorkloadWhenDisabled(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("col-disabled")
	ds.Observability = spec.DeploymentObservability{Enabled: false}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "col-disabled", ds, nil)

	w := workloadByKind(t, store, depID, "collector")
	if w != nil {
		t.Error("collector workload must not be created when observability is disabled")
	}
}

// ── Messaging sidecar workload ────────────────────────────────────────────────

// When interfaces are configured with at least one adapter, SaveNormalizedSpec
// must write a sidecar row with component_kind='messaging'. Without an adapter
// selected, no sidecar row should be created.
func TestSaveNormalizedSpec_MessagingSidecarCreated(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := agentOnlySpec("msg-sidecar")
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
		Resources: spec.MessagingResources,
	}
	depID := saveSpec(t, store, accountID, "msg-sidecar", ds, nil)

	sidecars, err := store.GetSidecars(depID)
	if err != nil {
		t.Fatalf("GetSidecars: %v", err)
	}

	var msgSidecar *Sidecar
	for _, sc := range sidecars {
		if sc.ComponentKind == "messaging" {
			msgSidecar = sc
			break
		}
	}
	if msgSidecar == nil {
		t.Fatal("messaging sidecar not found in deployment_sidecars")
	}
	if msgSidecar.Image != "messaging:latest" {
		t.Errorf("messaging sidecar image: want messaging:latest, got %q", msgSidecar.Image)
	}
	if msgSidecar.CPURequest != spec.MessagingResources.CPU {
		t.Errorf("messaging sidecar cpu_request: want %s, got %s", spec.MessagingResources.CPU, msgSidecar.CPURequest)
	}
}

func TestSaveNormalizedSpec_NoMessagingSidecarWithoutAdapters(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := agentOnlySpec("no-msg-sidecar")
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{}, // no adapter selected
		Image:    "messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
		},
		Resources: spec.MessagingResources,
	}
	depID := saveSpec(t, store, accountID, "no-msg-sidecar", ds, nil)

	sidecars, err := store.GetSidecars(depID)
	if err != nil {
		t.Fatalf("GetSidecars: %v", err)
	}
	for _, sc := range sidecars {
		if sc.ComponentKind == "messaging" {
			t.Error("messaging sidecar must not be created when no adapter is selected")
		}
	}
}

// ── R5: empty GetBuildEnv ─────────────────────────────────────────────────────

func TestGetBuildEnv_R5_EmptyDeploymentReturnsNilNotError(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	depID := saveSpec(t, store, ensureTestAccount(t, db), "r5-empty", agentOnlySpec("r5-empty"), nil)

	rows, err := store.GetBuildEnv(depID)
	if err != nil {
		t.Fatalf("R5: GetBuildEnv should not error for a deployment with no variables, got: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("R5: expected 0 rows, got %d", len(rows))
	}
}

// ── V5–V7: GetDeploymentVariables metadata preservation ──────────────────────

func TestGetDeploymentVariables_V5_MultiTargetReconstructsAllTargets(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "v5", Build: "b1"},
		Agent:  spec.DeploymentAgent{Image: "img:b1", Endpoints: map[string]spec.Endpoint{"http": {Port: 8080}}, Replicas: 1},
		Ingestion: map[string]spec.DeploymentIngestion{
			"nightly": {Image: "sync:b1", Trigger: spec.DeploymentTrigger{Type: "startup"}},
		},
		Variables: map[string]spec.Variable{
			"SHARED_VAR": {Value: "v", Targets: []string{"agent", "ingestion.nightly"}},
		},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "v5", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	for _, v := range vars {
		if v.Name != "SHARED_VAR" {
			continue
		}
		hasAgent := containsTarget(v.Targets, "agent")
		hasIngestion := false
		for _, tgt := range v.Targets {
			if tgt == "ingestion.nightly" || tgt == "ingestion" {
				hasIngestion = true
			}
		}
		if !hasAgent {
			t.Errorf("V5: SHARED_VAR.Targets missing agent: %v", v.Targets)
		}
		if !hasIngestion {
			t.Errorf("V5: SHARED_VAR.Targets missing ingestion target: %v", v.Targets)
		}
		return
	}
	t.Fatal("V5: SHARED_VAR not returned")
}

func TestGetDeploymentVariables_V6_SecretFlagPreserved(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("v6")
	ds.Variables = map[string]spec.Variable{
		"SECRET_VAR": {Value: "s", Secret: true, Targets: []string{"agent"}},
		"PLAIN_VAR":  {Value: "p", Secret: false, Targets: []string{"agent"}},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "v6", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	byName := map[string]Variable{}
	for _, v := range vars {
		byName[v.Name] = v
	}
	if !byName["SECRET_VAR"].Secret {
		t.Error("V6: SECRET_VAR.Secret should be true")
	}
	if byName["PLAIN_VAR"].Secret {
		t.Error("V6: PLAIN_VAR.Secret should be false")
	}
}

func TestGetDeploymentVariables_V7_OptionalFlagPreserved(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ds := agentOnlySpec("v7")
	ds.Variables = map[string]spec.Variable{
		"OPT_VAR": {Value: "v", Optional: true, Targets: []string{"agent"}},
		"REQ_VAR": {Value: "v", Optional: false, Targets: []string{"agent"}},
	}
	depID := saveSpec(t, store, ensureTestAccount(t, db), "v7", ds, nil)

	vars, err := store.GetDeploymentVariables(depID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	byName := map[string]Variable{}
	for _, v := range vars {
		byName[v.Name] = v
	}
	if !byName["OPT_VAR"].Optional {
		t.Error("V7: OPT_VAR.Optional should be true")
	}
	if byName["REQ_VAR"].Optional {
		t.Error("V7: REQ_VAR.Optional should be false")
	}
}
