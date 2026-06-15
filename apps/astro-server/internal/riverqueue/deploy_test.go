package riverqueue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/datasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// datasetColumns is the column list expected by datasetstore.Store.Get.
var datasetColumns = []string{
	"deployment_id", "account_id", "langfuse_dataset_name", "item_count",
	"created_at", "updated_at",
}

func langfuseOKServer(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	called := new(bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			*called = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, called
}

func newDatasetMock(t *testing.T) (*sqlmock.Sqlmock, *datasetstore.Store) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &mock, datasetstore.NewStore(db)
}

func newLangfuseMock(t *testing.T) (*sqlmock.Sqlmock, *langfuse.Store) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &mock, langfuse.NewStore(db)
}

func expectLangfuseCreds(mock *sqlmock.Sqlmock, accountID string) {
	(*mock).ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key",
			"encrypted_data_key", "nonce", "created_at",
		}).AddRow(accountID, "proj-1", "pk", "sk", []byte(nil), []byte(nil), time.Now()))
}

func expectNoCreds(mock *sqlmock.Sqlmock, accountID string) {
	(*mock).ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key",
			"encrypted_data_key", "nonce", "created_at",
		}))
}

func expectDatasetMissing(mock *sqlmock.Sqlmock, depID string) {
	(*mock).ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(depID).
		WillReturnRows(sqlmock.NewRows(datasetColumns))
}

func expectDatasetExists(mock *sqlmock.Sqlmock, depID string) {
	(*mock).ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(depID).
		WillReturnRows(sqlmock.NewRows(datasetColumns).AddRow(
			depID, "acct-1", "eval-"+depID, 0, time.Now(), time.Now(),
		))
}

func expectDatasetCreate(mock *sqlmock.Sqlmock, depID, accountID string) {
	(*mock).ExpectExec("INSERT INTO eval_datasets").
		WithArgs(depID, accountID, "eval-"+depID).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectDepDatasetExists(mock *sqlmock.Sqlmock, depID string) {
	(*mock).ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(depID).
		WillReturnRows(sqlmock.NewRows(datasetColumns).AddRow(
			depID, "acct-1", "dep-"+depID, 0, time.Now(), time.Now(),
		))
}

func expectDatasetRepoint(mock *sqlmock.Sqlmock, depID, newName string) {
	(*mock).ExpectExec("UPDATE eval_datasets").
		WithArgs(newName, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func testDep(depID, accountID string) *deploymentstore.Deployment {
	return &deploymentstore.Deployment{
		ID:        depID,
		AccountID: accountID,
		AgentName: "test-agent",
	}
}

// ---------------------------------------------------------------------------
// ensureDataset
// ---------------------------------------------------------------------------

func TestEnsureDataset_CreatesWhenNotExists(t *testing.T) {
	srv, called := langfuseOKServer(t)
	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectDatasetMissing(dsMock, dep.ID)
	expectDatasetCreate(dsMock, dep.ID, dep.AccountID)
	expectDatasetExists(dsMock, dep.ID) // re-read after create

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := ensureDataset(context.Background(), dep, dsStore, client)
	if err != nil {
		t.Fatalf("ensureDataset: %v", err)
	}
	if record == nil {
		t.Fatal("expected non-nil record")
	}
	if record.LangfuseDatasetName != "eval-dep-1" {
		t.Errorf("LangfuseDatasetName = %q, want eval-dep-1", record.LangfuseDatasetName)
	}
	if !*called {
		t.Error("expected Langfuse CreateDataset API to be called")
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureDataset_SkipsWhenAlreadyExists(t *testing.T) {
	srv, called := langfuseOKServer(t)
	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectDatasetExists(dsMock, dep.ID)

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := ensureDataset(context.Background(), dep, dsStore, client)
	if err != nil {
		t.Fatalf("ensureDataset: %v", err)
	}
	if record == nil {
		t.Fatal("expected non-nil record")
	}
	if *called {
		t.Error("Langfuse API should not be called when dataset already exists")
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureDataset_HealsDepRowOnRedeploy(t *testing.T) {
	srv, called := langfuseOKServer(t)
	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectDepDatasetExists(dsMock, dep.ID)
	expectDatasetRepoint(dsMock, dep.ID, "eval-dep-1")

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := ensureDataset(context.Background(), dep, dsStore, client)
	if err != nil {
		t.Fatalf("ensureDataset: %v", err)
	}
	if record.LangfuseDatasetName != "eval-dep-1" {
		t.Errorf("LangfuseDatasetName = %q, want eval-dep-1", record.LangfuseDatasetName)
	}
	if !*called {
		t.Error("expected Langfuse CreateDataset to be called for heal")
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureDataset_HealLangfuseFailureLeavesDepRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectDepDatasetExists(dsMock, dep.ID)

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	record, err := ensureDataset(context.Background(), dep, dsStore, client)
	if err != nil {
		t.Fatalf("ensureDataset returned error: %v", err)
	}
	if record.LangfuseDatasetName != "dep-dep-1" {
		t.Errorf("LangfuseDatasetName = %q, want dep-dep-1 (heal should not have flipped row)", record.LangfuseDatasetName)
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureDataset_LangfuseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectDatasetMissing(dsMock, dep.ID)

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	_, err := ensureDataset(context.Background(), dep, dsStore, client)
	if err == nil {
		t.Fatal("expected error from Langfuse, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeployWorker.provisionDataset
// ---------------------------------------------------------------------------

func TestProvisionDataset_HappyPath(t *testing.T) {
	srv, called := langfuseOKServer(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectLangfuseCreds(lfMock, dep.AccountID)
	expectDatasetMissing(dsMock, dep.ID)
	expectDatasetCreate(dsMock, dep.ID, dep.AccountID)
	expectDatasetExists(dsMock, dep.ID) // re-read after create

	w := &DeployWorker{
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		datasetStore:    dsStore,
		log:             logger.New("error", "json"),
	}
	w.provisionDataset(dep)

	if !*called {
		t.Error("expected Langfuse CreateDataset API to be called")
	}
	if err := (*lfMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet langfuse sql expectations: %v", err)
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}

func TestProvisionDataset_SkipsWhenNoCreds(t *testing.T) {
	_, called := langfuseOKServer(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectNoCreds(lfMock, dep.AccountID)

	w := &DeployWorker{
		langfuseStore: lfStore,
		datasetStore:  dsStore,
		log:           logger.New("error", "json"),
	}
	w.provisionDataset(dep)

	if *called {
		t.Error("Langfuse API should not be called when no creds")
	}
	if err := (*lfMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet langfuse sql expectations: %v", err)
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}

func TestProvisionDataset_SkipsWhenAlreadyExists(t *testing.T) {
	srv, called := langfuseOKServer(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	dep := testDep("dep-1", "acct-1")

	expectLangfuseCreds(lfMock, dep.AccountID)
	expectDatasetExists(dsMock, dep.ID)

	w := &DeployWorker{
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		datasetStore:    dsStore,
		log:             logger.New("error", "json"),
	}
	w.provisionDataset(dep)

	if *called {
		t.Error("Langfuse API should not be called when dataset already exists")
	}
	if err := (*lfMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet langfuse sql expectations: %v", err)
	}
	if err := (*dsMock).ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}
