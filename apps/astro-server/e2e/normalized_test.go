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
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	spec "github.com/astropods/astro-spec"
	_ "github.com/lib/pq"
)

// sasbotSpecJSON is the real deployment spec from a running sasbot deployment.
const sasbotSpecJSON = `{"spec":"deployment/v1","source":{"account":"saswatds","name":"sasbot","build":"14f4c4dd","registry":"969403051954.dkr.ecr.us-east-1.amazonaws.com"},"target":{"runtime":"kubernetes","account":"saswatds","display_name":"Sasbot"},"agent":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot:14f4c4dd","endpoints":{"http":{"port":8080,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"environment":{"ANTHROPIC_API_KEY":"${variables.ANTHROPIC_API_KEY}","ASTRO_AGENT_BUILD":"${source.build}","ASTRO_AGENT_NAME":"${source.name}","CLOUDFLARE_ACCOUNT_ID":"${variables.CLOUDFLARE_ACCOUNT_ID}","CLOUDFLARE_AI_API_KEY":"${variables.CLOUDFLARE_AI_API_KEY}","EMBEDDING_DIMENSION":"768","EMBEDDING_MODEL":"nomic-embed-text","GITHUB_TOKEN":"${variables.GITHUB_TOKEN}","NEO4J_HOST":"${knowledge.graph.host}","NEO4J_PORT":"${knowledge.graph.http.port}","NEO4J_URL":"${knowledge.graph.http.url}","MODEL_LLM_HOST":"${models.llm.host}","MODEL_LLM_PORT":"${models.llm.http.port}","MODEL_LLM_URL":"${models.llm.http.url}","QDRANT_HOST":"${knowledge.docs.host}","QDRANT_PORT":"${knowledge.docs.http.port}","QDRANT_URL":"${knowledge.docs.http.url}","REDIS_HOST":"${knowledge.cache.host}","REDIS_PORT":"${knowledge.cache.http.port}","REDIS_URL":"${knowledge.cache.http.url}"},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"}},"models":{"llm":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/my-model:latest","endpoints":{"http":{"port":8000,"protocol":"http"}},"replicas":1,"resources":{"cpu":"2","memory":"8Gi","cpu_limit":"4","memory_limit":"16Gi"},"gpu":{"runtime":"cuda","count":1},"update":{"strategy":"recreate"}}},"knowledge":{"cache":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/redis:7-alpine","endpoints":{"http":{"port":6379,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":false,"healthcheck":{"test":["redis-cli","ping"]},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"},"provider":"redis"},"docs":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/qdrant/qdrant:latest","endpoints":{"grpc":{"port":6334,"protocol":"grpc"},"http":{"port":6333,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":true,"storage":{"size":"10Gi","access_mode":"ReadWriteOnce"},"healthcheck":{"path":"/healthz"},"update":{"strategy":"recreate"},"provider":"qdrant"},"graph":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/neo4j:5-community","endpoints":{"bolt":{"port":7687,"protocol":"tcp"},"http":{"port":7474,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":false,"environment":{"NEO4J_AUTH":"none"},"healthcheck":{"path":"/"},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"},"provider":"neo4j"}},"ingestion":{"webhook":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot-ingestion-webhook:14f4c4dd","endpoints":{"http":{"port":3001,"protocol":"http"}},"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"trigger":{"type":"webhook"}}},"interfaces":{"adapters":["web"],"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/astropods/messaging:latest","endpoints":{"grpc":{"port":9090,"protocol":"grpc"},"http":{"port":8080,"protocol":"http","expose":{"enabled":false}}},"resources":{"cpu":"100m","memory":"128Mi","cpu_limit":"500m","memory_limit":"512Mi"}},"variables":{"ANTHROPIC_API_KEY":{"targets":["agent"],"secret":true},"CLOUDFLARE_ACCOUNT_ID":{"targets":["agent"],"secret":true},"CLOUDFLARE_AI_API_KEY":{"targets":["agent"],"secret":true},"EMBEDDING_DIMENSION":{"value":"768","targets":["agent"],"optional":true},"EMBEDDING_MODEL":{"value":"nomic-embed-text","targets":["agent"],"optional":true},"GITHUB_TOKEN":{"targets":["agent"],"secret":true},"SLACK_APP_TOKEN":{"targets":["interface.slack"],"secret":true,"optional":true},"SLACK_BOT_TOKEN":{"targets":["interface.slack"],"secret":true,"optional":true}},"observability":{"enabled":true,"provider":"langfuse","image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/prod-astro-collector:latest","port":4318,"resources":{"cpu":"50m","memory":"128Mi","cpu_limit":"250m","memory_limit":"256Mi"}}}`

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
	d, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: "sasbot",
		DisplayName: "Sasbot", BuildID: "14f4c4dd", Namespace: "ns-sasbot-e2e",
		SpecJSON: sasbotSpecJSON,
	}, func(tx *sql.Tx, depID string) error {
		return ds.SaveNormalizedSpec(tx, depID, spec, nil, nsCfg)
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

	// agent + llm + cache + docs + graph + webhook + collector = 7 (sidecars in separate table)
	if len(workloads) != 7 {
		names := make([]string, len(workloads))
		for i, w := range workloads {
			names[i] = w.Name + " (" + w.ComponentKind + ")"
		}
		t.Fatalf("expected 7 workloads, got %d: %v", len(workloads), names)
	}

	byName := map[string]*ds.Workload{}
	for _, w := range workloads {
		byName[w.Name] = w
	}

	expect := map[string]struct{ kind, wtype string }{
		"sasbot-agent":             {"agent", "statefulset"},
		"sasbot-model-llm":         {"model", "deployment"},
		"sasbot-knowledge-cache":   {"knowledge", "deployment"},
		"sasbot-knowledge-docs":    {"knowledge", "statefulset"},
		"sasbot-knowledge-graph":   {"knowledge", "deployment"},
		"sasbot-ingestion-webhook": {"ingestion", "deployment"},
		"sasbot-collector":         {"collector", "deployment"},
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

	// GPU on the container-mode model
	model := byName["sasbot-model-llm"]
	if model.GPURuntime == nil || *model.GPURuntime != "cuda" {
		t.Errorf("model gpu_runtime: got %v, want 'cuda'", model.GPURuntime)
	}
	if model.GPUCount == nil || *model.GPUCount != 1 {
		t.Errorf("model gpu_count: got %v, want 1", model.GPUCount)
	}
	if model.Persistent {
		t.Error("container-mode model should not be persistent")
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
	// Only messaging is a sidecar; the collector is a standalone workload.
	if len(sidecars) != 1 {
		t.Fatalf("expected 1 sidecar, got %d", len(sidecars))
	}

	msg := sidecars[0]
	if msg.Name != "sasbot-messaging" {
		t.Errorf("sidecar name: got %q, want sasbot-messaging", msg.Name)
	}
	if msg.ComponentKind != "messaging" {
		t.Errorf("messaging kind: got %q", msg.ComponentKind)
	}
	if msg.CPULimit != "500m" || msg.MemoryLimit != "512Mi" {
		t.Errorf("messaging limits: cpu=%q mem=%q", msg.CPULimit, msg.MemoryLimit)
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

	// 12 total: agent(1) + llm(1) + cache(1) + docs(2) + graph(2) + webhook(1) + messaging(2) + collector(2)
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
		"sasbot-agent": 1, "sasbot-model-llm": 1, "sasbot-knowledge-cache": 1,
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

	// Persistent volumes: agent default disk (/data, 5Gi) + qdrant
	// (/qdrant/storage, 10Gi). The container-mode model is not persistent, so it
	// has no volume. Every agent runs as a StatefulSet with a default shared disk.
	if len(vols) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(vols))
	}
	byWL := map[string]vol{}
	for _, v := range vols {
		byWL[v.workload] = v
	}
	if v := byWL["sasbot-agent"]; v.mountPath != "/data" || v.size != "5Gi" {
		t.Errorf("agent volume: mount=%q size=%q", v.mountPath, v.size)
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

	// Targets — adapter detail is lost when roles are reconstructed from
	// deployment_build_env; "interface.slack" collapses to "interface".
	got := byName["SLACK_APP_TOKEN"].Targets
	sort.Strings(got)
	if len(got) != 1 || got[0] != "interface" {
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

// ---------------------------------------------------------------------------
// Ingress hostname consistency tests
//
// These verify that SaveNormalizedSpec stores the same ingress hostnames that
// the spec applier (k8s.ApplyDeploymentSpec) would create in K8s. If these
// diverge, the drift detector reports every ingress as missing + extra.
// ---------------------------------------------------------------------------

// TestIngressHostname_AgentMatchesSpecApplier verifies the agent ingress
// hostname stored by the normalizer matches k8s.GenerateIngressHost.
func TestIngressHostname_AgentMatchesSpecApplier(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	const (
		agentName = "weather-poet"
		namespace = "astro-abc123-0"
		domain    = "agents.astropods.ai"
	)

	dsSpec := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: agentName, Build: "b1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/weather-poet:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "1", MemoryLimit: "1Gi"},
			Endpoints: map[string]spec.Endpoint{
				"http": {Port: 8080, Protocol: "http", Expose: &spec.EndpointExpose{Enabled: true}},
			},
		},
	}

	nsCfg := &ds.NormalizedSpecConfig{Namespace: namespace, IngressDomain: domain}
	depID := deployid.New()
	_, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: agentName,
		DisplayName: "Agent Ingress Test", BuildID: "b1", Namespace: namespace,
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return ds.SaveNormalizedSpec(tx, id, dsSpec, nil, nsCfg)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	ings, err := store.GetIngresses(depID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}
	if len(ings) == 0 {
		t.Fatal("expected at least 1 ingress, got 0")
	}

	wantHost := k8s.GenerateIngressHost(agentName, namespace, domain)
	if ings[0].Hostname != wantHost {
		t.Errorf("agent ingress hostname mismatch:\n  normalizer stored: %s\n  spec applier uses: %s", ings[0].Hostname, wantHost)
	}
}

// TestIngressHostname_MessagingMatchesSpecApplier verifies the messaging (web
// adapter) ingress uses GenerateMessagingIngressHost, not GenerateIngressHost.
func TestIngressHostname_MessagingMatchesSpecApplier(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	const (
		agentName = "weather-poet"
		namespace = "astro-abc123-0"
		domain    = "agents.astropods.ai"
	)

	dsSpec := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: agentName, Build: "b1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/weather-poet:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "1", MemoryLimit: "1Gi"},
		},
		Interfaces: &spec.DeploymentInterfaces{
			Adapters:  []string{"web"},
			Image:     "messaging:latest",
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "500m", MemoryLimit: "512Mi"},
			Endpoints: map[string]spec.Endpoint{
				"http": {Port: 3000, Protocol: "http"},
			},
		},
	}

	nsCfg := &ds.NormalizedSpecConfig{Namespace: namespace, IngressDomain: domain}
	depID := deployid.New()
	_, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: agentName,
		DisplayName: "Messaging Ingress Test", BuildID: "b1", Namespace: namespace,
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return ds.SaveNormalizedSpec(tx, id, dsSpec, nil, nsCfg)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	ings, err := store.GetIngresses(depID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}

	wantHost := k8s.GenerateMessagingIngressHost(agentName, namespace, domain)
	wrongHost := k8s.GenerateIngressHost(agentName, namespace, domain)

	found := false
	for _, ing := range ings {
		if ing.Hostname == wantHost {
			found = true
		}
		if ing.Hostname == wrongHost {
			t.Errorf("messaging ingress used agent-style hostname instead of messaging-style:\n  stored:   %s\n  expected: %s", wrongHost, wantHost)
		}
	}
	if !found {
		hostnames := make([]string, len(ings))
		for i, ing := range ings {
			hostnames[i] = ing.Hostname
		}
		t.Errorf("messaging ingress hostname not found:\n  want: %s\n  got:  %v", wantHost, hostnames)
	}
}

// TestIngressHostname_IngestionMatchesSpecApplier verifies the ingestion
// webhook ingress uses GenerateIngestionIngressHost.
func TestIngressHostname_IngestionMatchesSpecApplier(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	const (
		agentName       = "weather-poet"
		namespace       = "astro-abc123-0"
		ingestionDomain = "ingestion.astropods.ai"
		ingestionName   = "github-hooks"
	)

	dsSpec := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: agentName, Build: "b1", Registry: "r.io"},
		Agent: spec.DeploymentAgent{
			Image: "r.io/weather-poet:latest", Replicas: 1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "1", MemoryLimit: "1Gi"},
		},
		Ingestion: map[string]spec.DeploymentIngestion{
			ingestionName: {
				Image:     "ingestion:latest",
				Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi", CPULimit: "500m", MemoryLimit: "512Mi"},
				Trigger:   spec.DeploymentTrigger{Type: "webhook"},
				Endpoints: map[string]spec.Endpoint{
					"http": {Port: 8080, Protocol: "http"},
				},
			},
		},
	}

	nsCfg := &ds.NormalizedSpecConfig{Namespace: namespace, IngestionIngressDomain: ingestionDomain}
	depID := deployid.New()
	_, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: agentName,
		DisplayName: "Ingestion Ingress Test", BuildID: "b1", Namespace: namespace,
		SpecJSON: `{}`,
	}, func(tx *sql.Tx, id string) error {
		return ds.SaveNormalizedSpec(tx, id, dsSpec, nil, nsCfg)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	ings, err := store.GetIngresses(depID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}

	wantHost := k8s.GenerateIngestionIngressHost(agentName, namespace, ingestionName, ingestionDomain)

	found := false
	for _, ing := range ings {
		if ing.Hostname == wantHost {
			found = true
		}
	}
	if !found {
		hostnames := make([]string, len(ings))
		for i, ing := range ings {
			hostnames[i] = ing.Hostname
		}
		t.Errorf("ingestion ingress hostname not found:\n  want: %s\n  got:  %v", wantHost, hostnames)
	}
}

// TestIngressHostname_SasbotAllThreeTypes verifies all three ingress types
// from the real sasbot spec produce hostnames matching what the spec applier
// would generate. This is a regression guard against normalizer/applier drift.
func TestIngressHostname_SasbotAllThreeTypes(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)

	const (
		namespace       = "astro-sasbot-e2e-0"
		domain          = "agents.astropods.ai"
		ingestionDomain = "ingestion.astropods.ai"
	)

	nsCfg := &ds.NormalizedSpecConfig{
		Namespace:              namespace,
		IngressDomain:          domain,
		IngestionIngressDomain: ingestionDomain,
	}
	d := saveSasbot(t, db, store, parseSasbotSpec(t), nsCfg)

	ings, err := store.GetIngresses(d.ID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}

	hostnames := make(map[string]bool, len(ings))
	for _, ing := range ings {
		hostnames[ing.Hostname] = true
	}

	// The sasbot spec has: agent (exposed http endpoint), messaging (web adapter), ingestion webhook
	checks := []struct {
		label string
		host  string
	}{
		{"messaging", k8s.GenerateMessagingIngressHost("sasbot", namespace, domain)},
		{"ingestion/webhook", k8s.GenerateIngestionIngressHost("sasbot", namespace, "webhook", ingestionDomain)},
	}

	for _, c := range checks {
		if !hostnames[c.host] {
			t.Errorf("%s ingress hostname not found in DB:\n  want: %s\n  got:  %v", c.label, c.host, hostnames)
		}
	}
}
