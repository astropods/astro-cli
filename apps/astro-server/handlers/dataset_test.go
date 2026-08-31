package handlers

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/evaldismissalstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

// ---------------------------------------------------------------------------
// Dataset handler fixture
// ---------------------------------------------------------------------------

type datasetFixture struct {
	*traceDetailFixture
	datasetMock   sqlmock.Sqlmock
	judgmentMock  sqlmock.Sqlmock
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

	judgmentDB, judgmentMock, _ := sqlmock.New()
	t.Cleanup(func() { judgmentDB.Close() })
	judgmentStore := judgmentstore.NewStore(judgmentDB)

	itemDB, itemMock, _ := sqlmock.New()
	t.Cleanup(func() { itemDB.Close() })
	itemStore := evalitemstore.NewStore(itemDB)

	runDB, runMock, _ := sqlmock.New()
	t.Cleanup(func() { runDB.Close() })
	runStore := evalrunstore.NewStore(runDB)

	dismissalDB, dismissalMock, _ := sqlmock.New()
	t.Cleanup(func() { dismissalDB.Close() })
	dismissalStore := evaldismissalstore.NewStore(dismissalDB)

	f.router.GET("/api/v1/deployments/:id/dataset",
		GetEvalDataset(log, accountStore, deployStore, dsStore, itemStore))
	f.router.GET("/api/v1/deployments/:id/dataset/items",
		GetEvalDatasetItems(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore))
	f.router.POST("/api/v1/deployments/:id/dataset/items",
		PostDatasetItem(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore, runStore))
	f.router.PUT("/api/v1/deployments/:id/dataset/items/:trace_id/evaluator-outputs",
		PutDatasetItemEvaluatorOutputs(log, accountStore, deployStore, dsStore, itemStore))
	f.router.DELETE("/api/v1/deployments/:id/dataset/items/:trace_id",
		DeleteDatasetItem(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore))
	f.router.GET("/api/v1/deployments/:id/dataset/download",
		DownloadEvalDataset(log, cfg, accountStore, deployStore, dsStore, langfuseStore))
	f.router.GET("/api/v1/deployments/:id/dataset/review-queue",
		GetDatasetReviewQueue(log, cfg, accountStore, deployStore, dsStore, langfuseStore, itemStore, runStore, dismissalStore))
	f.router.GET("/api/v1/deployments/:id/dataset/review-queue/:trace_id/evaluation",
		GetDatasetTraceEvaluation(log, cfg, accountStore, deployStore, dsStore, langfuseStore, runStore, nil))
	f.router.POST("/api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss",
		PostReviewQueueDismissal(log, accountStore, deployStore, dsStore, dismissalStore))
	f.router.DELETE("/api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss",
		DeleteReviewQueueDismissal(log, accountStore, deployStore, dsStore, dismissalStore))
	f.router.GET("/api/v1/deployments/:id/dataset/evaluations/status",
		GetDatasetEvaluationStatus(log, accountStore, deployStore, dsStore, runStore))
	f.router.POST("/api/v1/deployments/:id/dataset/judgments",
		PostDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.PATCH("/api/v1/deployments/:id/dataset/judgments/:trace_id",
		PatchDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.PUT("/api/v1/deployments/:id/dataset/judgments/:trace_id/criteria",
		PutDatasetJudgmentCriteria(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.DELETE("/api/v1/deployments/:id/dataset/judgments/:trace_id",
		DeleteDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))

	return &datasetFixture{
		traceDetailFixture: f,
		datasetMock:        datasetMock,
		judgmentMock:       judgmentMock,
		itemMock:           itemMock,
		runMock:            runMock,
		dismissalMock:      dismissalMock,
	}
}

func expectDatasetRow(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount int) {
	expectDatasetRowCounts(mock, deploymentID, datasetName, itemCount, itemCount, 0)
}

func expectDatasetRowCounts(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount, goodCount, badCount int) {
	datasetstoretest.ExpectRow(mock, deploymentID, datasetName, goodCount, badCount)
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
