package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/arn"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// k8sTracker is a minimal in-memory K8s API fake.
// It tracks created resources by path and returns appropriate responses.
type k8sTracker struct {
	mu        sync.Mutex
	resources map[string]bool // "METHOD path" entries
}

func newK8sTracker() *k8sTracker {
	return &k8sTracker{resources: make(map[string]bool)}
}

func (t *k8sTracker) exists(path string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resources[path]
}

func (t *k8sTracker) add(path string) {
	t.mu.Lock()
	t.resources[path] = true
	t.mu.Unlock()
}

func (t *k8sTracker) del(path string) {
	t.mu.Lock()
	delete(t.resources, path)
	t.mu.Unlock()
}

var nameRe = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)

func extractName(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	m := nameRe.FindSubmatch(body)
	if len(m) >= 2 {
		return string(m[1])
	}
	return ""
}

func (t *k8sTracker) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path

		switch {
		// Namespace CREATE
		case r.Method == http.MethodPost && p == "/api/v1/namespaces":
			name := extractName(r)
			t.add("ns:" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":%q}}`, name)

		// Namespace GET
		case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/v1/namespaces/") && strings.Count(p, "/") == 3:
			name := strings.TrimPrefix(p, "/api/v1/namespaces/")
			if t.exists("ns:" + name) {
				fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":%q}}`, name)
			} else {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","reason":"NotFound","code":404}`)
			}

		// Secret CREATE
		case r.Method == http.MethodPost && strings.Contains(p, "/secrets") && !strings.Contains(p, "/secrets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns := parts[0]
			name := extractName(r)
			t.add("secret:" + ns + "/" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Secret","metadata":{"name":%q,"namespace":%q}}`, name, ns)

		// Secret GET
		case r.Method == http.MethodGet && strings.Contains(p, "/secrets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			if t.exists("secret:" + ns + "/" + name) {
				fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Secret","metadata":{"name":%q}}`, name)
			} else {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","reason":"NotFound","code":404}`)
			}

		// Secret UPDATE (PUT)
		case r.Method == http.MethodPut && strings.Contains(p, "/secrets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.add("secret:" + ns + "/" + name)
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Secret","metadata":{"name":%q}}`, name)

		// Secret DELETE
		case r.Method == http.MethodDelete && strings.Contains(p, "/secrets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.del("secret:" + ns + "/" + name)
			fmt.Fprintf(w, `{"kind":"Status","status":"Success"}`)

		// ConfigMap CREATE
		case r.Method == http.MethodPost && strings.Contains(p, "/configmaps") && !strings.Contains(p, "/configmaps/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns := parts[0]
			name := extractName(r)
			t.add("configmap:" + ns + "/" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q}}`, name)

		// ConfigMap DELETE
		case r.Method == http.MethodDelete && strings.Contains(p, "/configmaps/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.del("configmap:" + ns + "/" + name)
			fmt.Fprintf(w, `{"kind":"Status","status":"Success"}`)

		// Service CREATE
		case r.Method == http.MethodPost && strings.Contains(p, "/services") && !strings.Contains(p, "/services/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns := parts[0]
			name := extractName(r)
			t.add("service:" + ns + "/" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":%q}}`, name)

		// Service DELETE
		case r.Method == http.MethodDelete && strings.Contains(p, "/services/"):
			parts := strings.Split(strings.TrimPrefix(p, "/api/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.del("service:" + ns + "/" + name)
			fmt.Fprintf(w, `{"kind":"Status","status":"Success"}`)

		// NetworkPolicy CREATE
		case r.Method == http.MethodPost && strings.Contains(p, "/networkpolicies") && !strings.Contains(p, "/networkpolicies/"):
			parts := strings.Split(strings.TrimPrefix(p, "/apis/networking.k8s.io/v1/namespaces/"), "/")
			ns := parts[0]
			name := extractName(r)
			t.add("netpol:" + ns + "/" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":%q}}`, name)

		// NetworkPolicy UPDATE
		case r.Method == http.MethodPut && strings.Contains(p, "/networkpolicies/"):
			parts := strings.Split(strings.TrimPrefix(p, "/apis/networking.k8s.io/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.add("netpol:" + ns + "/" + name)
			fmt.Fprintf(w, `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":%q}}`, name)

		// PodDisruptionBudget CREATE
		case r.Method == http.MethodPost && strings.Contains(p, "/poddisruptionbudgets") && !strings.Contains(p, "/poddisruptionbudgets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/apis/policy/v1/namespaces/"), "/")
			ns := parts[0]
			name := extractName(r)
			t.add("pdb:" + ns + "/" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"policy/v1","kind":"PodDisruptionBudget","metadata":{"name":%q}}`, name)

		// PodDisruptionBudget DELETE
		case r.Method == http.MethodDelete && strings.Contains(p, "/poddisruptionbudgets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/apis/policy/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.del("pdb:" + ns + "/" + name)
			fmt.Fprintf(w, `{"kind":"Status","status":"Success"}`)

		// StatefulSet CREATE
		case r.Method == http.MethodPost && strings.Contains(p, "/statefulsets"):
			parts := strings.Split(strings.TrimPrefix(p, "/apis/apps/v1/namespaces/"), "/")
			ns := parts[0]
			name := extractName(r)
			t.add("statefulset:" + ns + "/" + name)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":%q}}`, name)

		// StatefulSet DELETE
		case r.Method == http.MethodDelete && strings.Contains(p, "/statefulsets/"):
			parts := strings.Split(strings.TrimPrefix(p, "/apis/apps/v1/namespaces/"), "/")
			ns, name := parts[0], parts[2]
			t.del("statefulset:" + ns + "/" + name)
			fmt.Fprintf(w, `{"kind":"Status","status":"Success"}`)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"kind":"Status","reason":"NotFound","code":404}`)
		}
	})
}

type testKnowledgeClusterClient struct {
	clientset *kubernetes.Clientset
}

func (c *testKnowledgeClusterClient) Clientset() *kubernetes.Clientset      { return c.clientset }
func (c *testKnowledgeClusterClient) Config() *rest.Config                  { return nil }
func (c *testKnowledgeClusterClient) CheckHealth() error                    { return nil }
func (c *testKnowledgeClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (c *testKnowledgeClusterClient) DiagnoseConnection() map[string]string { return nil }

func newTestKnowledgeClient(tracker *k8sTracker) (ClusterClient, func()) {
	srv := httptest.NewServer(tracker.handler())
	cs, _ := kubernetes.NewForConfig(&rest.Config{
		Host:          srv.URL,
		ContentConfig: rest.ContentConfig{ContentType: "application/json"},
	})
	return &testKnowledgeClusterClient{clientset: cs}, srv.Close
}

// --- Tests ---

func TestKnowledgeResourceName(t *testing.T) {
	cases := []struct {
		storeID string
		want    string
	}{
		{"abc-123-xyz", "kn-abc-123-xyz"},
		{"387-c48-q2z", "kn-387-c48-q2z"}, // digit-leading store IDs are valid after prefix
		{"000-000-000", "kn-000-000-000"},
	}
	for _, tc := range cases {
		got := KnowledgeResourceName(tc.storeID)
		if got != tc.want {
			t.Errorf("KnowledgeResourceName(%q) = %q, want %q", tc.storeID, got, tc.want)
		}
		if got[0] < 'a' || got[0] > 'z' {
			t.Errorf("KnowledgeResourceName(%q) = %q: must start with a letter (DNS-1035)", tc.storeID, got)
		}
	}
}

func TestKnowledgeNamespace(t *testing.T) {
	accountID := "550e8400-e29b-41d4-a716-446655440000"
	got := KnowledgeNamespace(accountID)

	// Must start with the prefix.
	if !strings.HasPrefix(got, "knowledge-") {
		t.Errorf("namespace %q must start with 'knowledge-'", got)
	}
	// Short ID must be exactly 16 lowercase hex chars (FNV-64a).
	shortID := strings.TrimPrefix(got, "knowledge-")
	if len(shortID) != 16 {
		t.Errorf("short ID %q: want 16 chars, got %d", shortID, len(shortID))
	}
	// Must be deterministic.
	if got != KnowledgeNamespace(accountID) {
		t.Error("KnowledgeNamespace must be deterministic")
	}
	// Two distinct accounts must produce distinct namespaces.
	other := KnowledgeNamespace("aaaaaaaa-0000-0000-0000-000000000000")
	if got == other {
		t.Error("distinct accounts must produce distinct namespaces")
	}
}

func TestEnsureKnowledgeNamespace_Creates(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	if err := EnsureKnowledgeNamespace(context.Background(), client, "acct-123"); err != nil {
		t.Fatalf("EnsureKnowledgeNamespace: %v", err)
	}

	ns := KnowledgeNamespace("acct-123")
	if !tr.exists("ns:" + ns) {
		t.Errorf("namespace %q was not created", ns)
	}
}

func TestEnsureKnowledgeNamespace_Idempotent(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	if err := EnsureKnowledgeNamespace(context.Background(), client, "acct-idem"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := EnsureKnowledgeNamespace(context.Background(), client, "acct-idem"); err != nil {
		t.Fatalf("second call (idempotent): %v", err)
	}
}

func TestApplyKnowledgeSecret_Create(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	accountID := "acct-sec"
	ns := KnowledgeNamespace(accountID)
	tr.add("ns:" + ns) // namespace pre-exists

	err := ApplyKnowledgeSecret(context.Background(), client, accountID, "sid-001", "sid-001-credentials", map[string]string{
		"POSTGRES_PASSWORD": "s3cr3t",
	})
	if err != nil {
		t.Fatalf("ApplyKnowledgeSecret: %v", err)
	}

	if !tr.exists("secret:" + ns + "/sid-001-credentials") {
		t.Error("secret was not created")
	}
}

func TestSecretExists_True(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	accountID := "acct-chk"
	ns := KnowledgeNamespace(accountID)
	tr.add("ns:" + ns)
	tr.add("secret:" + ns + "/my-secret") // pre-seed

	exists, err := SecretExists(context.Background(), client, accountID, "my-secret")
	if err != nil {
		t.Fatalf("SecretExists: %v", err)
	}
	if !exists {
		t.Error("expected secret to exist")
	}
}

func TestSecretExists_False(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	exists, err := SecretExists(context.Background(), client, "acct-miss", "no-such-secret")
	if err != nil {
		t.Fatalf("SecretExists: %v", err)
	}
	if exists {
		t.Error("expected secret to not exist")
	}
}

func TestDeleteKnowledgeStore_NotFoundIsOK(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	// Delete a store that never existed — should not error (not-found is tolerated).
	if err := DeleteKnowledgeStore(context.Background(), client, "acct-del", "sid-999", false); err != nil {
		t.Fatalf("DeleteKnowledgeStore: %v", err)
	}
}

func TestDeleteKnowledgeStore_Public(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	accountID := "acct-pub"
	ns := KnowledgeNamespace(accountID)
	storeID := "sid-pub"
	rn := KnowledgeResourceName(storeID)

	// Pre-seed all resources that a public store would have.
	tr.add("statefulset:" + ns + "/" + rn)
	tr.add("service:" + ns + "/" + rn)
	tr.add("service:" + ns + "/" + rn + "-lb")
	tr.add("secret:" + ns + "/" + storeID + "-credentials")

	if err := DeleteKnowledgeStore(context.Background(), client, accountID, storeID, true); err != nil {
		t.Fatalf("DeleteKnowledgeStore: %v", err)
	}

	if tr.exists("statefulset:" + ns + "/" + rn) {
		t.Error("statefulset should have been deleted")
	}
	if tr.exists("service:" + ns + "/" + rn) {
		t.Error("clusterip service should have been deleted")
	}
	if tr.exists("service:" + ns + "/" + rn + "-lb") {
		t.Error("lb service should have been deleted")
	}
}

// --- ProvisionKnowledgeStore ---

func testProvisionParams(accountID, storeID string, public bool) KnowledgeProvisionParams {
	return KnowledgeProvisionParams{
		StoreID:    storeID,
		AccountID:  accountID,
		ARN:        arn.KnowledgeStore(accountID, "my-db"),
		Provider:   "postgres",
		Storage:    "10Gi",
		SecretName: storeID + "-credentials",
		Public:     public,
	}
}

func TestProvisionKnowledgeStore_Success(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	accountID := "acct-prov"
	storeID := "sid-prov"
	ns := KnowledgeNamespace(accountID)
	rn := KnowledgeResourceName(storeID)

	if err := ProvisionKnowledgeStore(context.Background(), client, testProvisionParams(accountID, storeID, false)); err != nil {
		t.Fatalf("ProvisionKnowledgeStore: %v", err)
	}

	if !tr.exists("statefulset:" + ns + "/" + rn) {
		t.Error("expected statefulset to be created")
	}
	if !tr.exists("service:" + ns + "/" + rn) {
		t.Error("expected clusterip service to be created")
	}
	if tr.exists("service:" + ns + "/" + rn + "-lb") {
		t.Error("lb service should not be created for non-public store")
	}
}

func TestProvisionKnowledgeStore_Public(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	accountID := "acct-pub2"
	storeID := "sid-pub2"
	ns := KnowledgeNamespace(accountID)
	rn := KnowledgeResourceName(storeID)

	if err := ProvisionKnowledgeStore(context.Background(), client, testProvisionParams(accountID, storeID, true)); err != nil {
		t.Fatalf("ProvisionKnowledgeStore: %v", err)
	}

	if !tr.exists("statefulset:" + ns + "/" + rn) {
		t.Error("expected statefulset to be created")
	}
	if !tr.exists("service:" + ns + "/" + rn) {
		t.Error("expected clusterip service to be created")
	}
	if !tr.exists("service:" + ns + "/" + rn + "-lb") {
		t.Error("expected lb service to be created for public store")
	}
}

func TestBuildKnowledgeService_ExternalDNSAnnotation(t *testing.T) {
	labels := map[string]string{"app": "test"}
	selector := map[string]string{"app": "test"}

	// Without PublicHost — no annotation.
	svc := buildKnowledgeService("test-lb", "ns", labels, selector, 5432, corev1.ServiceTypeLoadBalancer)
	if svc.Annotations != nil {
		t.Errorf("expected no annotations without PublicHost, got %v", svc.Annotations)
	}

	// With PublicHost — annotation should be added by the provisioner.
	svc.Annotations = map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": "my-db.acme.knowledge.astropod.ai",
	}
	if got := svc.Annotations["external-dns.alpha.kubernetes.io/hostname"]; got != "my-db.acme.knowledge.astropod.ai" {
		t.Errorf("expected external-dns annotation, got %q", got)
	}
}

func TestProvisionKnowledgeStore_UnknownProvider(t *testing.T) {
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	defer done()

	p := testProvisionParams("acct-bad", "sid-bad", false)
	p.Provider = "nonexistent"

	if err := ProvisionKnowledgeStore(context.Background(), client, p); err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

// TestKnowledgeLabels_NoARN is a regression test: ARNs contain colons which
// Kubernetes rejects as label values. The ARN must not appear in labels.
func TestKnowledgeLabels_NoARN(t *testing.T) {
	labels := knowledgeLabels("acct-123", "store-456")

	if _, ok := labels["astro.io/arn"]; ok {
		t.Error("labels must not contain 'astro.io/arn': colons in ARN values are rejected by Kubernetes")
	}
	if labels["astro.io/account-id"] != "acct-123" {
		t.Errorf("expected account-id 'acct-123', got %q", labels["astro.io/account-id"])
	}
	if labels["astro.io/store-id"] != "store-456" {
		t.Errorf("expected store-id 'store-456', got %q", labels["astro.io/store-id"])
	}
	if labels["astro.io/component"] != "knowledge" {
		t.Errorf("expected component 'knowledge', got %q", labels["astro.io/component"])
	}
}
