//go:build integration

package e2e

import (
	"database/sql"
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
)

const configureInlineSecret = "sk-e2e-inline-configure-leak-test"

func minimalAPIKeySpec(agentName, buildID, accountName string) *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Account:  accountName,
			Name:     agentName,
			Build:    buildID,
			Registry: "test.ecr/repo",
		},
		Target: spec.DeploymentTarget{
			Runtime: "kubernetes",
			Account: accountName,
		},
		Agent: spec.DeploymentAgent{
			Image:     "test.ecr/repo/" + agentName + ":" + buildID,
			Replicas:  1,
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080, Protocol: "http"}},
		},
		Variables: map[string]spec.Variable{
			"API_KEY": {
				Value: configureInlineSecret, Secret: true, Optional: false,
				Targets: []string{"agent"},
			},
		},
	}
}

func saveInlineSecretDeployment(
	t *testing.T, db *sql.DB, store *ds.Store, accountID string, full *spec.AstroDeploymentSpec,
) *ds.Deployment {
	t.Helper()
	ns := "ns-inline-" + deployid.Compact(deployid.New())
	stripped := spec.StripSecretVariableValues(full)
	specJSON, err := json.Marshal(stripped)
	if err != nil {
		t.Fatalf("marshal stripped spec: %v", err)
	}
	dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:          deployid.New(),
		AccountID:   accountID,
		AgentName:   full.Source.Name,
		DisplayName: "Inline Secret Bot",
		BuildID:     full.Source.Build,
		Namespace:   ns,
		SpecJSON:    string(specJSON),
	}, func(tx *sql.Tx, depID string) error {
		return ds.SaveNormalizedSpec(tx, depID, full, nil, nil)
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	return dep
}

// TestConfigurePrefill_InlineSecret_NotInResponse verifies configure template
// prefill exposes configured metadata without returning the stored plaintext.
func TestConfigurePrefill_InlineSecret_NotInResponse(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	index := agentindex.NewIndexWithDB(db)
	deployStore := ds.NewStore(db)

	id := deployid.Compact(deployid.New())
	userID := "user-inline-" + id
	acctName := "inlinecfg" + id
	acct, err := accountStore.Create(acctName, "organization", userID, "Inline Configure")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	agentName := "inlineagt" + id
	buildID := "build-" + id[:6]
	agentSpec := map[string]any{
		"name": agentName,
		"agent": map[string]any{
			"image": "example.io/" + acctName + "/" + agentName + ":" + buildID,
			"inputs": []map[string]any{
				{"name": "API_KEY", "secret": true, "description": "API key"},
			},
		},
	}
	if err := index.Register(acct.ID, agentName, buildID, "registry.io", acctName, agentSpec, "", "", ""); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	full := minimalAPIKeySpec(agentName, buildID, acctName)
	dep := saveInlineSecretDeployment(t, db, deployStore, acct.ID, full)

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
		handlers.PostDeploymentTemplate(log, index, accountStore, cfg, deployStore, nil, nil, nil))

	body := fmt.Sprintf(`{"deployment_id":%q}`, dep.ID)
	req := httptest.NewRequest(http.MethodPost,
		"/agents/"+acctName+"/"+agentName+"/deployment-template",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, configureInlineSecret) {
		t.Fatal("configure template response must not contain inline secret plaintext")
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := resp.Variables["API_KEY"]
	if !ok {
		t.Fatalf("API_KEY missing from variables: %v", resp.Variables)
	}
	if !v.Configured {
		t.Error("expected API_KEY.Configured true")
	}
	if v.Value != "" {
		t.Errorf("expected empty API_KEY.Value, got %q", v.Value)
	}
	if tv, ok := resp.Template.Variables["API_KEY"]; ok && tv.Value != "" {
		t.Errorf("template.variables.API_KEY.Value: expected empty on prefill, got %q", tv.Value)
	}
}
