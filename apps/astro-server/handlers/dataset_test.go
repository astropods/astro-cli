package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/evaldismissalstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
)

const fakeStaleEvaluationRef = "agent/stale-test-ref"

type fakeEvalSetResolver struct{}

func (fakeEvalSetResolver) ActiveRef(context.Context, string, string) (string, error) {
	return evalpreset.RefDefaultSet, nil
}

func (fakeEvalSetResolver) Set(_ context.Context, ref string) ([]evaluator.Evaluator, error) {
	if ref == fakeStaleEvaluationRef {
		return evalpreset.ResolveSet(evalpreset.RefDefaultSet)
	}
	return evalpreset.ResolveSet(ref)
}

// ---------------------------------------------------------------------------
// Dataset handler fixture
// ---------------------------------------------------------------------------

type datasetFixture struct {
	*traceDetailFixture
	datasetMock   sqlmock.Sqlmock
	itemMock      sqlmock.Sqlmock
	runMock       sqlmock.Sqlmock
	dismissalMock sqlmock.Sqlmock
}

func setupDatasetRouter(t *testing.T, withUser bool, upstreamHandler http.HandlerFunc) *datasetFixture {
	t.Helper()
	f, log, cfg, accountStore, deployStore, langfuseStore := newLangfuseFixture(t, withUser, upstreamHandler)

	datasetDB, datasetMock, _ := sqlmock.New()
	t.Cleanup(func() { datasetDB.Close() })
	dsStore := evaldatasetstore.NewStore(datasetDB)

	itemDB, itemMock, _ := sqlmock.New()
	t.Cleanup(func() { itemDB.Close() })
	itemStore := evalitemstore.NewStore(itemDB)

	runDB, runMock, _ := sqlmock.New()
	t.Cleanup(func() { runDB.Close() })
	runStore := evalrunstore.NewStore(runDB)

	dismissalDB, dismissalMock, _ := sqlmock.New()
	t.Cleanup(func() { dismissalDB.Close() })
	dismissalStore := evaldismissalstore.NewStore(dismissalDB)

	resolver := fakeEvalSetResolver{}
	f.router.GET("/api/v1/deployments/:id/dataset",
		GetEvalDataset(log, accountStore, deployStore, dsStore, itemStore, resolver))
	f.router.GET("/api/v1/deployments/:id/dataset/items",
		GetEvalDatasetItems(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore, resolver))
	f.router.POST("/api/v1/deployments/:id/dataset/items",
		PostDatasetItem(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore, runStore, resolver))
	f.router.PUT("/api/v1/deployments/:id/dataset/items/:trace_id/evaluator-outputs",
		PutDatasetItemEvaluatorOutputs(log, accountStore, deployStore, dsStore, itemStore, resolver))
	f.router.DELETE("/api/v1/deployments/:id/dataset/items/:trace_id",
		DeleteDatasetItem(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore))
	f.router.GET("/api/v1/deployments/:id/dataset/download",
		DownloadEvalDataset(log, cfg, accountStore, deployStore, dsStore, langfuseStore))
	f.router.GET("/api/v1/deployments/:id/dataset/review-queue",
		GetDatasetReviewQueue(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore, runStore, dismissalStore))
	f.router.GET("/api/v1/deployments/:id/dataset/review-queue/:trace_id/evaluation",
		GetDatasetTraceEvaluation(log, cfg, accountStore, deployStore, dsStore, langfuseStore, runStore, nil, resolver))
	f.router.POST("/api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss",
		PostReviewQueueDismissal(log, accountStore, deployStore, dsStore, dismissalStore))
	f.router.DELETE("/api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss",
		DeleteReviewQueueDismissal(log, accountStore, deployStore, dsStore, dismissalStore))
	f.router.GET("/api/v1/deployments/:id/dataset/evaluations/status",
		GetDatasetEvaluationStatus(log, accountStore, deployStore, dsStore, runStore))

	return &datasetFixture{
		traceDetailFixture: f,
		datasetMock:        datasetMock,
		itemMock:           itemMock,
		runMock:            runMock,
		dismissalMock:      dismissalMock,
	}
}

func expectDatasetRow(mock sqlmock.Sqlmock, deploymentID, datasetName string) {
	datasetstoretest.ExpectRow(mock, deploymentID, datasetName)
}

func expectDatasetNotFound(mock sqlmock.Sqlmock, deploymentID string) {
	datasetstoretest.ExpectMissing(mock, deploymentID)
}

func expectDatasetAuthorization(f *datasetFixture, member bool) {
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "sasbot", "build-1", "ns-1")
	count := 0
	if member {
		count = 1
	}
	f.accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}
