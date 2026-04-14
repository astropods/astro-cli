package k8s

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/arn"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
)

// knowledgeE2EResult holds everything created by runKnowledgeE2E for assertions.
type knowledgeE2EResult struct {
	tracker    *k8sTracker
	client     ClusterClient
	storeID    string
	accountID  string
	provider   string
	secretName string
	ns         string
	public     bool
}

// runKnowledgeE2E executes the full managed knowledge store provisioning pipeline:
// ensure namespace → generate credentials → apply secret → provision K8s resources.
func runKnowledgeE2E(t *testing.T, accountID, storeID, provider string, public bool) *knowledgeE2EResult {
	t.Helper()

	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	t.Cleanup(done)

	ctx := context.Background()
	ns := KnowledgeNamespace(accountID)
	secretName := storeID + "-credentials"

	if err := EnsureKnowledgeNamespace(ctx, client, accountID); err != nil {
		t.Fatalf("EnsureKnowledgeNamespace: %v", err)
	}

	plainCreds, err := knowledgestore.GenerateCredentials(provider)
	if err != nil {
		t.Fatalf("GenerateCredentials: %v", err)
	}

	if err := ApplyKnowledgeSecret(ctx, client, accountID, storeID, secretName, plainCreds); err != nil {
		t.Fatalf("ApplyKnowledgeSecret: %v", err)
	}

	if err := ProvisionKnowledgeStore(ctx, client, KnowledgeProvisionParams{
		StoreID:    storeID,
		AccountID:  accountID,
		ARN:        arn.KnowledgeStore(accountID, storeID),
		Provider:   provider,
		Storage:    "10Gi",
		SecretName: secretName,
		Public:     public,
		LocalMode:  true, // skip security hardening for tests
	}); err != nil {
		t.Fatalf("ProvisionKnowledgeStore: %v", err)
	}

	return &knowledgeE2EResult{
		tracker:    tr,
		client:     client,
		storeID:    storeID,
		accountID:  accountID,
		provider:   provider,
		secretName: secretName,
		ns:         ns,
		public:     public,
	}
}

func (r *knowledgeE2EResult) assertNamespace(t *testing.T) {
	t.Helper()
	if !r.tracker.exists("ns:" + r.ns) {
		t.Errorf("namespace %q was not created", r.ns)
	}
}

func (r *knowledgeE2EResult) assertSecret(t *testing.T) {
	t.Helper()
	key := "secret:" + r.ns + "/" + r.secretName
	if !r.tracker.exists(key) {
		t.Errorf("credentials secret %q was not created", key)
	}
}

func (r *knowledgeE2EResult) assertStatefulSet(t *testing.T) {
	t.Helper()
	key := "statefulset:" + r.ns + "/" + KnowledgeResourceName(r.storeID)
	if !r.tracker.exists(key) {
		t.Errorf("StatefulSet %q was not created", key)
	}
}

func (r *knowledgeE2EResult) assertClusterIPService(t *testing.T) {
	t.Helper()
	key := "service:" + r.ns + "/" + KnowledgeResourceName(r.storeID)
	if !r.tracker.exists(key) {
		t.Errorf("ClusterIP service %q was not created", key)
	}
}

func (r *knowledgeE2EResult) assertLoadBalancerService(t *testing.T, exists bool) {
	t.Helper()
	key := "service:" + r.ns + "/" + KnowledgeResourceName(r.storeID) + "-lb"
	got := r.tracker.exists(key)
	if exists && !got {
		t.Errorf("LoadBalancer service %q should exist but was not created", key)
	}
	if !exists && got {
		t.Errorf("LoadBalancer service %q should not exist but was created", key)
	}
}

// --- test cases ---

func TestKnowledgeE2E_Postgres_Private(t *testing.T) {
	r := runKnowledgeE2E(t, "acct-pg-priv", "sid-pg-001", "postgres", false)

	r.assertNamespace(t)
	r.assertSecret(t)
	r.assertStatefulSet(t)
	r.assertClusterIPService(t)
	r.assertLoadBalancerService(t, false) // private: no LB
}

func TestKnowledgeE2E_Postgres_Public(t *testing.T) {
	r := runKnowledgeE2E(t, "acct-pg-pub", "sid-pg-002", "postgres", true)

	r.assertNamespace(t)
	r.assertSecret(t)
	r.assertStatefulSet(t)
	r.assertClusterIPService(t)
	r.assertLoadBalancerService(t, true) // public: LB required
}

func TestKnowledgeE2E_Qdrant_Private(t *testing.T) {
	r := runKnowledgeE2E(t, "acct-qdrant", "sid-qd-001", "qdrant", false)

	r.assertNamespace(t)
	r.assertSecret(t)
	r.assertStatefulSet(t)
	r.assertClusterIPService(t)
	r.assertLoadBalancerService(t, false)
}

func TestKnowledgeE2E_Redis_Private(t *testing.T) {
	r := runKnowledgeE2E(t, "acct-redis", "sid-rd-001", "redis", false)

	r.assertNamespace(t)
	r.assertSecret(t)
	r.assertStatefulSet(t)
	r.assertClusterIPService(t)
}

func TestKnowledgeE2E_Neo4j_Private(t *testing.T) {
	r := runKnowledgeE2E(t, "acct-neo4j", "sid-nj-001", "neo4j", false)

	r.assertNamespace(t)
	r.assertSecret(t)
	r.assertStatefulSet(t)
	r.assertClusterIPService(t)
}

func TestKnowledgeE2E_NamespaceSharedAcrossStores(t *testing.T) {
	// Both stores belong to the same account — should share one namespace.
	tr := newK8sTracker()
	client, done := newTestKnowledgeClient(tr)
	t.Cleanup(done)
	ctx := context.Background()
	accountID := "acct-shared"
	ns := KnowledgeNamespace(accountID)

	_ = EnsureKnowledgeNamespace(ctx, client, accountID)
	_ = ApplyKnowledgeSecret(ctx, client, accountID, "store-a", "store-a-credentials", map[string]string{})
	if err := ProvisionKnowledgeStore(ctx, client, KnowledgeProvisionParams{
		StoreID: "store-a", AccountID: accountID,
		ARN: arn.KnowledgeStore("acct-shared", "store-a"), Provider: "postgres",
		Storage: "10Gi", SecretName: "store-a-credentials", LocalMode: true,
	}); err != nil {
		t.Fatalf("ProvisionKnowledgeStore store-a: %v", err)
	}

	_ = ApplyKnowledgeSecret(ctx, client, accountID, "store-b", "store-b-credentials", map[string]string{})
	if err := ProvisionKnowledgeStore(ctx, client, KnowledgeProvisionParams{
		StoreID: "store-b", AccountID: accountID,
		ARN: arn.KnowledgeStore("acct-shared", "store-b"), Provider: "qdrant",
		Storage: "5Gi", SecretName: "store-b-credentials", LocalMode: true,
	}); err != nil {
		t.Fatalf("ProvisionKnowledgeStore store-b: %v", err)
	}

	// One namespace, two statefulsets
	if !tr.exists("ns:" + ns) {
		t.Error("shared namespace was not created")
	}
	if !tr.exists("statefulset:" + ns + "/" + KnowledgeResourceName("store-a")) {
		t.Error("statefulset store-a missing")
	}
	if !tr.exists("statefulset:" + ns + "/" + KnowledgeResourceName("store-b")) {
		t.Error("statefulset store-b missing")
	}
}

func TestKnowledgeE2E_Delete(t *testing.T) {
	r := runKnowledgeE2E(t, "acct-del-e2e", "sid-del-001", "postgres", true)

	// Verify provisioned.
	r.assertStatefulSet(t)
	r.assertClusterIPService(t)
	r.assertLoadBalancerService(t, true)
	r.assertSecret(t)

	// Delete.
	if err := DeleteKnowledgeStore(context.Background(), r.client, r.accountID, r.storeID, r.public); err != nil {
		t.Fatalf("DeleteKnowledgeStore: %v", err)
	}

	// Resources should be gone.
	if r.tracker.exists("statefulset:" + r.ns + "/" + r.storeID) {
		t.Error("StatefulSet should have been deleted")
	}
	if r.tracker.exists("service:" + r.ns + "/" + r.storeID) {
		t.Error("ClusterIP service should have been deleted")
	}
	if r.tracker.exists("service:" + r.ns + "/" + r.storeID + "-lb") {
		t.Error("LB service should have been deleted")
	}
	if r.tracker.exists("secret:" + r.ns + "/" + r.secretName) {
		t.Error("credentials secret should have been deleted")
	}
}
