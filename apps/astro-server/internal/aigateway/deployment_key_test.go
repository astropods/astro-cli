package aigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// fakeCustomerStore is an in-memory CustomerStore for provisioner tests.
type fakeCustomerStore struct{ ids map[string]string }

func newFakeCustomerStore() *fakeCustomerStore { return &fakeCustomerStore{ids: map[string]string{}} }

func (f *fakeCustomerStore) GetBifrostCustomerID(accountID string) (string, error) {
	return f.ids[accountID], nil
}

func (f *fakeCustomerStore) SetBifrostCustomerID(accountID, customerID string) error {
	f.ids[accountID] = customerID
	return nil
}

// fakeDeploymentGateway returns a test LiteLLM stub that records the
// /key/generate body so tests can assert metadata content.
func fakeDeploymentGateway(t *testing.T) (*httptest.Server, *int32, *bifrostVKRequest) {
	t.Helper()
	var generateCalls int32
	var captured bifrostVKRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/virtual-keys":
			atomic.AddInt32(&generateCalls, 1)
			_ = json.NewDecoder(r.Body).Decode(&captured)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"virtual_key": map[string]string{"id": "tok-fresh", "value": "sk-bf-fresh"},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/governance/virtual-keys/"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/customers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"customer": map[string]string{"id": "cust-fresh", "name": "acct"},
			})
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
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
		WithArgs("dep-1", "acct-1", "tok-fresh", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	store := NewStore(db)

	apiKey, baseURL, err := provisioner.EnsureDeploymentKey(
		context.Background(), store, testVault(t),
		DeploymentKeyParams{
			AccountID:    "acct-1",
			DeploymentID: "dep-1",
			ClusterID:    "cluster-a",
			AgentName:    "support-bot",
			AgentVersion: "v1.2.3",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-bf-fresh", apiKey)
	assert.Equal(t, srv.URL, baseURL)
	assert.Equal(t, int32(1), atomic.LoadInt32(generateCalls))

	// account-id rides in the VK name (attribution); deployment scope + cluster
	// land in name/description so they're visible in the Bifrost admin view.
	assert.Contains(t, captured.Name, "acct-1")
	assert.Contains(t, captured.Name, "deployment:dep-1")
	assert.Contains(t, captured.Description, "cluster-a")
	assert.Contains(t, captured.Description, "support-bot")
	assert.Contains(t, captured.Description, "v1.2.3")

	// Grant is Bedrock with all provider keys.
	assert.Len(t, captured.ProviderConfigs, 1)
	assert.Equal(t, "bedrock", captured.ProviderConfigs[0].Provider)
	assert.Equal(t, []string{"*"}, captured.ProviderConfigs[0].KeyIDs)

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

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	store := NewStore(db)

	apiKey, _, err := provisioner.EnsureDeploymentKey(
		context.Background(), store, testVault(t),
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
	provisioner := NewProvisioner(NewClient("http://nope", "", ""), newFakeCustomerStore(), nil)
	store := NewStore(nil)

	_, _, err := provisioner.EnsureDeploymentKey(
		context.Background(), store, testVault(t),
		DeploymentKeyParams{DeploymentID: "dep-1"},
	)
	assert.Error(t, err, "missing AccountID should fail")

	_, _, err = provisioner.EnsureDeploymentKey(
		context.Background(), store, testVault(t),
		DeploymentKeyParams{AccountID: "acct-1"},
	)
	assert.Error(t, err, "missing DeploymentID should fail")
}

func TestRevokeDeploymentKey_DeletesUpstreamAndRow(t *testing.T) {
	var deleteCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/governance/virtual-keys/") {
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

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
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

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	store := NewStore(db)
	require.NoError(t, provisioner.RevokeDeploymentKey(context.Background(), store, "dep-missing"))
	require.NoError(t, mock.ExpectationsWereMet())
}
