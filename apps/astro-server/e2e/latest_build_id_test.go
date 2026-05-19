//go:build integration || k8s

package e2e

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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
)

// These tests are the regression guard for the dashboard's "Update available"
// badge, which is now derived purely from the server-supplied latest_build_id
// on each deployment in the ListDeployments response. The contract on
// AgentDeployment.LatestBuildID says it is empty when:
//
//   - the lineage agent has no published versions
//   - the batch lookup fails
//   - the lineage agent is private and not owned by the viewer
//
// The third clause was promised in the doc comment but missing from the
// implementation, which let private cross-account upgrade signals leak through
// to the dashboard and surface a Redeploy affordance the deploy endpoint would
// reject (private blueprints don't deploy across account boundaries — see
// canDeploySourceAgent). This file exercises ListDeployments end-to-end against
// real Postgres + real agentindex so a regression on either side fails here.

// latestBuildIDFixture wires a viewer account, a separate publisher account,
// agents with deterministic version histories, and one deployment per scenario
// the dashboard cares about. Each deployment id is exposed on the fixture so
// individual tests can pick the deployment they care about out of the list
// response without re-deriving it from the spec.
type latestBuildIDFixture struct {
	db            *sql.DB
	router        *gin.Engine
	userID        string
	viewerAcct    *account.Account
	publisherAcct *account.Account

	depSameAcctStale            *ds.Deployment // same-account, public, newer build available
	depSameAcctCurrent          *ds.Deployment // same-account, public, on the latest build
	depSameAcctPrivateStale     *ds.Deployment // same-account, private, newer build available
	depXAcctPublicStale         *ds.Deployment // cross-account, public, newer build available
	depXAcctPublicCurrent       *ds.Deployment // cross-account, public, on the latest build
	depXAcctPrivateStale        *ds.Deployment // cross-account, PRIVATE — must NOT expose latest
	depAgentMissingFromIndex    *ds.Deployment // deployment for an agent with no agent_versions row
	depAgentArchivedFromPublish *ds.Deployment // cross-account, agent never published any version

	stalePublicLatest      string
	currentPublicLatest    string
	samePrivateLatest      string
	xacctPublicLatest      string
	xacctPublicLone        string
	xacctPrivateLatestRaw  string // newest build in the publisher's lineage; should be suppressed
	deployedStaleBuildID   string
	deployedCurrentBuildID string
	deployedPrivateBuildID string
	deployedXAcctOldBuild  string
}

func setupLatestBuildIDFixture(t *testing.T) *latestBuildIDFixture {
	t.Helper()
	db := latestBuildIDTestDB(t)
	accountStore := account.NewAccountStore(db)
	index := agentindex.NewIndexWithDB(db)
	deployStore := ds.NewStore(db)

	userID := "user-latest-" + deployid.New()
	viewerName := "lb-viewer-" + strings.ToLower(deployid.New())
	viewerAcct, err := accountStore.Create(viewerName, "personal", userID, "Viewer")
	if err != nil {
		t.Fatalf("create viewer account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", viewerAcct.ID) })

	publisherName := "lb-pub-" + strings.ToLower(deployid.New())
	publisherAcct, err := accountStore.Create(publisherName, "organization", "lb-pub-owner-"+deployid.New(), "Publisher Org")
	if err != nil {
		t.Fatalf("create publisher account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", publisherAcct.ID) })

	// Each agent name is unique per fixture so concurrent tests don't collide.
	suffix := strings.ToLower(deployid.New()[:6])
	stalePublicAgent := "lb-stale-pub-" + suffix
	currentPublicAgent := "lb-current-pub-" + suffix
	samePrivateAgent := "lb-same-private-" + suffix
	xacctPublicAgent := "lb-xacct-pub-" + suffix
	xacctPublicCurrentAgent := "lb-xacct-pub-cur-" + suffix
	xacctPrivateAgent := "lb-xacct-private-" + suffix
	missingVersionsAgent := "lb-no-versions-" + suffix
	archivedFromPublishAgent := "lb-pub-no-versions-" + suffix

	// Build IDs — readable enough that test failures point you at the right scenario.
	stalePubV1 := "build-stale-pub-v1-" + suffix
	stalePubV2 := "build-stale-pub-v2-" + suffix
	currentPubV1 := "build-current-pub-v1-" + suffix
	samePrivV1 := "build-same-priv-v1-" + suffix
	samePrivV2 := "build-same-priv-v2-" + suffix
	xacctPubV1 := "build-xacct-pub-v1-" + suffix
	xacctPubV2 := "build-xacct-pub-v2-" + suffix
	xacctPubCurrentV1 := "build-xacct-pub-cur-v1-" + suffix
	xacctPrivV1 := "build-xacct-priv-v1-" + suffix
	xacctPrivV2 := "build-xacct-priv-v2-" + suffix
	missingDeployBuild := "ghost-build-" + suffix

	// Same-account public, two versions oldest -> newest.
	registerVersion(t, db, index, viewerAcct, stalePublicAgent, stalePubV1, 0)
	registerVersion(t, db, index, viewerAcct, stalePublicAgent, stalePubV2, 1)
	setVisibility(t, index, viewerAcct.ID, stalePublicAgent, "public")

	// Same-account public, single version. Sentinel for "no upgrade signal" with a non-empty latest.
	registerVersion(t, db, index, viewerAcct, currentPublicAgent, currentPubV1, 0)
	setVisibility(t, index, viewerAcct.ID, currentPublicAgent, "public")

	// Same-account PRIVATE: visibility gate must NOT suppress same-account upgrades.
	// canDeploySourceAgent only blocks private blueprints crossing account
	// boundaries; in-account deploys are fine, so the badge must still light up.
	registerVersion(t, db, index, viewerAcct, samePrivateAgent, samePrivV1, 0)
	registerVersion(t, db, index, viewerAcct, samePrivateAgent, samePrivV2, 1)
	setVisibility(t, index, viewerAcct.ID, samePrivateAgent, "private")

	// Cross-account public, two versions. Real upgrade signal.
	registerVersion(t, db, index, publisherAcct, xacctPublicAgent, xacctPubV1, 0)
	registerVersion(t, db, index, publisherAcct, xacctPublicAgent, xacctPubV2, 1)
	setVisibility(t, index, publisherAcct.ID, xacctPublicAgent, "public")

	// Cross-account public, single version. Sentinel for "latest_build_id == build_id".
	registerVersion(t, db, index, publisherAcct, xacctPublicCurrentAgent, xacctPubCurrentV1, 0)
	setVisibility(t, index, publisherAcct.ID, xacctPublicCurrentAgent, "public")

	// Cross-account PRIVATE, two versions. THE regression guard.
	// agent_versions actually has a newer build, so a non-suppressing handler
	// would write XACCT_PRIVATE_V2 into latest_build_id and the dashboard
	// would render an "Update available" badge for an upgrade the deploy
	// endpoint would reject.
	registerVersion(t, db, index, publisherAcct, xacctPrivateAgent, xacctPrivV1, 0)
	registerVersion(t, db, index, publisherAcct, xacctPrivateAgent, xacctPrivV2, 1)
	setVisibility(t, index, publisherAcct.ID, xacctPrivateAgent, "private")

	// Same-account agent that never published a version. The handler should
	// quietly omit latest_build_id rather than choke on the lookup miss.
	registerAgentNoVersion(t, db, viewerAcct, missingVersionsAgent)
	setVisibility(t, index, viewerAcct.ID, missingVersionsAgent, "public")

	// Cross-account agent that has been registered (so the agents row exists)
	// but has no versions — visibility lookup succeeds, batch lookup misses.
	registerAgentNoVersion(t, db, publisherAcct, archivedFromPublishAgent)
	setVisibility(t, index, publisherAcct.ID, archivedFromPublishAgent, "public")

	// Deployments. Each one is on the *oldest* build in its lineage so the
	// "stale" scenarios reliably differ from latest.
	depSameAcctStale := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: viewerAcct.ID,
		AgentName:       stalePublicAgent,
		DisplayName:     "Same-Acct Stale",
		BuildID:         stalePubV1,
		Namespace:       "astro-lb-stale-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(viewerAcct.Name, viewerAcct.Name, stalePublicAgent, stalePubV1),
	})
	depSameAcctCurrent := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: viewerAcct.ID,
		AgentName:       currentPublicAgent,
		DisplayName:     "Same-Acct Current",
		BuildID:         currentPubV1,
		Namespace:       "astro-lb-current-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(viewerAcct.Name, viewerAcct.Name, currentPublicAgent, currentPubV1),
	})
	depSameAcctPrivateStale := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: viewerAcct.ID,
		AgentName:       samePrivateAgent,
		DisplayName:     "Same-Acct Private Stale",
		BuildID:         samePrivV1,
		Namespace:       "astro-lb-priv-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(viewerAcct.Name, viewerAcct.Name, samePrivateAgent, samePrivV1),
	})
	depXAcctPublicStale := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: publisherAcct.ID,
		AgentName:       xacctPublicAgent,
		DisplayName:     "X-Acct Public Stale",
		BuildID:         xacctPubV1,
		Namespace:       "astro-lb-xpub-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(publisherAcct.Name, viewerAcct.Name, xacctPublicAgent, xacctPubV1),
	})
	depXAcctPublicCurrent := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: publisherAcct.ID,
		AgentName:       xacctPublicCurrentAgent,
		DisplayName:     "X-Acct Public Current",
		BuildID:         xacctPubCurrentV1,
		Namespace:       "astro-lb-xpubc-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(publisherAcct.Name, viewerAcct.Name, xacctPublicCurrentAgent, xacctPubCurrentV1),
	})
	depXAcctPrivateStale := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: publisherAcct.ID,
		AgentName:       xacctPrivateAgent,
		DisplayName:     "X-Acct Private Stale",
		BuildID:         xacctPrivV1,
		Namespace:       "astro-lb-xpriv-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(publisherAcct.Name, viewerAcct.Name, xacctPrivateAgent, xacctPrivV1),
	})
	depAgentMissingFromIndex := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: viewerAcct.ID,
		AgentName:       missingVersionsAgent,
		DisplayName:     "Same-Acct Missing Versions",
		BuildID:         missingDeployBuild,
		Namespace:       "astro-lb-missing-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(viewerAcct.Name, viewerAcct.Name, missingVersionsAgent, missingDeployBuild),
	})
	depAgentArchivedFromPublish := saveLatestBuildIDDeployment(t, deployStore, ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       viewerAcct.ID,
		SourceAccountID: publisherAcct.ID,
		AgentName:       archivedFromPublishAgent,
		DisplayName:     "X-Acct Missing Versions",
		BuildID:         missingDeployBuild,
		Namespace:       "astro-lb-xmissing-" + deployid.Compact(deployid.New()),
		SpecJSON:        buildSourceAccountDeploymentSpecJSON(publisherAcct.Name, viewerAcct.Name, archivedFromPublishAgent, missingDeployBuild),
	})

	return &latestBuildIDFixture{
		db:                          db,
		router:                      newLatestBuildIDRouter(t, userID, accountStore, index, deployStore),
		userID:                      userID,
		viewerAcct:                  viewerAcct,
		publisherAcct:               publisherAcct,
		depSameAcctStale:            depSameAcctStale,
		depSameAcctCurrent:          depSameAcctCurrent,
		depSameAcctPrivateStale:     depSameAcctPrivateStale,
		depXAcctPublicStale:         depXAcctPublicStale,
		depXAcctPublicCurrent:       depXAcctPublicCurrent,
		depXAcctPrivateStale:        depXAcctPrivateStale,
		depAgentMissingFromIndex:    depAgentMissingFromIndex,
		depAgentArchivedFromPublish: depAgentArchivedFromPublish,
		stalePublicLatest:           stalePubV2,
		currentPublicLatest:         currentPubV1,
		samePrivateLatest:           samePrivV2,
		xacctPublicLatest:           xacctPubV2,
		xacctPublicLone:             xacctPubCurrentV1,
		xacctPrivateLatestRaw:       xacctPrivV2,
		deployedStaleBuildID:        stalePubV1,
		deployedCurrentBuildID:      currentPubV1,
		deployedPrivateBuildID:      samePrivV1,
		deployedXAcctOldBuild:       xacctPubV1,
	}
}

func latestBuildIDTestDB(t *testing.T) *sql.DB {
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

// registerVersion inserts an agent + agent_versions row and then forces
// published_at to a deterministic offset from a fixed base. The handler's
// BatchLatestBuildIDs query orders by published_at DESC; relying on time.Now()
// across two back-to-back Register calls is flaky on fast machines because
// Postgres timestamps can collide at the microsecond.
func registerVersion(t *testing.T, db *sql.DB, index *agentindex.Index, acct *account.Account, name, buildID string, orderHint int) {
	t.Helper()
	registerAgent(t, index, acct, name, buildID)
	// Spread published_at by minutes so even sloppy clock skew won't reorder them.
	pub := time.Date(2026, 1, 1, 0, orderHint, 0, 0, time.UTC)
	if _, err := db.Exec(
		`UPDATE agent_versions SET published_at = $1 WHERE account_id = $2 AND name = $3 AND build_id = $4`,
		pub, acct.ID, name, buildID,
	); err != nil {
		t.Fatalf("force published_at on %s/%s@%s: %v", acct.Name, name, buildID, err)
	}
}

// registerAgentNoVersion inserts only the agents row, leaving agent_versions
// empty. Mirrors the state of an agent that's been registered (e.g. by the
// blueprint-create flow) but never had a build pushed to it.
func registerAgentNoVersion(t *testing.T, db *sql.DB, acct *account.Account, name string) {
	t.Helper()
	now := time.Now()
	if _, err := db.Exec(
		`INSERT INTO agents (account_id, name, registry, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (account_id, name) DO UPDATE SET registry = $3, updated_at = $5, archived_at = NULL`,
		acct.ID, name, "registry.io", now, now,
	); err != nil {
		t.Fatalf("insert agents row %s/%s: %v", acct.Name, name, err)
	}
}

func setVisibility(t *testing.T, index *agentindex.Index, accountID, name, visibility string) {
	t.Helper()
	if err := index.SetVisibility(accountID, name, visibility); err != nil {
		t.Fatalf("set visibility %s on %s: %v", visibility, name, err)
	}
}

func saveLatestBuildIDDeployment(t *testing.T, store *ds.Store, p ds.SaveDeploymentParams) *ds.Deployment {
	t.Helper()
	dep, err := store.SaveDeploymentPending(p, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending(%s): %v", p.AgentName, err)
	}
	return dep
}

// newLatestBuildIDRouter wires the real ListDeployments handler with a real
// agentindex.Index so populateLatestBuildIDs actually runs. The previous
// source_account_attribution fixture passes nil for agentIdx because that
// test only cares about source_account; here we *need* the index so the
// visibility gate exercises real SQL.
func newLatestBuildIDRouter(t *testing.T, userID string, accountStore *account.AccountStore, index *agentindex.Index, deployStore *ds.Store) *gin.Engine {
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
		log, accountStore, cfg, k8s.NewRegistryWithPrimary(k8sClient), deployStore, index, nil, nil, k8scache.NoopCache{},
	))
	return r
}

// listLatestBuildIDDeployments hits ListDeployments and returns each
// deployment as a json.RawMessage map so tests can assert on the exact
// on-the-wire JSON (including json:"latest_build_id,omitempty" semantics:
// an empty value must be absent from the payload, not present-but-empty).
func listLatestBuildIDDeployments(t *testing.T, fx *latestBuildIDFixture) map[string]map[string]json.RawMessage {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments?account="+fx.viewerAcct.Name, nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListDeployments: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp listDeploymentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return indexByID(resp.Deployments)
}

func mustDep(t *testing.T, byID map[string]map[string]json.RawMessage, depID, label string) map[string]json.RawMessage {
	t.Helper()
	dep, ok := byID[depID]
	if !ok {
		t.Fatalf("%s deployment %q missing from response (got %d deployments)", label, depID, len(byID))
	}
	return dep
}

// hasField returns true when the JSON object actually carries the field.
// json.RawMessage is nil when absent, so this distinguishes "field omitted"
// from "field present with empty string" — the omitempty/missing distinction
// is exactly what the dashboard's hasNewBuildAvailable check depends on.
func hasField(dep map[string]json.RawMessage, field string) bool {
	raw, ok := dep[field]
	return ok && len(raw) > 0
}

// TestListDeploymentsE2E_LatestBuildID_SameAccountStale asserts the bread-and-
// butter case: a deployment running an older build of an in-account agent
// reports the newest published build_id so the dashboard can render the
// upgrade badge. Without this the dashboard never lights up at all.
func TestListDeploymentsE2E_LatestBuildID_SameAccountStale(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	dep := mustDep(t, byID, fx.depSameAcctStale.ID, "same-account stale")
	got := decodeString(t, dep["latest_build_id"], "latest_build_id")
	if got != fx.stalePublicLatest {
		t.Errorf("same-account stale deployment: latest_build_id = %q, want %q (newest published build)",
			got, fx.stalePublicLatest)
	}
	if buildID := decodeString(t, dep["build_id"], "build_id"); buildID == got {
		t.Errorf("same-account stale deployment: build_id and latest_build_id both = %q; the dashboard would never render the upgrade badge",
			got)
	}
}

// TestListDeploymentsE2E_LatestBuildID_SameAccountCurrent asserts that an
// up-to-date deployment carries latest_build_id = build_id (not omitted), so
// the client can affirmatively know "you are current" instead of treating an
// absent field as "no signal available".
func TestListDeploymentsE2E_LatestBuildID_SameAccountCurrent(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	dep := mustDep(t, byID, fx.depSameAcctCurrent.ID, "same-account current")
	got := decodeString(t, dep["latest_build_id"], "latest_build_id")
	if got != fx.currentPublicLatest {
		t.Errorf("same-account current: latest_build_id = %q, want %q", got, fx.currentPublicLatest)
	}
	if buildID := decodeString(t, dep["build_id"], "build_id"); buildID != got {
		t.Errorf("same-account current: build_id %q != latest_build_id %q; an up-to-date deployment must report parity",
			buildID, got)
	}
}

// TestListDeploymentsE2E_LatestBuildID_SameAccountPrivate asserts the
// visibility gate ONLY blocks cross-account private blueprints. Same-account
// private deploys are first-class and must still see upgrade signals — a
// regression on the gate's branch condition would silently hide all private
// in-account upgrades.
func TestListDeploymentsE2E_LatestBuildID_SameAccountPrivate(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	dep := mustDep(t, byID, fx.depSameAcctPrivateStale.ID, "same-account private stale")
	got := decodeString(t, dep["latest_build_id"], "latest_build_id")
	if got != fx.samePrivateLatest {
		t.Errorf("same-account PRIVATE stale: latest_build_id = %q, want %q. The visibility gate must not affect same-account deploys (canDeploySourceAgent allows them).",
			got, fx.samePrivateLatest)
	}
}

// TestListDeploymentsE2E_LatestBuildID_CrossAccountPublic guards the original
// cross-account fix: a deployment whose source_account_id points at a
// different (public) publisher must surface the publisher's latest build, not
// the viewer-account same-named blueprint or the deployment's own pinned
// build.
func TestListDeploymentsE2E_LatestBuildID_CrossAccountPublic(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	dep := mustDep(t, byID, fx.depXAcctPublicStale.ID, "cross-account public stale")
	got := decodeString(t, dep["latest_build_id"], "latest_build_id")
	if got != fx.xacctPublicLatest {
		t.Errorf("cross-account public stale: latest_build_id = %q, want %q (publisher's newest build)",
			got, fx.xacctPublicLatest)
	}

	currentDep := mustDep(t, byID, fx.depXAcctPublicCurrent.ID, "cross-account public current")
	currentGot := decodeString(t, currentDep["latest_build_id"], "latest_build_id")
	if currentGot != fx.xacctPublicLone {
		t.Errorf("cross-account public current: latest_build_id = %q, want %q",
			currentGot, fx.xacctPublicLone)
	}
}

// TestListDeploymentsE2E_LatestBuildID_CrossAccountPrivateSuppressed is the
// regression guard for the bug this whole audit caught: populateLatestBuildIDs'
// doc comment promised that private cross-account agents are suppressed, but
// the implementation didn't enforce it. The dashboard then advertised an
// "Update available" badge whose Redeploy was guaranteed to 404 because
// canDeploySourceAgent refuses private blueprints across the boundary.
//
// agent_versions has a strictly newer publisher build for this deployment,
// so a non-suppressing handler would happily expose it. We assert the
// handler omits latest_build_id entirely (not present-but-equal-to-build_id —
// the JSON tag is omitempty and the absence is what the client gates on).
func TestListDeploymentsE2E_LatestBuildID_CrossAccountPrivateSuppressed(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	dep := mustDep(t, byID, fx.depXAcctPrivateStale.ID, "cross-account private stale")
	if hasField(dep, "latest_build_id") {
		t.Fatalf("cross-account PRIVATE stale: latest_build_id present (=%s); must be suppressed because the deploy endpoint refuses private blueprints across account boundaries (canDeploySourceAgent). Without suppression the dashboard renders an 'Update available' badge whose Redeploy would 404.",
			string(dep["latest_build_id"]))
	}

	// Cross-check: the publisher's newest build *exists* in the index, so
	// the suppression is doing real work — not just hiding a vacuous lookup miss.
	if fx.xacctPrivateLatestRaw == fx.deployedXAcctOldBuild {
		t.Fatalf("fixture invariant broken: private cross-account agent has no newer build; this test isn't actually exercising the suppression branch")
	}
}

// TestListDeploymentsE2E_LatestBuildID_NoVersionsForAgent asserts that an
// agent without any agent_versions row produces an absent latest_build_id
// (not an empty string, not the deployment's own build_id) — matches the
// "no signal available" branch the client treats as "stay silent".
func TestListDeploymentsE2E_LatestBuildID_NoVersionsForAgent(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	for _, c := range []struct {
		dep   *ds.Deployment
		label string
	}{
		{fx.depAgentMissingFromIndex, "same-account no-versions"},
		{fx.depAgentArchivedFromPublish, "cross-account no-versions"},
	} {
		dep := mustDep(t, byID, c.dep.ID, c.label)
		if hasField(dep, "latest_build_id") {
			t.Errorf("%s: latest_build_id present (=%s); want field omitted when agent has no published versions",
				c.label, string(dep["latest_build_id"]))
		}
	}
}

// TestListDeploymentsE2E_LatestBuildID_AllScenariosInOneCall asserts the
// shape of the entire response in a single GET, so a cross-deployment leak
// (e.g. mis-keyed map writing one deployment's latest into another) fails
// here even when individual single-deployment tests pass.
func TestListDeploymentsE2E_LatestBuildID_AllScenariosInOneCall(t *testing.T) {
	fx := setupLatestBuildIDFixture(t)
	byID := listLatestBuildIDDeployments(t, fx)

	checks := []struct {
		dep        *ds.Deployment
		wantLatest string
		wantField  bool
		label      string
	}{
		{fx.depSameAcctStale, fx.stalePublicLatest, true, "same-account stale"},
		{fx.depSameAcctCurrent, fx.currentPublicLatest, true, "same-account current"},
		{fx.depSameAcctPrivateStale, fx.samePrivateLatest, true, "same-account private stale"},
		{fx.depXAcctPublicStale, fx.xacctPublicLatest, true, "cross-account public stale"},
		{fx.depXAcctPublicCurrent, fx.xacctPublicLone, true, "cross-account public current"},
		{fx.depXAcctPrivateStale, "", false, "cross-account private stale (suppressed)"},
		{fx.depAgentMissingFromIndex, "", false, "same-account no-versions"},
		{fx.depAgentArchivedFromPublish, "", false, "cross-account no-versions"},
	}

	for _, c := range checks {
		dep := mustDep(t, byID, c.dep.ID, c.label)
		got := decodeString(t, dep["latest_build_id"], "latest_build_id")
		present := hasField(dep, "latest_build_id")
		if present != c.wantField {
			t.Errorf("%s (id=%s): latest_build_id present=%v, want present=%v (got=%q)",
				c.label, c.dep.ID, present, c.wantField, got)
			continue
		}
		if c.wantField && got != c.wantLatest {
			t.Errorf("%s (id=%s): latest_build_id = %q, want %q",
				c.label, c.dep.ID, got, c.wantLatest)
		}
	}
}
