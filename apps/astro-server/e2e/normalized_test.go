//go:build integration

package e2e

import (
	"database/sql"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
)

// sasbotSpecJSON is the real deployment spec from a running sasbot deployment.
const sasbotSpecJSON = `{"spec":"deployment/v1","source":{"account":"saswatds","name":"sasbot","build":"14f4c4dd","registry":"969403051954.dkr.ecr.us-east-1.amazonaws.com"},"target":{"runtime":"kubernetes","account":"saswatds","display_name":"Sasbot"},"agent":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot:14f4c4dd","endpoints":{"http":{"port":8080,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"environment":{"ANTHROPIC_API_KEY":"${variables.ANTHROPIC_API_KEY}","ASTRO_AGENT_BUILD":"${source.build}","ASTRO_AGENT_NAME":"${source.name}","CLOUDFLARE_ACCOUNT_ID":"${variables.CLOUDFLARE_ACCOUNT_ID}","CLOUDFLARE_AI_API_KEY":"${variables.CLOUDFLARE_AI_API_KEY}","EMBEDDING_DIMENSION":"768","EMBEDDING_MODEL":"nomic-embed-text","GITHUB_TOKEN":"${variables.GITHUB_TOKEN}","NEO4J_HOST":"${knowledge.graph.host}","NEO4J_PORT":"${knowledge.graph.http.port}","NEO4J_URL":"${knowledge.graph.http.url}","OLLAMA_BASE_URL":"${models.ollama.http.url}/api","OLLAMA_HOST":"${models.ollama.host}","OLLAMA_MODEL":"qwen3.5:2b","OLLAMA_PORT":"${models.ollama.http.port}","OLLAMA_URL":"${models.ollama.http.url}","QDRANT_HOST":"${knowledge.docs.host}","QDRANT_PORT":"${knowledge.docs.http.port}","QDRANT_URL":"${knowledge.docs.http.url}","REDIS_HOST":"${knowledge.cache.host}","REDIS_PORT":"${knowledge.cache.http.port}","REDIS_URL":"${knowledge.cache.http.url}"},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"}},"models":{"ollama":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ollama/ollama:latest","endpoints":{"http":{"port":11434,"protocol":"http"}},"replicas":1,"resources":{"cpu":"2","memory":"8Gi","cpu_limit":"4","memory_limit":"16Gi"},"gpu":{"runtime":"cuda","count":1},"environment":{"OLLAMA_HOST":"0.0.0.0","OLLAMA_KEEP_ALIVE":"-1","OLLAMA_MODEL":"qwen3.5:2b"},"healthcheck":{"test":["sh","-c","ollama list | grep -q 'qwen3.5:2b'"],"interval":"15s","timeout":"5s","retries":40},"update":{"strategy":"recreate"},"model":"qwen3.5:2b","persistent":true,"provider":"ollama"}},"knowledge":{"cache":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/redis:7-alpine","endpoints":{"http":{"port":6379,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":false,"healthcheck":{"test":["redis-cli","ping"]},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"},"provider":"redis"},"docs":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/qdrant/qdrant:latest","endpoints":{"grpc":{"port":6334,"protocol":"grpc"},"http":{"port":6333,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":true,"storage":{"size":"10Gi","access_mode":"ReadWriteOnce"},"healthcheck":{"path":"/healthz"},"update":{"strategy":"recreate"},"provider":"qdrant"},"graph":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/neo4j:5-community","endpoints":{"bolt":{"port":7687,"protocol":"tcp"},"http":{"port":7474,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":false,"environment":{"NEO4J_AUTH":"none"},"healthcheck":{"path":"/"},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"},"provider":"neo4j"}},"ingestion":{"webhook":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot-ingestion-webhook:14f4c4dd","endpoints":{"http":{"port":3001,"protocol":"http"}},"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"trigger":{"type":"webhook"}}},"interfaces":{"adapters":["web"],"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/astropods/messaging:latest","endpoints":{"grpc":{"port":9090,"protocol":"grpc"},"http":{"port":8080,"protocol":"http","expose":{"enabled":false}}},"resources":{"cpu":"100m","memory":"128Mi","cpu_limit":"500m","memory_limit":"512Mi"}},"variables":{"ANTHROPIC_API_KEY":{"targets":["agent"],"secret":true},"CLOUDFLARE_ACCOUNT_ID":{"targets":["agent"],"secret":true},"CLOUDFLARE_AI_API_KEY":{"targets":["agent"],"secret":true},"EMBEDDING_DIMENSION":{"value":"768","targets":["agent"],"optional":true},"EMBEDDING_MODEL":{"value":"nomic-embed-text","targets":["agent"],"optional":true},"GITHUB_TOKEN":{"targets":["agent"],"secret":true},"SLACK_APP_TOKEN":{"targets":["interface.slack"],"secret":true,"optional":true},"SLACK_BOT_TOKEN":{"targets":["interface.slack"],"secret":true,"optional":true}},"observability":{"enabled":true,"provider":"langfuse","image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/prod-astro-collector:latest","port":4318,"resources":{"cpu":"50m","memory":"128Mi","cpu_limit":"250m","memory_limit":"256Mi"}}}`

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
	return db
}

func ensureTestAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('test-e2e', 'personal')
		ON CONFLICT DO NOTHING RETURNING id
	`).Scan(&id)
	if err != nil {
		err = db.QueryRow(`SELECT id FROM accounts WHERE name = 'test-e2e'`).Scan(&id)
		if err != nil {
			t.Fatalf("failed to get test account: %v", err)
		}
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM deployments WHERE account_id = $1", id) })
	return id
}

func parseSasbotSpec(t *testing.T) *spec.AstroDeploymentSpec {
	t.Helper()
	var s spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(sasbotSpecJSON), &s); err != nil {
		t.Fatalf("failed to parse sasbot spec: %v", err)
	}
	return &s
}

func saveSasbot(t *testing.T, db *sql.DB, store *ds.Store, spec *spec.AstroDeploymentSpec, nsCfg *ds.NormalizedSpecConfig) *ds.Deployment {
	t.Helper()
	accountID := ensureTestAccount(t, db)
	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{},
		SecretData:    map[string]string{},
	}
	d, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: "sasbot",
		DisplayName: "Sasbot", BuildID: "14f4c4dd", Namespace: "ns-sasbot-e2e",
		SpecJSON: sasbotSpecJSON,
	}, func(tx *sql.Tx, depID string) error {
		return ds.SaveNormalizedSpec(tx, depID, spec, resolved, nil, nsCfg)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	return d
}

// --- Tests ---

func TestSasbot_Workloads(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nil)

	workloads, err := store.GetWorkloads(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}

	// agent + ollama + cache + docs + graph + webhook = 6 (sidecars in separate table)
	if len(workloads) != 6 {
		names := make([]string, len(workloads))
		for i, w := range workloads {
			names[i] = w.Name + " (" + w.ComponentKind + ")"
		}
		t.Fatalf("expected 6 workloads, got %d: %v", len(workloads), names)
	}

	byName := map[string]*ds.Workload{}
	for _, w := range workloads {
		byName[w.Name] = w
	}

	expect := map[string]struct{ kind, wtype string }{
		"sasbot-agent":             {"agent", "deployment"},
		"sasbot-model-ollama":      {"model", "statefulset"},
		"sasbot-knowledge-cache":   {"knowledge", "deployment"},
		"sasbot-knowledge-docs":    {"knowledge", "statefulset"},
		"sasbot-knowledge-graph":   {"knowledge", "deployment"},
		"sasbot-ingestion-webhook": {"ingestion", "deployment"},
	}
	for name, e := range expect {
		w := byName[name]
		if w == nil {
			t.Errorf("workload %q not found", name)
			continue
		}
		if w.ComponentKind != e.kind {
			t.Errorf("%s kind: got %q, want %q", name, w.ComponentKind, e.kind)
		}
		if w.WorkloadType != e.wtype {
			t.Errorf("%s type: got %q, want %q", name, w.WorkloadType, e.wtype)
		}
	}

	// GPU on ollama
	ollama := byName["sasbot-model-ollama"]
	if ollama.GPURuntime == nil || *ollama.GPURuntime != "cuda" {
		t.Errorf("ollama gpu_runtime: got %v, want 'cuda'", ollama.GPURuntime)
	}
	if ollama.GPUCount == nil || *ollama.GPUCount != 1 {
		t.Errorf("ollama gpu_count: got %v, want 1", ollama.GPUCount)
	}
	if !ollama.Persistent {
		t.Error("ollama should be persistent")
	}
}

func TestSasbot_Sidecars(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nil)

	sidecars, err := store.GetSidecars(d.ID)
	if err != nil {
		t.Fatalf("GetSidecars: %v", err)
	}
	if len(sidecars) != 2 {
		t.Fatalf("expected 2 sidecars, got %d", len(sidecars))
	}

	byName := map[string]*ds.Sidecar{}
	for _, sc := range sidecars {
		byName[sc.Name] = sc
	}

	msg := byName["sasbot-messaging"]
	if msg == nil {
		t.Fatal("sidecar 'sasbot-messaging' not found")
	}
	if msg.ComponentKind != "messaging" {
		t.Errorf("messaging kind: got %q", msg.ComponentKind)
	}
	if msg.CPULimit != "500m" || msg.MemoryLimit != "512Mi" {
		t.Errorf("messaging limits: cpu=%q mem=%q", msg.CPULimit, msg.MemoryLimit)
	}

	col := byName["sasbot-collector"]
	if col == nil {
		t.Fatal("sidecar 'sasbot-collector' not found")
	}
	if col.ComponentKind != "collector" {
		t.Errorf("collector kind: got %q", col.ComponentKind)
	}
}

func TestSasbot_Services(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nil)

	services, err := store.GetServices(d.ID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}

	// 12 total: agent(1) + ollama(1) + cache(1) + docs(2) + graph(2) + webhook(1) + messaging(2) + collector(2)
	if len(services) != 12 {
		for _, s := range services {
			t.Logf("  %s/%s port=%d", s.WorkloadName, s.Name, s.Port)
		}
		t.Fatalf("expected 12 services, got %d", len(services))
	}

	// Every service must have WorkloadName populated via join
	for _, svc := range services {
		if svc.WorkloadName == "" {
			t.Errorf("service %q port=%d has empty WorkloadName", svc.Name, svc.Port)
		}
	}

	// Verify per-workload service counts
	counts := map[string]int{}
	for _, svc := range services {
		counts[svc.WorkloadName]++
	}
	expectedCounts := map[string]int{
		"sasbot-agent": 1, "sasbot-model-ollama": 1, "sasbot-knowledge-cache": 1,
		"sasbot-knowledge-docs": 2, "sasbot-knowledge-graph": 2, "sasbot-ingestion-webhook": 1,
		"sasbot-messaging": 2, "sasbot-collector": 2,
	}
	for wl, want := range expectedCounts {
		if counts[wl] != want {
			t.Errorf("%s: expected %d services, got %d", wl, want, counts[wl])
		}
	}
}

func TestSasbot_Volumes(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nil)

	rows, err := db.Query(`
		SELECT dv.mount_path, dv.size, dv.access_mode, dw.name
		FROM deployment_volumes dv
		JOIN deployment_workloads dw ON dw.id = dv.workload_id
		WHERE dw.deployment_id = $1 ORDER BY dw.name
	`, d.ID)
	if err != nil {
		t.Fatalf("query volumes: %v", err)
	}
	defer rows.Close()

	type vol struct{ mountPath, size, accessMode, workload string }
	var vols []vol
	for rows.Next() {
		var v vol
		if err := rows.Scan(&v.mountPath, &v.size, &v.accessMode, &v.workload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		vols = append(vols, v)
	}

	// Only persistent: ollama (/root/.ollama, 50Gi) + qdrant (/qdrant/storage, 10Gi)
	if len(vols) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(vols))
	}
	byWL := map[string]vol{}
	for _, v := range vols {
		byWL[v.workload] = v
	}
	if v := byWL["sasbot-model-ollama"]; v.mountPath != "/root/.ollama" || v.size != "50Gi" {
		t.Errorf("ollama volume: mount=%q size=%q", v.mountPath, v.size)
	}
	if v := byWL["sasbot-knowledge-docs"]; v.mountPath != "/qdrant/storage" || v.size != "10Gi" {
		t.Errorf("docs volume: mount=%q size=%q", v.mountPath, v.size)
	}
}

func TestSasbot_Ingresses_WithoutDomain(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nil)

	ingresses, err := store.GetIngresses(d.ID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}
	if len(ingresses) != 0 {
		t.Errorf("expected 0 ingresses without ingressDomain, got %d", len(ingresses))
	}
}

func TestSasbot_Ingresses_WithDomain(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), &ds.NormalizedSpecConfig{
		Namespace:              "ns-sasbot-e2e",
		IngressDomain:          "agents.example.com",
		IngestionIngressDomain: "ingestion.example.com",
	})

	ingresses, err := store.GetIngresses(d.ID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}
	// messaging (web adapter) + ingestion webhook = 2
	if len(ingresses) != 2 {
		t.Fatalf("expected 2 ingresses, got %d", len(ingresses))
	}

	var hasAgents, hasIngestion bool
	for _, ing := range ingresses {
		if strings.Contains(ing.Hostname, "agents.example.com") {
			hasAgents = true
		}
		if strings.Contains(ing.Hostname, "ingestion.example.com") {
			hasIngestion = true
		}
		if !ing.TLSEnabled {
			t.Errorf("ingress %q should have TLS", ing.Hostname)
		}
	}
	if !hasAgents {
		t.Error("missing messaging ingress on agents.example.com")
	}
	if !hasIngestion {
		t.Error("missing webhook ingress on ingestion.example.com")
	}
}

func TestSasbot_Variables(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nil)

	vars, err := store.GetDeploymentVariables(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}
	if len(vars) != 8 {
		t.Fatalf("expected 8 variables, got %d", len(vars))
	}

	byName := map[string]ds.Variable{}
	for _, v := range vars {
		byName[v.Name] = v
	}

	// Required secrets (no value without encryptor)
	for _, name := range []string{"ANTHROPIC_API_KEY", "CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_AI_API_KEY", "GITHUB_TOKEN"} {
		v := byName[name]
		if !v.Secret {
			t.Errorf("%s should be secret", name)
		}
		if v.Optional {
			t.Errorf("%s should not be optional", name)
		}
	}

	// Optional secrets
	for _, name := range []string{"SLACK_APP_TOKEN", "SLACK_BOT_TOKEN"} {
		v := byName[name]
		if !v.Secret || !v.Optional {
			t.Errorf("%s: secret=%v optional=%v, want both true", name, v.Secret, v.Optional)
		}
	}

	// Non-secret defaults
	if v := byName["EMBEDDING_DIMENSION"]; v.Value != "768" || v.Secret || !v.Optional {
		t.Errorf("EMBEDDING_DIMENSION: value=%q secret=%v optional=%v", v.Value, v.Secret, v.Optional)
	}

	// Targets
	got := byName["SLACK_APP_TOKEN"].Targets
	sort.Strings(got)
	if len(got) != 1 || got[0] != "interface.slack" {
		t.Errorf("SLACK_APP_TOKEN targets: %v", got)
	}
}

func TestSasbot_SpecParse(t *testing.T) {
	s := parseSasbotSpec(t)

	if s.Source.Name != "sasbot" {
		t.Errorf("source.name: %q", s.Source.Name)
	}
	if len(s.Models) != 1 {
		t.Errorf("models: %d", len(s.Models))
	}
	if len(s.Knowledge) != 3 {
		t.Errorf("knowledge: %d", len(s.Knowledge))
	}
	if len(s.Ingestion) != 1 {
		t.Errorf("ingestion: %d", len(s.Ingestion))
	}
	if s.Interfaces == nil || len(s.Interfaces.Adapters) != 1 {
		t.Error("interfaces missing or wrong adapters")
	}
	if !s.Observability.Enabled {
		t.Error("observability should be enabled")
	}
	if len(s.Variables) != 8 {
		t.Errorf("variables: %d", len(s.Variables))
	}
}
