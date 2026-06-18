package riverqueue

import (
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

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

func testDep(depID, accountID string) *deploymentstore.Deployment {
	return &deploymentstore.Deployment{
		ID:        depID,
		AccountID: accountID,
		AgentName: "test-agent",
	}
}

// ---------------------------------------------------------------------------
// DeployWorker.provisionDataset
// ---------------------------------------------------------------------------

func TestProvisionDataset_HappyPath(t *testing.T) {
	srv, called := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := datasetstoretest.NewMock(t)
	dep := testDep("dep-1", "acct-1")

	expectLangfuseCreds(lfMock, dep.AccountID)
	datasetstoretest.ExpectMissing(dsMock, dep.ID)
	datasetstoretest.ExpectCreate(dsMock, dep.ID, dep.AccountID)
	datasetstoretest.ExpectExists(dsMock, dep.ID) // re-read after create

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
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}

func TestProvisionDataset_SkipsWhenNoCreds(t *testing.T) {
	_, called := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := datasetstoretest.NewMock(t)
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
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}

func TestProvisionDataset_SkipsWhenAlreadyExists(t *testing.T) {
	srv, called := datasetstoretest.LangfuseStatusServer(t, http.StatusOK)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := datasetstoretest.NewMock(t)
	dep := testDep("dep-1", "acct-1")

	expectLangfuseCreds(lfMock, dep.AccountID)
	datasetstoretest.ExpectExists(dsMock, dep.ID)

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
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}

func TestProvisionDataset_IgnoresEnsureError(t *testing.T) {
	srv, called := datasetstoretest.LangfuseStatusServer(t, http.StatusInternalServerError)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := datasetstoretest.NewMock(t)
	dep := testDep("dep-1", "acct-1")

	expectLangfuseCreds(lfMock, dep.AccountID)
	datasetstoretest.ExpectLegacyExists(dsMock, dep.ID)

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
	if err := dsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset sql expectations: %v", err)
	}
}
