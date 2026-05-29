//go:build integration || k8s

package e2e

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/handlers"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// These tests assert that a deployment's owning publisher account ("source
// account") is plumbed end-to-end from the deployments table through the
// ListDeployments / GetDeployment handlers and out to the JSON the client
// consumes. They guard the cross-account lineage attribution fix: before
// source_account was on the DTO, two same-named blueprints in different
// accounts were indistinguishable to the client, which would mis-attribute
// upgrade signals to the viewer's same-named (but lineage-unrelated)
// blueprint and surface false "Update available" badges on cross-account
// deployments.
//
// The tests run against a real Postgres (DATABASE_URL) and invoke the real
// gin handlers via httptest, so any drift in the SQL, the
// resolveSourceAccountName fallback chain, or the JSON tag will fail here
// even though sqlmock unit tests would still pass. K8s is stubbed with a
// 404-for-everything httptest server so enrichDeployment falls into its
// dbOnly path; that path is what populates SourceAccount, which is what
// we're asserting.

// fakeClusterClient implements k8s.ClusterClient with a clientset pointed
// at a local httptest server. The server returns 404 for every K8s API
// call, which forces handlers.enrichDeployment into its dbOnly branch.
// That branch is the one that calls agentDeploymentFromDB, which in turn
// runs resolveSourceAccountName — the path under test.
type fakeClusterClient struct{ cs *kubernetes.Clientset }

func (f *fakeClusterClient) Clientset() *kubernetes.Clientset      { return f.cs }
func (f *fakeClusterClient) Config() *rest.Config                  { return nil }
func (f *fakeClusterClient) CheckHealth() error                    { return nil }
func (f *fakeClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (f *fakeClusterClient) DiagnoseConnection() map[string]string { return nil }

func newFakeK8sNotFound(t *testing.T) k8s.ClusterClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("kubernetes.NewForConfig: %v", err)
	}
	return &fakeClusterClient{cs: cs}
}

// sourceAccountFixture wires three deployments living under the same
// target account with three different source-account shapes:
//
//   - xAcctDep: source_account_id points at a *different* publisher
//     account. This is the cross-account scenario where the bug
//     reproduces in production.
//   - sameAcctDep: source_account_id equals the target account. Same name
//     as xAcctDep so we exercise the "same name, different lineage"
//     collision the bug originally hid.
//   - legacyDep: source_account_id is NULL (legacy row), but the
//     deployment_spec_json has source.account = publisher. Exercises the
//     resolveSourceAccountName fallback.
type sourceAccountFixture struct {
	db            *sql.DB
	router        *gin.Engine
	userID        string
	targetAcct    *account.Account
	publisherAcct *account.Account
	agentName     string
	xAcctDep      *ds.Deployment
	sameAcctDep   *ds.Deployment
	legacyDep     *ds.Deployment
}

func setupSourceAccountFixture(t *testing.T) *sourceAccountFixture {
	t.Helper()
	db := sourceAccountTestDB(t)
	accountStore := account.NewAccountStore(db)
	index := agentindex.NewIndexWithDB(db)
	deployStore := ds.NewStore(db).WithLineageValidator(index)

	userID := "user-srcacct-" + deployid.New()
	targetName := "src-tgt-" + strings.ToLower(deployid.New())
	targetAcct, err := accountStore.Create(targetName, "personal", userID, "Target")
	if err != nil {
		t.Fatalf("create target account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", targetAcct.ID) })

	publisherName := "src-pub-" + strings.ToLower(deployid.New())
	publisherAcct, err := accountStore.Create(publisherName, "organization", "pub-owner-"+deployid.New(), "Publisher Org")
	if err != nil {
		t.Fatalf("create publisher account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", publisherAcct.ID) })

	// Same agent name registered under both accounts with different build IDs is
	// the central collision scenario the bug hid. The handler must surface
	// source_account on the JSON so the client can disambiguate which lineage a
	// given deployment came from.
	agentName := "name-collision-bot-" + strings.ToLower(deployid.New()[:6])
	pubBuild := "build-pub-" + strings.ToLower(deployid.New()[:6])
	tgtBuild := "build-tgt-" + strings.ToLower(deployid.New()[:6])
	registerAgent(t, index, publisherAcct, agentName, pubBuild)
	registerAgent(t, index, targetAcct, agentName, tgtBuild)

	legacyAgent := agentName + "-legacy"
	registerAgent(t, index, publisherAcct, legacyAgent, pubBuild)

	xAcctDep := saveSourceAccountDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       targetAcct.ID,
		SourceAccountID: publisherAcct.ID,
		AgentName:       agentName,
		DisplayName:     "Cross-Account Bot",
		BuildID:         pubBuild,
		Namespace:       "astro-xacct-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(publisherAcct.Name, targetAcct.Name, agentName, pubBuild),
	})

	sameAcctDep := saveSourceAccountDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       targetAcct.ID,
		SourceAccountID: targetAcct.ID,
		AgentName:       agentName,
		DisplayName:     "Same-Account Bot",
		BuildID:         tgtBuild,
		Namespace:       "astro-same-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(targetAcct.Name, targetAcct.Name, agentName, tgtBuild),
	})

	legacyDep := saveSourceAccountDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       targetAcct.ID,
		SourceAccountID: "", // intentionally NULL — exercises spec-JSON fallback
		AgentName:       legacyAgent,
		DisplayName:     "Legacy Bot",
		BuildID:         pubBuild,
		Namespace:       "astro-legacy-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(publisherAcct.Name, targetAcct.Name, legacyAgent, pubBuild),
	})

	return &sourceAccountFixture{
		db:            db,
		router:        newSourceAccountRouter(t, userID, accountStore, index, deployStore),
		userID:        userID,
		targetAcct:    targetAcct,
		publisherAcct: publisherAcct,
		agentName:     agentName,
		xAcctDep:      xAcctDep,
		sameAcctDep:   sameAcctDep,
		legacyDep:     legacyDep,
	}
}

func sourceAccountTestDB(t *testing.T) *sql.DB {
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

func registerAgent(t *testing.T, index *agentindex.Index, acct *account.Account, name, buildID string) {
	t.Helper()
	specMap := map[string]any{
		"name": name,
		"agent": map[string]any{
			"image": "registry.io/" + acct.Name + "/" + name + ":" + buildID,
		},
	}
	if err := index.Register(acct.ID, name, buildID, "registry.io", acct.Name, specMap, "", "", ""); err != nil {
		t.Fatalf("register agent %s/%s@%s: %v", acct.Name, name, buildID, err)
	}
}

func buildSourceAccountDeploymentSpecJSON(sourceAccount, targetAccount, agentName, buildID string) string {
	payload := map[string]any{
		"spec": "deployment/v1",
		"source": map[string]any{
			"account": sourceAccount,
			"name":    agentName,
			"build":   buildID,
		},
		"target": map[string]any{
			"runtime": "kubernetes",
			"account": targetAccount,
		},
	}
	out, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal deployment spec: %v", err))
	}
	return string(out)
}

func saveSourceAccountDeployment(t *testing.T, store *ds.Store, p ds.SaveDeploymentParams) *ds.Deployment {
	t.Helper()
	dep, err := store.SaveDeploymentPending(p, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending(%s): %v", p.AgentName, err)
	}
	return dep
}

func newSourceAccountRouter(t *testing.T, userID string, accountStore *account.AccountStore, index *agentindex.Index, deployStore *ds.Store) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	cfg := &config.Config{}
	k8sClient := newFakeK8sNotFound(t)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
		c.Next()
	})
	r.GET("/api/v1/deployments", handlers.ListDeployments(
		log, accountStore, deployStore, index, nil, nil, k8scache.NoopCache{},
	))
	r.GET("/api/v1/deployments/:id", handlers.GetDeployment(
		log, accountStore, cfg, k8s.NewRegistryWithPrimary(k8sClient), deployStore, index, nil, nil, k8scache.NoopCache{},
	))
	return r
}

// listDeploymentsResponse mirrors the JSON shape returned by ListDeployments.
// It deliberately uses a loose map for each deployment so the test asserts on
// the on-the-wire JSON (including json:"source_account,omitempty" semantics),
// not against a re-marshalling of handlers.AgentDeployment.
type listDeploymentsResponse struct {
	Deployments []map[string]json.RawMessage `json:"deployments"`
	Count       int                          `json:"count"`
}

func listDeployments(t *testing.T, fx *sourceAccountFixture) listDeploymentsResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/deployments?account="+fx.targetAcct.Name, nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListDeployments: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp listDeploymentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return resp
}

func getDeployment(t *testing.T, fx *sourceAccountFixture, depID string) map[string]json.RawMessage {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/"+depID, nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetDeployment(%s): expected 200, got %d: %s", depID, rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	var dep map[string]json.RawMessage
	if err := json.Unmarshal(resp["deployment"], &dep); err != nil {
		t.Fatalf("decode deployment: %v; body=%s", err, rec.Body.String())
	}
	return dep
}

func decodeString(t *testing.T, raw json.RawMessage, field string) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode %s: %v (raw=%s)", field, err, string(raw))
	}
	return s
}

func indexByID(deps []map[string]json.RawMessage) map[string]map[string]json.RawMessage {
	out := make(map[string]map[string]json.RawMessage, len(deps))
	for _, d := range deps {
		id := strings.Trim(string(d["id"]), `"`)
		out[id] = d
	}
	return out
}

// TestGetDeploymentE2E_CrossAccountSurfacesSourceAccount verifies the detail
// endpoint surfaces source_account for cross-account deployments. The
// deployment-detail "Update available" banner reads source_account from this
// response; without it the banner would compare against the wrong lineage.
func TestGetDeploymentE2E_CrossAccountSurfacesSourceAccount(t *testing.T) {
	fx := setupSourceAccountFixture(t)

	dep := getDeployment(t, fx, fx.xAcctDep.ID)
	got := decodeString(t, dep["source_account"], "source_account")
	if got != fx.publisherAcct.Name {
		t.Errorf("GetDeployment cross-account: source_account = %q, want %q",
			got, fx.publisherAcct.Name)
	}
}

// TestGetDeploymentE2E_SameAccountSurfacesSourceAccount verifies the detail
// endpoint surfaces source_account when the deployment's source is the same
// account as the viewer.
func TestGetDeploymentE2E_SameAccountSurfacesSourceAccount(t *testing.T) {
	fx := setupSourceAccountFixture(t)

	dep := getDeployment(t, fx, fx.sameAcctDep.ID)
	got := decodeString(t, dep["source_account"], "source_account")
	if got != fx.targetAcct.Name {
		t.Errorf("GetDeployment same-account: source_account = %q, want %q",
			got, fx.targetAcct.Name)
	}
}

// TestGetDeploymentE2E_LegacyRowFallsBackToSpecAccount covers pre-migration
// rows where source_account_id is NULL. The handler must still surface the
// publisher's account name by parsing deployment_spec_json.source.account.
func TestGetDeploymentE2E_LegacyRowFallsBackToSpecAccount(t *testing.T) {
	fx := setupSourceAccountFixture(t)

	dep := getDeployment(t, fx, fx.legacyDep.ID)
	got := decodeString(t, dep["source_account"], "source_account")
	if got != fx.publisherAcct.Name {
		t.Errorf("legacy fallback: source_account = %q, want %q (from spec JSON)",
			got, fx.publisherAcct.Name)
	}
}

// TestGetDeploymentE2E_SourceIDFallsBackToSpecWhenAccountLookupFails covers
// the fallback branch where source_account_id is populated but no longer
// resolves to an active account row (soft-deleted publisher).
func TestGetDeploymentE2E_SourceIDFallsBackToSpecWhenAccountLookupFails(t *testing.T) {
	fx := setupSourceAccountFixture(t)
	if _, err := fx.db.Exec("UPDATE accounts SET deleted_at = NOW() WHERE id = $1", fx.publisherAcct.ID); err != nil {
		t.Fatalf("soft-delete publisher account: %v", err)
	}

	dep := getDeployment(t, fx, fx.xAcctDep.ID)
	got := decodeString(t, dep["source_account"], "source_account")
	if got != fx.publisherAcct.Name {
		t.Errorf("source_account fallback after unresolved source_account_id = %q, want %q from spec JSON",
			got, fx.publisherAcct.Name)
	}
}

// TestGetDeploymentE2E_StaleSourceIDWithoutTupleLeavesSourceAccountEmpty
// verifies that a deployment with a source_account_id that has no matching
// lineage tuple does not fabricate a source_account value.
func TestGetDeploymentE2E_StaleSourceIDWithoutTupleLeavesSourceAccountEmpty(t *testing.T) {
	fx := setupSourceAccountFixture(t)
	ghost := "ghost-build-" + deployid.New()
	kindID := deployid.New()

	t.Cleanup(func() {
		_, _ = fx.db.Exec("DELETE FROM deployments WHERE id = $1", kindID)
	})

	_, err := fx.db.Exec(
		`INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id,
			namespace, display_name, deployment_spec_json, status, status_changed_at, deployed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', NOW(), NOW())`,
		kindID,
		fx.targetAcct.ID,
		fx.publisherAcct.ID,
		fx.agentName,
		ghost,
		"astro-bad-src-"+deployid.Compact(deployid.New()),
		"BadTuple",
		buildSourceAccountDeploymentSpecJSON(fx.publisherAcct.Name, fx.targetAcct.Name, fx.agentName, ghost),
	)
	if err != nil {
		t.Fatalf("insert bad-tuple deployment: %v", err)
	}

	dep := getDeployment(t, fx, kindID)
	if raw, has := dep["source_account"]; has && len(raw) > 0 {
		t.Fatalf("expected empty/absent source_account for invalid tuple; got %s", string(raw))
	}
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(b)
}
