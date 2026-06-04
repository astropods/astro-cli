package aigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deploymentKeyColumns mirrors the SELECT in Store.Get — keep in sync if
// the table or projection changes.
var deploymentKeyColumns = []string{
	"deployment_id", "account_id", "key_id", "encrypted_api_key",
	"encrypted_data_key", "nonce", "issued_at", "created_at", "updated_at",
}

// fakeDeploymentGateway returns a test LiteLLM stub that records the
// /key/generate body so tests can assert metadata content.
func fakeDeploymentGateway(t *testing.T) (*httptest.Server, *int32, *KeyRequest) {
	t.Helper()
	var generateCalls int32
	var captured KeyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/key/generate":
			atomic.AddInt32(&generateCalls, 1)
			_ = json.NewDecoder(r.Body).Decode(&captured)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"key":   "sk-astro-fresh",
				"token": "tok-fresh",
			})
		case "/key/delete":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusBadRequest)
		}
	}))
	return srv, &generateCalls, &captured
}

func TestEnsureDeploymentKey_MintsWhenAbsentAndStampsMetadata(t *testing.T) {
	srv, generateCalls, captured := fakeDeploymentGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// Get → no row.
	mock.ExpectQuery("SELECT deployment_id, account_id, key_id").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentKeyColumns))
	// Save (insert).
	mock.ExpectExec("INSERT INTO deployment_ai_gateway").
		WithArgs("dep-1", "acct-1", "tok-fresh", "sk-astro-fresh", []byte(nil), []byte(nil), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	provisioner := NewProvisioner(NewClient(srv.URL, "master"))
	store := NewStore(db)

	apiKey, baseURL, err := provisioner.EnsureDeploymentKey(
		context.Background(), store, "", nil,
		DeploymentKeyParams{
			AccountID:    "acct-1",
			DeploymentID: "dep-1",
			ClusterID:    "cluster-a",
			AgentName:    "support-bot",
			AgentVersion: "v1.2.3",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-astro-fresh", apiKey)
	assert.Equal(t, srv.URL, baseURL)
	assert.Equal(t, int32(1), atomic.LoadInt32(generateCalls))

	// UserID/TeamID are the account-id — load-bearing for OpenMeter attribution.
	assert.Equal(t, "acct-1", captured.UserID)
	assert.Equal(t, "acct-1", captured.TeamID)

	// Metadata: deployment-scoped tags + cluster_id.
	tags, _ := captured.Metadata["tags"].([]any)
	tagStrs := make([]string, 0, len(tags))
	for _, t := range tags {
		if s, ok := t.(string); ok {
			tagStrs = append(tagStrs, s)
		}
	}
	assert.Contains(t, tagStrs, "deployment:dep-1")
	assert.Contains(t, tagStrs, "agent:support-bot")
	assert.Contains(t, tagStrs, "version:v1.2.3")
	assert.Equal(t, "cluster-a", captured.Metadata["cluster_id"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDeploymentKey_IsIdempotentOnExistingRow(t *testing.T) {
	srv, generateCalls, _ := fakeDeploymentGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// Get → existing row with plaintext key (no KMS in test).
	mock.ExpectQuery("SELECT deployment_id, account_id, key_id").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentKeyColumns).AddRow(
			"dep-1", "acct-1", "tok-existing", "sk-astro-existing",
			nil, nil, time.Now(), time.Now(), time.Now(),
		))

	provisioner := NewProvisioner(NewClient(srv.URL, "master"))
	store := NewStore(db)

	apiKey, _, err := provisioner.EnsureDeploymentKey(
		context.Background(), store, "", nil,
		DeploymentKeyParams{AccountID: "acct-1", DeploymentID: "dep-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-astro-existing", apiKey,
		"reuse path must return the stored plaintext, not mint a new key")
	assert.Equal(t, int32(0), atomic.LoadInt32(generateCalls),
		"reuse path must not call /key/generate")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDeploymentKey_RequiresAccountAndDeployment(t *testing.T) {
	provisioner := NewProvisioner(NewClient("http://nope", "master"))
	store := NewStore(nil)

	_, _, err := provisioner.EnsureDeploymentKey(
		context.Background(), store, "", nil,
		DeploymentKeyParams{DeploymentID: "dep-1"},
	)
	assert.Error(t, err, "missing AccountID should fail")

	_, _, err = provisioner.EnsureDeploymentKey(
		context.Background(), store, "", nil,
		DeploymentKeyParams{AccountID: "acct-1"},
	)
	assert.Error(t, err, "missing DeploymentID should fail")
}

func TestRevokeDeploymentKey_DeletesUpstreamAndRow(t *testing.T) {
	var deleteCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/key/delete" {
			deleteCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT deployment_id, account_id, key_id").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentKeyColumns).AddRow(
			"dep-1", "acct-1", "tok-existing", "sk-astro-existing",
			nil, nil, time.Now(), time.Now(), time.Now(),
		))
	mock.ExpectExec("DELETE FROM deployment_ai_gateway").
		WithArgs("dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	provisioner := NewProvisioner(NewClient(srv.URL, "master"))
	store := NewStore(db)
	require.NoError(t, provisioner.RevokeDeploymentKey(context.Background(), store, "dep-1"))
	assert.Equal(t, int32(1), deleteCalls.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevokeDeploymentKey_NoopWhenRowMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream should not be called when row is absent")
	}))
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT deployment_id, account_id, key_id").
		WithArgs("dep-missing").
		WillReturnRows(sqlmock.NewRows(deploymentKeyColumns))

	provisioner := NewProvisioner(NewClient(srv.URL, "master"))
	store := NewStore(db)
	require.NoError(t, provisioner.RevokeDeploymentKey(context.Background(), store, "dep-missing"))
	require.NoError(t, mock.ExpectationsWereMet())
}
