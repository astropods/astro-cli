package driftcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// --- mock K8s client ---

type mockClusterClient struct {
	clientset *kubernetes.Clientset
}

func (m *mockClusterClient) Clientset() *kubernetes.Clientset      { return m.clientset }
func (m *mockClusterClient) Config() *rest.Config                  { return nil }
func (m *mockClusterClient) CheckHealth() error                    { return nil }
func (m *mockClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (m *mockClusterClient) DiagnoseConnection() map[string]string { return nil }

func newMockK8s(t *testing.T, handler http.Handler) k8s.ClusterClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("kubernetes.NewForConfig: %v", err)
	}
	return &mockClusterClient{clientset: cs}
}

// --- unit tests for drift detection against K8s ---

func TestCheckK8sDeployment_Missing(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for any Deployment GET
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	w := &deploymentstore.Workload{Name: "agent-a-agent", WorkloadType: "deployment", Image: "img:v1", Replicas: 1}

	drifts := c.checkK8sDeployment(context.Background(), dep, w, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].Type != DriftMissing {
		t.Errorf("type: got %q, want %q", drifts[0].Type, DriftMissing)
	}
	if drifts[0].Resource != "agent-a-agent" {
		t.Errorf("resource: got %q", drifts[0].Resource)
	}
}

func TestCheckK8sDeployment_ReplicaDrift(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a Deployment with 1 replica (desired is 3)
		w.Header().Set("Content-Type", "application/json")
		replicas := 1
		fmt.Fprintf(w, `{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": {"name": "agent-a-agent", "namespace": "ns-1"},
			"spec": {
				"replicas": %d,
				"template": {"spec": {"containers": [{"name": "main", "image": "img:v1"}]}}
			}
		}`, replicas)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	w := &deploymentstore.Workload{Name: "agent-a-agent", WorkloadType: "deployment", Image: "img:v1", Replicas: 3}

	drifts := c.checkK8sDeployment(context.Background(), dep, w, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift (replicas), got %d", len(drifts))
	}
	if drifts[0].Type != DriftReplicas {
		t.Errorf("type: got %q, want %q", drifts[0].Type, DriftReplicas)
	}
	if !strings.Contains(drifts[0].Detail, "desired=3") || !strings.Contains(drifts[0].Detail, "actual=1") {
		t.Errorf("detail: got %q", drifts[0].Detail)
	}
}

func TestCheckK8sDeployment_ImageDrift(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": {"name": "agent-a-agent", "namespace": "ns-1"},
			"spec": {
				"replicas": 2,
				"template": {"spec": {"containers": [{"name": "main", "image": "img:old"}]}}
			}
		}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	w := &deploymentstore.Workload{Name: "agent-a-agent", WorkloadType: "deployment", Image: "img:new", Replicas: 2}

	drifts := c.checkK8sDeployment(context.Background(), dep, w, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift (image), got %d", len(drifts))
	}
	if drifts[0].Type != DriftImage {
		t.Errorf("type: got %q, want %q", drifts[0].Type, DriftImage)
	}
	if !strings.Contains(drifts[0].Detail, "img:new") || !strings.Contains(drifts[0].Detail, "img:old") {
		t.Errorf("detail: got %q", drifts[0].Detail)
	}
}

func TestCheckK8sDeployment_NoDrift(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": {"name": "agent-a-agent", "namespace": "ns-1"},
			"spec": {
				"replicas": 2,
				"template": {"spec": {"containers": [{"name": "main", "image": "img:v1"}]}}
			}
		}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	w := &deploymentstore.Workload{Name: "agent-a-agent", WorkloadType: "deployment", Image: "img:v1", Replicas: 2}

	drifts := c.checkK8sDeployment(context.Background(), dep, w, "ns-1")
	if len(drifts) != 0 {
		t.Errorf("expected 0 drifts, got %d: %+v", len(drifts), drifts)
	}
}

func TestCheckStatefulSet_Missing(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	w := &deploymentstore.Workload{Name: "agent-a-model-llm", WorkloadType: "statefulset", Image: "ollama:v1", Replicas: 1}

	drifts := c.checkStatefulSet(context.Background(), dep, w, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].Type != DriftMissing {
		t.Errorf("type: got %q", drifts[0].Type)
	}
	if drifts[0].ResourceKind != "StatefulSet" {
		t.Errorf("kind: got %q", drifts[0].ResourceKind)
	}
}

func TestCheckStatefulSet_NoDrift(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "apps/v1", "kind": "StatefulSet",
			"metadata": {"name": "agent-a-model-llm", "namespace": "ns-1"},
			"spec": {
				"replicas": 1,
				"template": {"spec": {"containers": [{"name": "main", "image": "ollama:v1"}]}}
			}
		}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	w := &deploymentstore.Workload{Name: "agent-a-model-llm", WorkloadType: "statefulset", Image: "ollama:v1", Replicas: 1}

	drifts := c.checkStatefulSet(context.Background(), dep, w, "ns-1")
	if len(drifts) != 0 {
		t.Errorf("expected 0 drifts, got %d", len(drifts))
	}
}

func TestCheckCronJob_Missing(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	sched := "*/5 * * * *"
	w := &deploymentstore.Workload{Name: "agent-a-ingestion-sync", WorkloadType: "cronjob", TriggerSchedule: &sched}

	drifts := c.checkCronJob(context.Background(), dep, w, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].ResourceKind != "CronJob" {
		t.Errorf("kind: got %q", drifts[0].ResourceKind)
	}
}

func TestCheckCronJob_ScheduleDrift(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "batch/v1", "kind": "CronJob",
			"metadata": {"name": "agent-a-ingestion-sync", "namespace": "ns-1"},
			"spec": {"schedule": "0 * * * *"}
		}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}
	sched := "*/5 * * * *"
	w := &deploymentstore.Workload{Name: "agent-a-ingestion-sync", WorkloadType: "cronjob", TriggerSchedule: &sched}

	drifts := c.checkCronJob(context.Background(), dep, w, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift (schedule), got %d", len(drifts))
	}
	if drifts[0].Type != DriftSchedule {
		t.Errorf("type: got %q", drifts[0].Type)
	}
}

func TestCheckServices_Missing(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for any service GET
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}

	services := []*deploymentstore.Service{
		{Name: "agent-a-agent", Port: 8080, TargetPort: 8080, Protocol: "http"},
		{Name: "agent-a-model-llm", Port: 11434, TargetPort: 11434, Protocol: "http"},
	}

	drifts := c.checkServices(context.Background(), dep, services, "ns-1")
	if len(drifts) != 2 {
		t.Fatalf("expected 2 drifts (both missing), got %d", len(drifts))
	}
	for _, d := range drifts {
		if d.Type != DriftMissing {
			t.Errorf("type: got %q", d.Type)
		}
		if d.ResourceKind != "Service" {
			t.Errorf("kind: got %q", d.ResourceKind)
		}
	}
}

func TestCheckServices_Found(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a valid service for any GET
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "v1", "kind": "Service",
			"metadata": {"name": "agent-a-agent", "namespace": "ns-1"},
			"spec": {"ports": [{"port": 8080}]}
		}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}

	services := []*deploymentstore.Service{
		{Name: "agent-a-agent", Port: 8080},
	}

	drifts := c.checkServices(context.Background(), dep, services, "ns-1")
	if len(drifts) != 0 {
		t.Errorf("expected 0 drifts, got %d: %+v", len(drifts), drifts)
	}
}

func TestCheckIngresses_Missing(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty ingress list
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion": "networking.k8s.io/v1", "kind": "IngressList", "items": []}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}

	ingresses := []*deploymentstore.Ingress{
		{Hostname: "agent-a.astro.dev"},
	}

	drifts := c.checkIngresses(context.Background(), dep, ingresses, "ns-1")
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].Type != DriftMissing {
		t.Errorf("type: got %q", drifts[0].Type)
	}
	if drifts[0].ResourceKind != "Ingress" {
		t.Errorf("kind: got %q", drifts[0].ResourceKind)
	}
}

func TestCheckIngresses_Found(t *testing.T) {
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "networking.k8s.io/v1", "kind": "IngressList",
			"items": [{
				"metadata": {"name": "agent-a-ingress", "namespace": "ns-1"},
				"spec": {"rules": [{"host": "agent-a.astro.dev"}]}
			}]
		}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}

	ingresses := []*deploymentstore.Ingress{
		{Hostname: "agent-a.astro.dev"},
	}

	drifts := c.checkIngresses(context.Background(), dep, ingresses, "ns-1")
	if len(drifts) != 0 {
		t.Errorf("expected 0 drifts, got %d", len(drifts))
	}
}

func TestCheckMultipleWorkloads_AllMissing(t *testing.T) {
	// Respond 404 for all K8s requests — every workload should report "missing"
	k8sClient := newMockK8s(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
	}))

	log := logger.New("error", "json")
	c := &Checker{k8sClient: k8sClient, log: log}
	dep := &deploymentstore.Deployment{ID: "dep-1", Namespace: "ns-1", AgentName: "agent-a"}

	// Check each workload type individually and collect drifts
	var drifts []Drift
	drifts = append(drifts, c.checkK8sDeployment(context.Background(), dep,
		&deploymentstore.Workload{Name: "agent-a-agent", WorkloadType: "deployment", Image: "img:v1", Replicas: 1}, "ns-1")...)
	drifts = append(drifts, c.checkStatefulSet(context.Background(), dep,
		&deploymentstore.Workload{Name: "agent-a-model-llm", WorkloadType: "statefulset", Image: "ollama:v1", Replicas: 1}, "ns-1")...)

	sched := "*/5 * * * *"
	drifts = append(drifts, c.checkCronJob(context.Background(), dep,
		&deploymentstore.Workload{Name: "agent-a-ingestion-sync", WorkloadType: "cronjob", TriggerSchedule: &sched}, "ns-1")...)

	if len(drifts) != 3 {
		t.Fatalf("expected 3 drifts (all missing), got %d", len(drifts))
	}
	for _, d := range drifts {
		if d.Type != DriftMissing {
			t.Errorf("expected missing, got %q for %s", d.Type, d.Resource)
		}
	}

	// Verify each resource kind
	kinds := map[string]bool{}
	for _, d := range drifts {
		kinds[d.ResourceKind] = true
	}
	for _, expected := range []string{"Deployment", "StatefulSet", "CronJob"} {
		if !kinds[expected] {
			t.Errorf("missing %s drift", expected)
		}
	}
}

// --- Report structure tests ---

func TestReport_JSONRoundTrip(t *testing.T) {
	r := &Report{
		Timestamp:          time.Now().UTC(),
		DeploymentsChecked: 2,
		Drifts: []Drift{
			{
				DeploymentID: "dep-1", Namespace: "ns-1", AgentName: "agent-a",
				Resource: "agent-a-agent", ResourceKind: "Deployment",
				Type: DriftMissing, Detail: `Deployment "agent-a-agent" expected but not found`,
			},
			{
				DeploymentID: "dep-1", Namespace: "ns-1", AgentName: "agent-a",
				Resource: "agent-a-agent", ResourceKind: "Deployment",
				Type: DriftReplicas, Detail: "replicas: desired=2 actual=1",
			},
			{
				DeploymentID: "dep-2", Namespace: "ns-2", AgentName: "agent-b",
				Resource: "agent-b-model-gpt4", ResourceKind: "StatefulSet",
				Type: DriftImage, Detail: `image: desired="ollama:v2" actual="ollama:v1"`,
			},
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.DeploymentsChecked != 2 {
		t.Errorf("deployments_checked: got %d", decoded.DeploymentsChecked)
	}
	if len(decoded.Drifts) != 3 {
		t.Fatalf("drifts: got %d", len(decoded.Drifts))
	}
	if decoded.Drifts[0].Type != DriftMissing {
		t.Errorf("drift[0].type: got %q", decoded.Drifts[0].Type)
	}
	if decoded.Drifts[2].ResourceKind != "StatefulSet" {
		t.Errorf("drift[2].kind: got %q", decoded.Drifts[2].ResourceKind)
	}
}

func TestLogReport(t *testing.T) {
	log := logger.New("error", "json")
	c := &Checker{log: log}

	// Should not panic for any case
	c.logReport(&Report{DeploymentsChecked: 0})
	c.logReport(&Report{DeploymentsChecked: 5, Drifts: nil})
	c.logReport(&Report{
		DeploymentsChecked: 1,
		Drifts: []Drift{{
			DeploymentID: "d1", Namespace: "ns", AgentName: "a",
			Resource: "r", ResourceKind: "Deployment",
			Type: DriftMissing, Detail: "test",
		}},
	})
}

func TestNew(t *testing.T) {
	log := logger.New("error", "json")
	store := deploymentstore.NewStore(nil)
	c := New(store, nil, log)
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.deployStore != store {
		t.Error("deployStore not set")
	}
}
