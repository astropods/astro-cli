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

func TestKnowledgeNamespace(t *testing.T) {
	got := KnowledgeNamespace("550e8400-e29b-41d4-a716-446655440000")
	want := "knlg0-550e8400-e29b-41d4-a716-446655440000"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
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
