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

// devKeyColumns mirrors the SELECT in DevStore.Get — keep in sync if the
// dev-key schema or Get() projection changes.
var devKeyColumns = []string{
	"account_id", "user_id", "key_id", "encrypted_api_key",
	"encrypted_data_key", "nonce", "expires_at", "created_at", "updated_at",
}

// fakeGateway returns a test LiteLLM stub that counts /key/generate hits.
// Each call returns the same (sk-bf-fresh, tok-fresh) — tests assert
// the number of generate calls to distinguish reuse vs mint paths.
func fakeGateway(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var generateCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/virtual-keys":
			atomic.AddInt32(&generateCalls, 1)
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
	return srv, &generateCalls
}

func TestEnsureDevKey_MintsFreshWhenAbsent(t *testing.T) {
	srv, generateCalls := fakeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// Get → no row.
	mock.ExpectQuery("SELECT account_id, user_id, key_id").
		WithArgs("acct-1", "user-7").
		WillReturnRows(sqlmock.NewRows(devKeyColumns))
	// Upsert returns NULL previous (first time).
	mock.ExpectQuery("WITH existing AS").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow(nil))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	devStore := NewDevStore(db)

	apiKey, baseURL, expiresAt, err := provisioner.EnsureDevKey(
		context.Background(), devStore, "", nil,
		"acct-1", "user-7",
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-bf-fresh", apiKey)
	assert.Equal(t, srv.URL, baseURL)
	assert.True(t, time.Until(expiresAt) > 7*time.Hour, "expires_at should be ~8h out")
	assert.Equal(t, int32(1), atomic.LoadInt32(generateCalls),
		"first-time mint must call /key/generate exactly once")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDevKey_ReusesWhenNotExpired(t *testing.T) {
	srv, generateCalls := fakeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	plaintext := "sk-astro-existing"
	expiresAt := time.Now().Add(4 * time.Hour) // well above the safety margin

	mock.ExpectQuery("SELECT account_id, user_id, key_id").
		WithArgs("acct-1", "user-7").
		WillReturnRows(sqlmock.NewRows(devKeyColumns).AddRow(
			"acct-1", "user-7", "tok-existing", plaintext,
			nil, nil, expiresAt, time.Now(), time.Now(),
		))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	devStore := NewDevStore(db)

	apiKey, baseURL, gotExpiresAt, err := provisioner.EnsureDevKey(
		context.Background(), devStore, "", nil,
		"acct-1", "user-7",
	)
	require.NoError(t, err)
	assert.Equal(t, plaintext, apiKey, "reuse path must return the existing plaintext")
	assert.Equal(t, srv.URL, baseURL)
	assert.WithinDuration(t, expiresAt, gotExpiresAt, time.Second)
	assert.Equal(t, int32(0), atomic.LoadInt32(generateCalls),
		"reuse path must not call /key/generate")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDevKey_ReplacesWhenExpiring(t *testing.T) {
	srv, generateCalls := fakeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// Expires in 10m — under DevKeySafetyMargin (30m), treat as expired.
	expiresSoon := time.Now().Add(10 * time.Minute)

	mock.ExpectQuery("SELECT account_id, user_id, key_id").
		WithArgs("acct-1", "user-7").
		WillReturnRows(sqlmock.NewRows(devKeyColumns).AddRow(
			"acct-1", "user-7", "tok-old", "sk-old",
			nil, nil, expiresSoon, time.Now(), time.Now(),
		))
	// Upsert returns the old key_id so the provisioner revokes upstream.
	mock.ExpectQuery("WITH existing AS").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("tok-old"))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	devStore := NewDevStore(db)

	apiKey, _, _, err := provisioner.EnsureDevKey(
		context.Background(), devStore, "", nil,
		"acct-1", "user-7",
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-bf-fresh", apiKey)
	assert.Equal(t, int32(1), atomic.LoadInt32(generateCalls),
		"expiring-key path must mint exactly one fresh key")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureDevKey_PerUserIsolation(t *testing.T) {
	// Two users on the same account get independent keys — alice's row
	// being absent doesn't affect bob's lookup, and vice versa.
	srv, _ := fakeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// alice: no row → mint
	mock.ExpectQuery("SELECT account_id, user_id, key_id").
		WithArgs("acct-1", "alice").
		WillReturnRows(sqlmock.NewRows(devKeyColumns))
	mock.ExpectQuery("WITH existing AS").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow(nil))

	// bob: pre-existing usable key → reuse
	expiresAt := time.Now().Add(4 * time.Hour)
	mock.ExpectQuery("SELECT account_id, user_id, key_id").
		WithArgs("acct-1", "bob").
		WillReturnRows(sqlmock.NewRows(devKeyColumns).AddRow(
			"acct-1", "bob", "tok-bob", "sk-bob",
			nil, nil, expiresAt, time.Now(), time.Now(),
		))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	devStore := NewDevStore(db)

	aliceKey, _, _, err := provisioner.EnsureDevKey(
		context.Background(), devStore, "", nil, "acct-1", "alice",
	)
	require.NoError(t, err)
	bobKey, _, _, err := provisioner.EnsureDevKey(
		context.Background(), devStore, "", nil, "acct-1", "bob",
	)
	require.NoError(t, err)

	assert.Equal(t, "sk-bf-fresh", aliceKey)
	assert.Equal(t, "sk-bob", bobKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevokeAccountDevKeys_SweepsUpstreamThenDeletesRows(t *testing.T) {
	// Counts /key/delete calls so the test can prove the upstream sweep
	// happens for every row before the local rows are dropped — without
	// the sweep, the FK cascade on account hard-delete would leave the
	// LiteLLM keys lingering until their 8h TTL.
	var deleteCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/governance/virtual-keys/") {
			atomic.AddInt32(&deleteCalls, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
	}))
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT key_id FROM account_ai_gateway_dev_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).
			AddRow("tok-alice").
			AddRow("tok-bob"))
	mock.ExpectExec("DELETE FROM account_ai_gateway_dev_keys").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 2))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	devStore := NewDevStore(db)

	err = provisioner.RevokeAccountDevKeys(context.Background(), devStore, "acct-1")
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&deleteCalls),
		"every dev key row must trigger an upstream /key/delete")
	require.NoError(t, mock.ExpectationsWereMet())
}
