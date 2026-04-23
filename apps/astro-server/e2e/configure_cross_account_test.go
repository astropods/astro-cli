//go:build integration

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/handlers"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// These tests exercise the POST /agents/:account/:name/deployment-template
// handler end-to-end against a real Postgres. They cover the three shapes
// deployment_spec_json can take for the Configure prefill path:
//
//  1. Cross-account — spec.source.account differs from the URL :account.
//     The handler resolves the agent and its build under the source
//     account; auth is scoped to the deployment's (target) account.
//  2. Same-account — spec.source.account equals the URL :account.
//  3. Legacy — spec omits source.account entirely. The handler falls back
//     to the URL :account for the agent/build lookup.
//
// Each test asserts the externally observable template response
// (source.account, source.build, target.account) against real rows in
// accounts, agents, agent_versions, and deployments. Because the stores
// run against the real schema, any drift in SQL or store-side plumbing
// that sqlmock-based unit tests would miss surfaces here.

type configureE2EFixture struct {
	router      *gin.Engine
	userID      string
	sourceAcct  *account.Account
	targetAcct  *account.Account
	agentName   string
	pinnedBuild string
	deployment  *ds.Deployment
}

// setupConfigureE2E wires the real stores, creates the accounts/agent/builds
// requested by `crossAccount` / `sourceHasNewerBuild` / `visibility`, and
// inserts a deployment whose spec carries (or omits) source.account per the
// `includeSourceInSpec` flag. Returns a gin router ready to accept POSTs.
//
// Cleanup via t.Cleanup drops both accounts, which cascades the agents /
// agent_versions / deployments / deployment_revisions rows we created.
func setupConfigureE2E(
	t *testing.T,
	crossAccount bool,
	includeSourceInSpec bool,
	visibility string,
	sourceHasNewerBuild bool,
) *configureE2EFixture {
	t.Helper()

	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	index := agentindex.NewIndexWithDB(db)
	deployStore := ds.NewStore(db)

	userID := "user-e2e-" + deployid.New()
	targetName := "cfg-tgt-" + strings.ToLower(deployid.New())

	targetAcct, err := accountStore.Create(targetName, "organization", userID, "Target Org")
	if err != nil {
		t.Fatalf("create target account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", targetAcct.ID) })

	sourceAcct := targetAcct
	if crossAccount {
		sourceName := "cfg-src-" + strings.ToLower(deployid.New())
		sourceAcct, err = accountStore.Create(sourceName, "organization", "publisher-owner-"+deployid.New(), "Publisher")
		if err != nil {
			t.Fatalf("create source account: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", sourceAcct.ID) })
	}

	agentName := "cross-agent-" + strings.ToLower(deployid.New())
	pinnedBuild := "build-pinned-" + strings.ToLower(deployid.New()[:3])
	pinnedSpec := map[string]any{
		"name": agentName,
		"agent": map[string]any{
			"image": "example.io/" + sourceAcct.Name + "/" + agentName + ":" + pinnedBuild,
		},
	}
	if err := index.Register(sourceAcct.ID, agentName, pinnedBuild, "registry.io", sourceAcct.Name, pinnedSpec, "", "", ""); err != nil {
		t.Fatalf("register pinned build: %v", err)
	}
	if sourceHasNewerBuild {
		newerBuild := "build-new-" + strings.ToLower(deployid.New()[:3])
		newerSpec := map[string]any{
			"name": agentName,
			"agent": map[string]any{
				"image": "example.io/" + sourceAcct.Name + "/" + agentName + ":" + newerBuild,
			},
		}
		if err := index.Register(sourceAcct.ID, agentName, newerBuild, "registry.io", sourceAcct.Name, newerSpec, "", "", ""); err != nil {
			t.Fatalf("register newer build: %v", err)
		}
	}
	if visibility != "" {
		if err := index.SetVisibility(sourceAcct.ID, agentName, visibility); err != nil {
			t.Fatalf("SetVisibility: %v", err)
		}
	}

	depSpec := buildDeploymentSpecJSON(sourceAcct.Name, targetAcct.Name, agentName, pinnedBuild, includeSourceInSpec)
	deployment, err := deployStore.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:          deployid.New(),
		AccountID:   targetAcct.ID,
		AgentName:   agentName,
		DisplayName: "E2E Cross-Account Bot",
		BuildID:     pinnedBuild,
		Namespace:   "astro-" + deployid.Compact(deployid.New()) + "-0",
		SpecJSON:    depSpec,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	return &configureE2EFixture{
		router:      newConfigureE2ERouter(userID, index, accountStore, deployStore),
		userID:      userID,
		sourceAcct:  sourceAcct,
		targetAcct:  targetAcct,
		agentName:   agentName,
		pinnedBuild: pinnedBuild,
		deployment:  deployment,
	}
}

// buildDeploymentSpecJSON produces the deployment_spec_json stored for a
// deployment. When includeSource is false the "source" block is omitted so
// tests can cover the legacy shape where the publisher is not recorded in
// the spec.
func buildDeploymentSpecJSON(sourceAccount, targetAccount, agentName, buildID string, includeSource bool) string {
	payload := map[string]any{
		"spec": "deployment/v1",
		"target": map[string]any{
			"runtime": "kubernetes",
			"account": targetAccount,
		},
	}
	if includeSource {
		payload["source"] = map[string]any{
			"account": sourceAccount,
			"name":    agentName,
			"build":   buildID,
		}
	}
	out, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal deployment spec: %v", err))
	}
	return string(out)
}

// newConfigureE2ERouter wires PostDeploymentTemplate with the real stores and
// a middleware that stamps the test user onto every request.
func newConfigureE2ERouter(userID string, index *agentindex.Index, accountStore *account.AccountStore, deployStore *ds.Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
		c.Next()
	})
	r.POST("/agents/:account/:name/deployment-template",
		handlers.PostDeploymentTemplate(log, index, accountStore, cfg, deployStore))
	return r
}

// postConfigure POSTs a deployment-template request for the given URL path
// and returns the parsed response body plus the raw recorder.
func postConfigure(t *testing.T, fx *configureE2EFixture, urlAccount string) (*httptest.ResponseRecorder, spec.TemplateResponse) {
	t.Helper()
	body := fmt.Sprintf(`{"deployment_id":%q}`, fx.deployment.ID)
	req := httptest.NewRequest(
		http.MethodPost,
		"/agents/"+urlAccount+"/"+fx.agentName+"/deployment-template",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)

	var resp spec.TemplateResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v; body=%s", err, rec.Body.String())
		}
	}
	return rec, resp
}

// TestConfigureE2E_CrossAccountPrefill_ResolvesBuildFromSourceAccount covers
// the cross-account prefill path: a deployment lives in the target account's
// workspace, its spec's source.account points at a different publisher
// account, and the agent/build only exist under that publisher. The handler
// must resolve the build under the source account and return a template
// whose source.account matches the publisher and whose target.account
// matches the URL (workspace) account.
func TestConfigureE2E_CrossAccountPrefill_ResolvesBuildFromSourceAccount(t *testing.T) {
	fx := setupConfigureE2E(t, true, true, "public", false)

	rec, resp := postConfigure(t, fx, fx.targetAcct.Name)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.Template.Source.Account != fx.sourceAcct.Name {
		t.Errorf("source.account: expected %q (publisher), got %q",
			fx.sourceAcct.Name, resp.Template.Source.Account)
	}
	if resp.Template.Source.Build != fx.pinnedBuild {
		t.Errorf("source.build: expected %q, got %q", fx.pinnedBuild, resp.Template.Source.Build)
	}
	if resp.Template.Target.Account != fx.targetAcct.Name {
		t.Errorf("target.account: expected %q (workspace), got %q", fx.targetAcct.Name, resp.Template.Target.Account)
	}
	if resp.Template.Target.DeploymentID != fx.deployment.ID {
		t.Errorf("target.deployment_id: expected %q, got %q", fx.deployment.ID, resp.Template.Target.DeploymentID)
	}
}

// TestConfigureE2E_PrivateSourceAgent_TargetMemberOnlyCanOpen verifies the
// auth boundary for cross-account Configure. The test user is a member of
// only the target (deployment) account. The source agent is private in the
// publisher account. Configure must still succeed: authorization is scoped
// to the deployment's account, and source-account membership is not
// required once the deployment already exists.
func TestConfigureE2E_PrivateSourceAgent_TargetMemberOnlyCanOpen(t *testing.T) {
	fx := setupConfigureE2E(t, true, true, "private", false)

	rec, resp := postConfigure(t, fx, fx.targetAcct.Name)

	if rec.Code != http.StatusOK {
		t.Fatalf("target-only member should reach Configure for private source agent, got %d: %s",
			rec.Code, rec.Body.String())
	}
	if resp.Template.Source.Account != fx.sourceAcct.Name {
		t.Errorf("source.account: expected %q, got %q", fx.sourceAcct.Name, resp.Template.Source.Account)
	}
	if resp.Template.Source.Build != fx.pinnedBuild {
		t.Errorf("source.build: expected %q, got %q", fx.pinnedBuild, resp.Template.Source.Build)
	}
}

// TestConfigureE2E_CrossAccountPrefill_PinsDeployedBuild verifies that a
// cross-account Configure call returns the deployed build, not whatever the
// publisher has since shipped as "latest". The source account here has two
// versions — the pinned one and a newer one — and the template must match
// the pinned build.
func TestConfigureE2E_CrossAccountPrefill_PinsDeployedBuild(t *testing.T) {
	fx := setupConfigureE2E(t, true, true, "public", true)

	rec, resp := postConfigure(t, fx, fx.targetAcct.Name)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.Template.Source.Build != fx.pinnedBuild {
		t.Errorf("source.build: expected pinned %q, got %q (newer build leaked through)",
			fx.pinnedBuild, resp.Template.Source.Build)
	}
}

// TestConfigureE2E_SameAccountPrefill covers the common case where the
// deployment and the source agent live in the same account. The template's
// source.account matches the URL account and the pinned build is returned.
func TestConfigureE2E_SameAccountPrefill(t *testing.T) {
	fx := setupConfigureE2E(t, false, true, "public", false)

	rec, resp := postConfigure(t, fx, fx.targetAcct.Name)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.Template.Source.Account != fx.targetAcct.Name {
		t.Errorf("source.account should equal URL account %q, got %q",
			fx.targetAcct.Name, resp.Template.Source.Account)
	}
	if resp.Template.Source.Build != fx.pinnedBuild {
		t.Errorf("source.build: expected %q, got %q", fx.pinnedBuild, resp.Template.Source.Build)
	}
}

// TestConfigureE2E_LegacyPrefill_FallsBackToURLAccount covers deployments
// whose deployment_spec_json has no source block at all. With no publisher
// recorded, the handler resolves the agent and its build under the URL
// account. The agent is registered there so the lookup succeeds and the
// template reports the pinned build.
func TestConfigureE2E_LegacyPrefill_FallsBackToURLAccount(t *testing.T) {
	fx := setupConfigureE2E(t, false, false, "public", false)

	rec, resp := postConfigure(t, fx, fx.targetAcct.Name)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.Template.Source.Build != fx.pinnedBuild {
		t.Errorf("source.build: expected %q, got %q",
			fx.pinnedBuild, resp.Template.Source.Build)
	}
	if resp.Template.Target.Account != fx.targetAcct.Name {
		t.Errorf("target.account: expected %q, got %q",
			fx.targetAcct.Name, resp.Template.Target.Account)
	}
}
