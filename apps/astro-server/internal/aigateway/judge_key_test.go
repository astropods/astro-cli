package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

type failingJudgeKMS struct{}

func (failingJudgeKMS) GenerateDataKey(context.Context, *kms.GenerateDataKeyInput, ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return nil, errors.New("kms unavailable")
}

func (failingJudgeKMS) Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return nil, errors.New("kms unavailable")
}

func judgeGateway(t *testing.T) (*httptest.Server, *bifrostVKRequest, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var captured bifrostVKRequest
	var creates atomic.Int32
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/virtual-keys":
			creates.Add(1)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"virtual_key": map[string]string{"id": "vk-judge", "value": "sk-bf-judge"},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/governance/virtual-keys/"):
			deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
		}
	}))
	return srv, &captured, &creates, &deletes
}

func TestEnsureJudgeKeyMintsAccountScopedKey(t *testing.T) {
	srv, captured, creates, _ := judgeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))
	mock.ExpectExec("INSERT INTO account_llm_judge_keys").
		WithArgs("acct-1", "vk-judge", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	customers := newFakeCustomerStore()
	customers.ids["acct-1"] = "cust-existing"
	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), customers, nil)
	apiKey, baseURL, err := provisioner.EnsureJudgeKey(
		context.Background(), NewJudgeStore(db), testVault(t), "acct-1",
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-bf-judge", apiKey)
	assert.Equal(t, srv.URL, baseURL)
	assert.Equal(t, int32(1), creates.Load())
	assert.Equal(t, "eval-judge/acct-1", captured.Name)
	assert.Equal(t, "cust-existing", captured.CustomerID)
	assert.Empty(t, captured.ExpiresAt)
	require.Len(t, captured.ProviderConfigs, 1)
	assert.Equal(t, []string{"*"}, captured.ProviderConfigs[0].AllowedModels)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureJudgeKeyReusesStoredKey(t *testing.T) {
	srv, _, creates, _ := judgeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC()
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns).AddRow(
			"acct-1", "vk-existing", "sk-bf-existing", nil, nil, now, now, now,
		))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	apiKey, baseURL, err := provisioner.EnsureJudgeKey(
		context.Background(), NewJudgeStore(db), testVault(t), "acct-1",
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-bf-existing", apiKey)
	assert.Equal(t, srv.URL, baseURL)
	assert.Zero(t, creates.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureJudgeKeyRecoversFromConcurrentGenerateConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/governance/virtual-keys" {
			http.Error(w, `{"error":"virtual key name already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC()
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns).AddRow(
			"acct-1", "vk-winner", "sk-bf-winner", nil, nil, now, now, now,
		))

	customers := newFakeCustomerStore()
	customers.ids["acct-1"] = "cust-existing"
	apiKey, baseURL, err := NewProvisioner(NewClient(srv.URL, "", ""), customers, nil).EnsureJudgeKey(
		context.Background(), NewJudgeStore(db), testVault(t), "acct-1",
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-bf-winner", apiKey)
	assert.Equal(t, srv.URL, baseURL)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureJudgeKeyReturnsOrphanedConflictWithoutDeleting(t *testing.T) {
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/virtual-keys":
			http.Error(w, `{"error":"virtual key name already exists"}`, http.StatusConflict)
		case r.Method == http.MethodDelete:
			deletes.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))

	customers := newFakeCustomerStore()
	customers.ids["acct-1"] = "cust-existing"
	_, _, err = NewProvisioner(NewClient(srv.URL, "", ""), customers, nil).EnsureJudgeKey(
		context.Background(), NewJudgeStore(db), testVault(t), "acct-1",
	)
	require.ErrorIs(t, err, ErrJudgeKeyOrphaned)
	assert.ErrorContains(t, err, "account_id=acct-1")
	assert.ErrorContains(t, err, "customer_id=cust-existing")
	assert.ErrorContains(t, err, "key_name=eval-judge/acct-1")
	assert.Zero(t, deletes.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureJudgeKeyValidatesInputs(t *testing.T) {
	provisioner := NewProvisioner(NewClient("http://unused", "", ""), newFakeCustomerStore(), nil)
	_, _, err := provisioner.EnsureJudgeKey(context.Background(), nil, testVault(t), "")
	require.ErrorContains(t, err, "accountID")
}

func TestEnsureJudgeKeyRevokesMintWhenEncryptionFails(t *testing.T) {
	srv, _, _, deletes := judgeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))
	customers := newFakeCustomerStore()
	customers.ids["acct-1"] = "cust-existing"
	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), customers, nil)

	_, _, err = provisioner.EnsureJudgeKey(
		context.Background(), NewJudgeStore(db), envelope.NewVault(failingJudgeKMS{}, "kms-key"), "acct-1",
	)
	require.ErrorContains(t, err, "encrypt judge key")
	assert.Equal(t, int32(1), deletes.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureJudgeKeyRevokesMintWhenSaveFails(t *testing.T) {
	srv, _, _, deletes := judgeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))
	mock.ExpectExec("INSERT INTO account_llm_judge_keys").
		WillReturnError(errors.New("database unavailable"))
	customers := newFakeCustomerStore()
	customers.ids["acct-1"] = "cust-existing"
	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), customers, nil)

	_, _, err = provisioner.EnsureJudgeKey(
		context.Background(), NewJudgeStore(db), testVault(t), "acct-1",
	)
	require.ErrorContains(t, err, "save judge key")
	assert.Equal(t, int32(1), deletes.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevokeAccountJudgeKeysSweepsUpstreamThenDeletesRow(t *testing.T) {
	srv, _, _, deletes := judgeGateway(t)
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-judge"))
	mock.ExpectExec("DELETE FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	require.NoError(t, provisioner.RevokeAccountJudgeKeys(context.Background(), NewJudgeStore(db), "acct-1"))
	assert.Equal(t, int32(1), deletes.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevokeAccountJudgeKeysPreservesRowAfterUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer srv.Close()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-judge"))
	provisioner := NewProvisioner(NewClient(srv.URL, "", ""), newFakeCustomerStore(), nil)
	err = provisioner.RevokeAccountJudgeKeys(context.Background(), NewJudgeStore(db), "acct-1")
	require.ErrorContains(t, err, "delete judge key vk-judge")
	require.NotContains(t, err.Error(), "delete judge key row")
	require.NoError(t, mock.ExpectationsWereMet())
}
