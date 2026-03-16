package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"sort"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
)

// sasbotSpecJSON is the real deployment spec from a running sasbot deployment.
// This is used as the source of truth for integration tests.
const sasbotSpecJSON = `{"spec":"deployment/v1","source":{"account":"saswatds","name":"sasbot","build":"14f4c4dd","registry":"969403051954.dkr.ecr.us-east-1.amazonaws.com"},"target":{"runtime":"kubernetes","account":"saswatds","display_name":"Sasbot"},"agent":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot:14f4c4dd","endpoints":{"http":{"port":8080,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"environment":{"ANTHROPIC_API_KEY":"${variables.ANTHROPIC_API_KEY}","ASTRO_AGENT_BUILD":"${source.build}","ASTRO_AGENT_NAME":"${source.name}","CLOUDFLARE_ACCOUNT_ID":"${variables.CLOUDFLARE_ACCOUNT_ID}","CLOUDFLARE_AI_API_KEY":"${variables.CLOUDFLARE_AI_API_KEY}","EMBEDDING_DIMENSION":"768","EMBEDDING_MODEL":"nomic-embed-text","GITHUB_TOKEN":"${variables.GITHUB_TOKEN}","NEO4J_HOST":"${knowledge.graph.host}","NEO4J_PORT":"${knowledge.graph.http.port}","NEO4J_URL":"${knowledge.graph.http.url}","OLLAMA_BASE_URL":"${models.ollama.http.url}/api","OLLAMA_HOST":"${models.ollama.host}","OLLAMA_MODEL":"qwen3.5:2b","OLLAMA_PORT":"${models.ollama.http.port}","OLLAMA_URL":"${models.ollama.http.url}","QDRANT_HOST":"${knowledge.docs.host}","QDRANT_PORT":"${knowledge.docs.http.port}","QDRANT_URL":"${knowledge.docs.http.url}","REDIS_HOST":"${knowledge.cache.host}","REDIS_PORT":"${knowledge.cache.http.port}","REDIS_URL":"${knowledge.cache.http.url}"},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"}},"models":{"ollama":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ollama/ollama:latest","endpoints":{"http":{"port":11434,"protocol":"http"}},"replicas":1,"resources":{"cpu":"2","memory":"8Gi","cpu_limit":"4","memory_limit":"16Gi"},"gpu":{"runtime":"cuda","count":1},"environment":{"OLLAMA_HOST":"0.0.0.0","OLLAMA_KEEP_ALIVE":"-1","OLLAMA_MODEL":"qwen3.5:2b"},"healthcheck":{"test":["sh","-c","ollama list | grep -q 'qwen3.5:2b'"],"interval":"15s","timeout":"5s","retries":40},"update":{"strategy":"recreate"},"model":"qwen3.5:2b","persistent":true,"provider":"ollama"}},"knowledge":{"cache":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/redis:7-alpine","endpoints":{"http":{"port":6379,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":false,"healthcheck":{"test":["redis-cli","ping"]},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"},"provider":"redis"},"docs":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/qdrant/qdrant:latest","endpoints":{"grpc":{"port":6334,"protocol":"grpc"},"http":{"port":6333,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":true,"storage":{"size":"10Gi","access_mode":"ReadWriteOnce"},"healthcheck":{"path":"/healthz"},"update":{"strategy":"recreate"},"provider":"qdrant"},"graph":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/neo4j:5-community","endpoints":{"bolt":{"port":7687,"protocol":"tcp"},"http":{"port":7474,"protocol":"http"}},"replicas":1,"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"persistent":false,"environment":{"NEO4J_AUTH":"none"},"healthcheck":{"path":"/"},"update":{"strategy":"rolling","max_unavailable":"25%","max_surge":"25%"},"provider":"neo4j"}},"ingestion":{"webhook":{"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot-ingestion-webhook:14f4c4dd","endpoints":{"http":{"port":3001,"protocol":"http"}},"resources":{"cpu":"100m","memory":"256Mi","cpu_limit":"1","memory_limit":"1Gi"},"trigger":{"type":"webhook"}}},"interfaces":{"adapters":["web"],"image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/astropods/messaging:latest","endpoints":{"grpc":{"port":9090,"protocol":"grpc"},"http":{"port":8080,"protocol":"http","expose":{"enabled":false}}},"resources":{"cpu":"100m","memory":"128Mi","cpu_limit":"500m","memory_limit":"512Mi"}},"variables":{"ANTHROPIC_API_KEY":{"targets":["agent"],"secret":true},"CLOUDFLARE_ACCOUNT_ID":{"targets":["agent"],"secret":true},"CLOUDFLARE_AI_API_KEY":{"targets":["agent"],"secret":true},"EMBEDDING_DIMENSION":{"value":"768","targets":["agent"],"optional":true},"EMBEDDING_MODEL":{"value":"nomic-embed-text","targets":["agent"],"optional":true},"GITHUB_TOKEN":{"targets":["agent"],"secret":true},"SLACK_APP_TOKEN":{"targets":["interface.slack"],"secret":true,"optional":true},"SLACK_BOT_TOKEN":{"targets":["interface.slack"],"secret":true,"optional":true}},"observability":{"enabled":true,"provider":"galileo","image":"969403051954.dkr.ecr.us-east-1.amazonaws.com/prod-astro-collector:latest","port":4318,"resources":{"cpu":"50m","memory":"128Mi","cpu_limit":"250m","memory_limit":"256Mi"}}}`

func parseSasbotSpec(t *testing.T) *spec.AstroDeploymentSpec {
	t.Helper()
	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(sasbotSpecJSON), &ds); err != nil {
		t.Fatalf("failed to parse sasbot spec: %v", err)
	}
	return &ds
}

func saveSasbotDeployment(t *testing.T, db *sql.DB, store *Store, ds *spec.AstroDeploymentSpec) *Deployment {
	t.Helper()
	accountID := ensureTestAccount(t, db)
	resolved := &deployment.ResolvedEnv{
		ConfigMapData: map[string]string{},
		SecretData:    map[string]string{},
	}

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "sasbot",
		DisplayName: "Sasbot", BuildID: "14f4c4dd", Namespace: "ns-sasbot-test",
		SpecJSON: sasbotSpecJSON,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, resolved, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	return d
}

// TestSasbotNormalized_Workloads verifies SaveNormalizedSpec creates the correct
// workload rows for the real sasbot deployment spec.
func TestSasbotNormalized_Workloads(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	workloads, err := store.GetWorkloads(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}

	// Expected: agent + ollama model + cache + docs + graph + webhook ingestion + messaging + collector = 8
	if len(workloads) != 8 {
		names := make([]string, len(workloads))
		for i, w := range workloads {
			names[i] = w.Name + " (" + w.ComponentKind + "/" + w.ComponentKey + ")"
		}
		t.Fatalf("expected 8 workloads, got %d: %v", len(workloads), names)
	}

	byName := make(map[string]*Workload)
	for _, w := range workloads {
		byName[w.Name] = w
	}

	// --- Agent ---
	agent := byName["sasbot-agent"]
	if agent == nil {
		t.Fatal("agent workload 'sasbot-agent' not found")
	}
	assertWorkload(t, agent, workloadExpect{
		componentKind: "agent", componentKey: "",
		workloadType: "deployment", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot:14f4c4dd",
		cpuRequest: "100m", memoryRequest: "256Mi",
		cpuLimit: "1", memoryLimit: "1Gi",
		persistent:  false,
		updateStrat: strPtr("rolling"), updateMaxUnavail: strPtr("25%"), updateMaxSurge: strPtr("25%"),
	})

	// --- Model: ollama ---
	ollama := byName["sasbot-model-ollama"]
	if ollama == nil {
		t.Fatal("model workload 'sasbot-model-ollama' not found")
	}
	assertWorkload(t, ollama, workloadExpect{
		componentKind: "model", componentKey: "ollama",
		workloadType: "statefulset", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ollama/ollama:latest",
		cpuRequest: "2", memoryRequest: "8Gi",
		cpuLimit: "4", memoryLimit: "16Gi",
		persistent: true,
		gpuRuntime: strPtr("cuda"), gpuCount: intPtr(1),
		updateStrat: strPtr("recreate"),
		hcTest:      strPtr("sh -c ollama list | grep -q 'qwen3.5:2b'"),
		hcInterval:  strPtr("15s"), hcTimeout: strPtr("5s"), hcRetries: intPtr(40),
	})

	// --- Knowledge: cache (redis, not persistent) ---
	cache := byName["sasbot-knowledge-cache"]
	if cache == nil {
		t.Fatal("knowledge workload 'sasbot-knowledge-cache' not found")
	}
	assertWorkload(t, cache, workloadExpect{
		componentKind: "knowledge", componentKey: "cache",
		workloadType: "deployment", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/redis:7-alpine",
		cpuRequest: "100m", memoryRequest: "256Mi",
		cpuLimit: "1", memoryLimit: "1Gi",
		persistent:  false,
		updateStrat: strPtr("rolling"), updateMaxUnavail: strPtr("25%"), updateMaxSurge: strPtr("25%"),
		hcTest: strPtr("redis-cli ping"),
	})

	// --- Knowledge: docs (qdrant, persistent) ---
	docs := byName["sasbot-knowledge-docs"]
	if docs == nil {
		t.Fatal("knowledge workload 'sasbot-knowledge-docs' not found")
	}
	assertWorkload(t, docs, workloadExpect{
		componentKind: "knowledge", componentKey: "docs",
		workloadType: "statefulset", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/qdrant/qdrant:latest",
		cpuRequest: "100m", memoryRequest: "256Mi",
		cpuLimit: "1", memoryLimit: "1Gi",
		persistent:  true,
		updateStrat: strPtr("recreate"),
		hcPath:      strPtr("/healthz"),
	})

	// --- Knowledge: graph (neo4j, not persistent) ---
	graph := byName["sasbot-knowledge-graph"]
	if graph == nil {
		t.Fatal("knowledge workload 'sasbot-knowledge-graph' not found")
	}
	assertWorkload(t, graph, workloadExpect{
		componentKind: "knowledge", componentKey: "graph",
		workloadType: "deployment", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/neo4j:5-community",
		cpuRequest: "100m", memoryRequest: "256Mi",
		cpuLimit: "1", memoryLimit: "1Gi",
		persistent:  false,
		updateStrat: strPtr("rolling"), updateMaxUnavail: strPtr("25%"), updateMaxSurge: strPtr("25%"),
		hcPath: strPtr("/"),
	})

	// --- Ingestion: webhook ---
	webhook := byName["sasbot-ingestion-webhook"]
	if webhook == nil {
		t.Fatal("ingestion workload 'sasbot-ingestion-webhook' not found")
	}
	assertWorkload(t, webhook, workloadExpect{
		componentKind: "ingestion", componentKey: "webhook",
		workloadType: "deployment", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/preview-tenant-saswatds/sasbot-ingestion-webhook:14f4c4dd",
		cpuRequest: "100m", memoryRequest: "256Mi",
		cpuLimit: "1", memoryLimit: "1Gi",
		persistent:  false,
		triggerType: strPtr("webhook"),
	})

	// --- Messaging (interfaces sidecar) ---
	messaging := byName["sasbot-messaging"]
	if messaging == nil {
		t.Fatal("messaging workload 'sasbot-messaging' not found")
	}
	assertWorkload(t, messaging, workloadExpect{
		componentKind: "messaging", componentKey: "",
		workloadType: "sidecar", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/dockerhub/astropods/messaging:latest",
		cpuRequest: "100m", memoryRequest: "128Mi",
		cpuLimit: "500m", memoryLimit: "512Mi",
		persistent: false,
	})

	// --- Collector (observability sidecar) ---
	collector := byName["sasbot-collector"]
	if collector == nil {
		t.Fatal("collector workload 'sasbot-collector' not found")
	}
	assertWorkload(t, collector, workloadExpect{
		componentKind: "collector", componentKey: "",
		workloadType: "sidecar", replicas: 1,
		image:      "969403051954.dkr.ecr.us-east-1.amazonaws.com/prod-astro-collector:latest",
		cpuRequest: "50m", memoryRequest: "128Mi",
		cpuLimit: "250m", memoryLimit: "256Mi",
		persistent: false,
	})
}

// TestSasbotNormalized_Services verifies the correct service rows are created.
func TestSasbotNormalized_Services(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	services, err := store.GetServices(d.ID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}

	// Expected services:
	// agent:      http(8080)
	// ollama:     http(11434)
	// cache:      http(6379)
	// docs:       grpc(6334), http(6333)
	// graph:      bolt(7687), http(7474)
	// webhook:    http(3001)
	// messaging:  grpc(9090), http(8080)
	// collector:  otlp-grpc(4317), otlp-http(4318)
	// Total: 12
	if len(services) != 12 {
		for _, s := range services {
			t.Logf("  service: workload=%s name=%s port=%d protocol=%s", s.WorkloadName, s.Name, s.Port, s.Protocol)
		}
		t.Fatalf("expected 12 services, got %d", len(services))
	}

	type svcKey struct {
		workloadName string
		name         string
	}
	bySvc := make(map[svcKey]*Service)
	for _, s := range services {
		bySvc[svcKey{s.WorkloadName, s.Name}] = s
	}

	expectations := []struct {
		workloadName string
		name         string
		port         int
		targetPort   int
		protocol     string
	}{
		{"sasbot-agent", "http", 8080, 8080, "http"},
		{"sasbot-model-ollama", "http", 11434, 11434, "http"},
		{"sasbot-knowledge-cache", "http", 6379, 6379, "http"},
		{"sasbot-knowledge-docs", "grpc", 6334, 6334, "grpc"},
		{"sasbot-knowledge-docs", "http", 6333, 6333, "http"},
		{"sasbot-knowledge-graph", "bolt", 7687, 7687, "tcp"},
		{"sasbot-knowledge-graph", "http", 7474, 7474, "http"},
		{"sasbot-ingestion-webhook", "http", 3001, 3001, "http"},
		{"sasbot-messaging", "grpc", 9090, 9090, "grpc"},
		{"sasbot-messaging", "http", 8080, 8080, "http"},
		{"sasbot-collector", "otlp-grpc", 4317, 4317, "grpc"},
		{"sasbot-collector", "otlp-http", 4318, 4318, "http"},
	}

	for _, exp := range expectations {
		key := svcKey{exp.workloadName, exp.name}
		svc := bySvc[key]
		if svc == nil {
			t.Errorf("service not found: workload=%q name=%q", exp.workloadName, exp.name)
			continue
		}
		if svc.Port != exp.port {
			t.Errorf("service %s/%s port: got %d, want %d", exp.workloadName, exp.name, svc.Port, exp.port)
		}
		if svc.TargetPort != exp.targetPort {
			t.Errorf("service %s/%s target_port: got %d, want %d", exp.workloadName, exp.name, svc.TargetPort, exp.targetPort)
		}
		if svc.Protocol != exp.protocol {
			t.Errorf("service %s/%s protocol: got %q, want %q", exp.workloadName, exp.name, svc.Protocol, exp.protocol)
		}
	}
}

// TestSasbotNormalized_Volumes verifies PVC rows for persistent components.
func TestSasbotNormalized_Volumes(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	// Query volumes via workload join
	rows, err := db.Query(`
		SELECT dv.mount_path, dv.size, dv.storage_class, dv.access_mode, dw.name
		FROM deployment_volumes dv
		JOIN deployment_workloads dw ON dw.id = dv.workload_id
		WHERE dw.deployment_id = $1
		ORDER BY dw.name
	`, d.ID)
	if err != nil {
		t.Fatalf("query volumes: %v", err)
	}
	defer rows.Close()

	type volRow struct {
		mountPath, size, accessMode, workloadName string
		storageClass                              *string
	}
	var volumes []volRow
	for rows.Next() {
		var v volRow
		if err := rows.Scan(&v.mountPath, &v.size, &v.storageClass, &v.accessMode, &v.workloadName); err != nil {
			t.Fatalf("scan volume: %v", err)
		}
		volumes = append(volumes, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	// Only persistent components get volumes: ollama model + qdrant docs = 2
	// redis cache and neo4j graph are NOT persistent
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(volumes))
	}

	byWL := make(map[string]volRow)
	for _, v := range volumes {
		byWL[v.workloadName] = v
	}

	// Qdrant docs: mount_path from provider (/qdrant/storage), size from storage config (10Gi)
	docsVol := byWL["sasbot-knowledge-docs"]
	if docsVol.mountPath != "/qdrant/storage" {
		t.Errorf("docs volume mount_path: got %q, want '/qdrant/storage'", docsVol.mountPath)
	}
	if docsVol.size != "10Gi" {
		t.Errorf("docs volume size: got %q, want '10Gi'", docsVol.size)
	}
	if docsVol.accessMode != "ReadWriteOnce" {
		t.Errorf("docs volume access_mode: got %q, want 'ReadWriteOnce'", docsVol.accessMode)
	}

	// Ollama model: mount_path from provider (/root/.ollama), default size (50Gi)
	ollamaVol := byWL["sasbot-model-ollama"]
	if ollamaVol.mountPath != "/root/.ollama" {
		t.Errorf("ollama volume mount_path: got %q, want '/root/.ollama'", ollamaVol.mountPath)
	}
	if ollamaVol.size != "50Gi" {
		t.Errorf("ollama volume size: got %q, want '50Gi'", ollamaVol.size)
	}
	if ollamaVol.accessMode != "ReadWriteOnce" {
		t.Errorf("ollama volume access_mode: got %q, want 'ReadWriteOnce'", ollamaVol.accessMode)
	}
}

// TestSasbotNormalized_NoIngresses verifies no ingress rows are created
// (the messaging http endpoint has expose.enabled=false).
func TestSasbotNormalized_NoIngresses(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	ingresses, err := store.GetIngresses(d.ID)
	if err != nil {
		t.Fatalf("GetIngresses: %v", err)
	}
	if len(ingresses) != 0 {
		t.Errorf("expected 0 ingresses (no exposed endpoints), got %d", len(ingresses))
	}
}

// TestSasbotNormalized_Variables verifies variable rows match the spec.
func TestSasbotNormalized_Variables(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	vars, err := store.GetDeploymentVariables(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentVariables: %v", err)
	}

	// 8 variables from the spec
	if len(vars) != 8 {
		names := make([]string, len(vars))
		for i, v := range vars {
			names[i] = v.Name
		}
		t.Fatalf("expected 8 variables, got %d: %v", len(vars), names)
	}

	byName := make(map[string]Variable)
	for _, v := range vars {
		byName[v.Name] = v
	}

	// Secrets with no value (encryptor is nil)
	for _, name := range []string{"ANTHROPIC_API_KEY", "CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_AI_API_KEY", "GITHUB_TOKEN"} {
		v := byName[name]
		if !v.Secret {
			t.Errorf("%s should be secret", name)
		}
		if v.Optional {
			t.Errorf("%s should NOT be optional", name)
		}
		if v.Value != "" {
			t.Errorf("%s value should be empty (no encryptor), got %q", name, v.Value)
		}
	}

	// Optional secrets
	for _, name := range []string{"SLACK_APP_TOKEN", "SLACK_BOT_TOKEN"} {
		v := byName[name]
		if !v.Secret {
			t.Errorf("%s should be secret", name)
		}
		if !v.Optional {
			t.Errorf("%s should be optional", name)
		}
	}

	// Non-secret with defaults
	emDim := byName["EMBEDDING_DIMENSION"]
	if emDim.Secret {
		t.Error("EMBEDDING_DIMENSION should not be secret")
	}
	if !emDim.Optional {
		t.Error("EMBEDDING_DIMENSION should be optional")
	}
	if emDim.Value != "768" {
		t.Errorf("EMBEDDING_DIMENSION value: got %q, want '768'", emDim.Value)
	}

	emModel := byName["EMBEDDING_MODEL"]
	if emModel.Secret {
		t.Error("EMBEDDING_MODEL should not be secret")
	}
	if emModel.Value != "nomic-embed-text" {
		t.Errorf("EMBEDDING_MODEL value: got %q, want 'nomic-embed-text'", emModel.Value)
	}

	// Verify targets
	assertTargets := func(name string, want []string) {
		v := byName[name]
		got := v.Targets
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("%s targets: got %v, want %v", name, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s targets[%d]: got %q, want %q", name, i, got[i], want[i])
			}
		}
	}
	assertTargets("ANTHROPIC_API_KEY", []string{"agent"})
	assertTargets("SLACK_APP_TOKEN", []string{"interface.slack"})
}

// TestSasbotNormalized_WorkloadTypeMapping verifies that persistent components
// are statefulsets and non-persistent are deployments, and sidecars are sidecars.
func TestSasbotNormalized_WorkloadTypeMapping(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	workloads, err := store.GetWorkloads(d.ID)
	if err != nil {
		t.Fatalf("GetWorkloads: %v", err)
	}

	expected := map[string]string{
		"sasbot-agent":             "deployment",  // agent is always a deployment
		"sasbot-model-ollama":      "statefulset", // persistent=true
		"sasbot-knowledge-cache":   "deployment",  // persistent=false
		"sasbot-knowledge-docs":    "statefulset", // persistent=true
		"sasbot-knowledge-graph":   "deployment",  // persistent=false
		"sasbot-ingestion-webhook": "deployment",  // webhook trigger
		"sasbot-messaging":         "sidecar",     // interfaces
		"sasbot-collector":         "sidecar",     // observability
	}

	for _, w := range workloads {
		want, ok := expected[w.Name]
		if !ok {
			t.Errorf("unexpected workload: %q", w.Name)
			continue
		}
		if w.WorkloadType != want {
			t.Errorf("workload %q type: got %q, want %q", w.Name, w.WorkloadType, want)
		}
	}
}

// TestSasbotNormalized_ServiceWorkloadNameJoin verifies that GetServices
// populates WorkloadName correctly via the join on deployment_workloads.
func TestSasbotNormalized_ServiceWorkloadNameJoin(t *testing.T) {
	db := testDB(t)
	ds := parseSasbotSpec(t)
	store := NewStore(db)
	d := saveSasbotDeployment(t, db, store, ds)

	services, err := store.GetServices(d.ID)
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}

	// Every service must have a non-empty WorkloadName
	for _, svc := range services {
		if svc.WorkloadName == "" {
			t.Errorf("service %q (port %d) has empty WorkloadName", svc.Name, svc.Port)
		}
	}

	// Group services by workload name and verify counts
	byWorkload := make(map[string]int)
	for _, svc := range services {
		byWorkload[svc.WorkloadName]++
	}

	expectedCounts := map[string]int{
		"sasbot-agent":             1, // http
		"sasbot-model-ollama":      1, // http
		"sasbot-knowledge-cache":   1, // http
		"sasbot-knowledge-docs":    2, // grpc + http
		"sasbot-knowledge-graph":   2, // bolt + http
		"sasbot-ingestion-webhook": 1, // http
		"sasbot-messaging":         2, // grpc + http
		"sasbot-collector":         2, // otlp-grpc + otlp-http
	}

	for wl, want := range expectedCounts {
		got := byWorkload[wl]
		if got != want {
			t.Errorf("workload %q: expected %d services, got %d", wl, want, got)
		}
	}
}

// TestSasbotNormalized_SpecParse verifies the JSON round-trips correctly through
// the spec parser, catching any field mapping issues.
func TestSasbotNormalized_SpecParse(t *testing.T) {
	ds := parseSasbotSpec(t)

	if ds.Source.Name != "sasbot" {
		t.Errorf("source.name: got %q, want 'sasbot'", ds.Source.Name)
	}
	if ds.Source.Build != "14f4c4dd" {
		t.Errorf("source.build: got %q", ds.Source.Build)
	}
	if ds.Agent.Replicas != 1 {
		t.Errorf("agent.replicas: got %d", ds.Agent.Replicas)
	}
	if len(ds.Models) != 1 {
		t.Errorf("models count: got %d", len(ds.Models))
	}
	if len(ds.Knowledge) != 3 {
		t.Errorf("knowledge count: got %d", len(ds.Knowledge))
	}
	if len(ds.Ingestion) != 1 {
		t.Errorf("ingestion count: got %d", len(ds.Ingestion))
	}
	if ds.Interfaces == nil {
		t.Fatal("interfaces should not be nil")
	}
	if len(ds.Interfaces.Adapters) != 1 || ds.Interfaces.Adapters[0] != "web" {
		t.Errorf("interfaces.adapters: got %v", ds.Interfaces.Adapters)
	}
	if !ds.Observability.Enabled {
		t.Error("observability should be enabled")
	}
	if len(ds.Variables) != 8 {
		t.Errorf("variables count: got %d", len(ds.Variables))
	}
}

// --- Helpers ---

type workloadExpect struct {
	componentKind, componentKey string
	workloadType                string
	replicas                    int
	image                       string
	cpuRequest, memoryRequest   string
	cpuLimit, memoryLimit       string
	persistent                  bool
	distributed                 bool
	gpuRuntime                  *string
	gpuCount                    *int
	updateStrat                 *string
	updateMaxUnavail            *string
	updateMaxSurge              *string
	hcPath                      *string
	hcTest                      *string
	hcInterval                  *string
	hcTimeout                   *string
	hcRetries                   *int
	triggerType                 *string
	triggerSchedule             *string
}

func assertWorkload(t *testing.T, w *Workload, exp workloadExpect) {
	t.Helper()
	prefix := w.Name

	if w.ComponentKind != exp.componentKind {
		t.Errorf("%s component_kind: got %q, want %q", prefix, w.ComponentKind, exp.componentKind)
	}
	if w.ComponentKey != exp.componentKey {
		t.Errorf("%s component_key: got %q, want %q", prefix, w.ComponentKey, exp.componentKey)
	}
	if w.WorkloadType != exp.workloadType {
		t.Errorf("%s workload_type: got %q, want %q", prefix, w.WorkloadType, exp.workloadType)
	}
	if w.Replicas != exp.replicas {
		t.Errorf("%s replicas: got %d, want %d", prefix, w.Replicas, exp.replicas)
	}
	if w.Image != exp.image {
		t.Errorf("%s image: got %q, want %q", prefix, w.Image, exp.image)
	}
	if w.CPURequest != exp.cpuRequest {
		t.Errorf("%s cpu_request: got %q, want %q", prefix, w.CPURequest, exp.cpuRequest)
	}
	if w.MemoryRequest != exp.memoryRequest {
		t.Errorf("%s memory_request: got %q, want %q", prefix, w.MemoryRequest, exp.memoryRequest)
	}
	if w.CPULimit != exp.cpuLimit {
		t.Errorf("%s cpu_limit: got %q, want %q", prefix, w.CPULimit, exp.cpuLimit)
	}
	if w.MemoryLimit != exp.memoryLimit {
		t.Errorf("%s memory_limit: got %q, want %q", prefix, w.MemoryLimit, exp.memoryLimit)
	}
	if w.Persistent != exp.persistent {
		t.Errorf("%s persistent: got %v, want %v", prefix, w.Persistent, exp.persistent)
	}
	if w.Distributed != exp.distributed {
		t.Errorf("%s distributed: got %v, want %v", prefix, w.Distributed, exp.distributed)
	}

	assertOptStr(t, prefix+" gpu_runtime", w.GPURuntime, exp.gpuRuntime)
	assertOptInt(t, prefix+" gpu_count", w.GPUCount, exp.gpuCount)
	assertOptStr(t, prefix+" update_strategy", w.UpdateStrategy, exp.updateStrat)
	assertOptStr(t, prefix+" update_max_unavail", w.UpdateMaxUnavail, exp.updateMaxUnavail)
	assertOptStr(t, prefix+" update_max_surge", w.UpdateMaxSurge, exp.updateMaxSurge)
	assertOptStr(t, prefix+" healthcheck_path", w.HealthcheckPath, exp.hcPath)
	assertOptStr(t, prefix+" healthcheck_test", w.HealthcheckTest, exp.hcTest)
	assertOptStr(t, prefix+" healthcheck_interval", w.HealthcheckIntv, exp.hcInterval)
	assertOptStr(t, prefix+" healthcheck_timeout", w.HealthcheckTimeout, exp.hcTimeout)
	assertOptInt(t, prefix+" healthcheck_retries", w.HealthcheckRetries, exp.hcRetries)
	assertOptStr(t, prefix+" trigger_type", w.TriggerType, exp.triggerType)
	assertOptStr(t, prefix+" trigger_schedule", w.TriggerSchedule, exp.triggerSchedule)
}

func assertOptStr(t *testing.T, label string, got, want *string) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s: got %q, want nil", label, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s: got nil, want %q", label, *want)
		return
	}
	if *got != *want {
		t.Errorf("%s: got %q, want %q", label, *got, *want)
	}
}

func assertOptInt(t *testing.T, label string, got, want *int) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s: got %d, want nil", label, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s: got nil, want %d", label, *want)
		return
	}
	if *got != *want {
		t.Errorf("%s: got %d, want %d", label, *got, *want)
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
