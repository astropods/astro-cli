// Package judgmentstoretest provides test doubles and sqlmock helpers for tests
// that exercise judgment storage from other packages. It keeps shared behavior
// and query expectations in one place as the judgment schema evolves.
package judgmentstoretest

import (
	"context"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

// ReasonColumns is the column list returned when clearing reasons.
var ReasonColumns = []string{"dimension_key", "dimension_value"}

// expectReasonReplacement queues the shared reason-replacement body: the
// DELETE ... RETURNING (returning cleared) and, when inserted is non-empty, the
// INSERT ... unnest exec. Used by both ExpectSetVerdict and ExpectReplaceReasons.
func expectReasonReplacement(mock sqlmock.Sqlmock, datasetID, traceID string, cleared, inserted []judgmentstore.Reason) {
	rows := sqlmock.NewRows(ReasonColumns)
	for _, r := range cleared {
		rows.AddRow(string(r.Dimension), r.Value)
	}
	mock.ExpectQuery("DELETE FROM eval_dataset_judgment_reasons").
		WithArgs(datasetID, traceID).
		WillReturnRows(rows)
	if len(inserted) > 0 {
		keys := make([]string, len(inserted))
		vals := make([]float64, len(inserted))
		for i, r := range inserted {
			keys[i] = string(r.Dimension)
			vals[i] = r.Value
		}
		mock.ExpectExec("INSERT INTO eval_dataset_judgment_reasons").
			WithArgs(datasetID, traceID, pq.Array(keys), pq.Array(vals)).
			WillReturnResult(sqlmock.NewResult(0, int64(len(inserted))))
	}
}

func ExpectSetVerdict(mock sqlmock.Sqlmock, datasetID, traceID string, next, prev judgmentstore.Verdict, cleared, inserted []judgmentstore.Reason) {
	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs(datasetID, traceID, string(next)).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow(string(prev)))
	if next != prev {
		expectReasonReplacement(mock, datasetID, traceID, cleared, inserted)
	}
	mock.ExpectCommit()
}

func ExpectSetVerdictMissing(mock sqlmock.Sqlmock, datasetID, traceID string, next judgmentstore.Verdict) {
	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs(datasetID, traceID, string(next)).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}))
	mock.ExpectRollback()
}

func ExpectReplaceReasons(mock sqlmock.Sqlmock, datasetID, traceID string, verdict judgmentstore.Verdict, previous, inserted []judgmentstore.Reason) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs(datasetID, traceID).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow(string(verdict)))
	expectReasonReplacement(mock, datasetID, traceID, previous, inserted)
	mock.ExpectCommit()
}

func ExpectReplaceReasonsUnknown(mock sqlmock.Sqlmock, datasetID, traceID string) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs(datasetID, traceID).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow(string(judgmentstore.VerdictUnknown)))
	mock.ExpectRollback()
}

func ExpectReplaceReasonsMissing(mock sqlmock.Sqlmock, datasetID, traceID string) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs(datasetID, traceID).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}))
	mock.ExpectRollback()
}

func ExpectJudgedTraceIDs(mock sqlmock.Sqlmock, datasetID string, traceIDs ...string) {
	rows := sqlmock.NewRows([]string{"trace_id"})
	for _, traceID := range traceIDs {
		rows.AddRow(traceID)
	}
	mock.ExpectQuery("SELECT trace_id FROM eval_dataset_judgments").
		WithArgs(datasetID, sqlmock.AnyArg()).
		WillReturnRows(rows)
}

// PredictionRequestUpdate records one batch lifecycle update made through a
// FakePredictionStore.
type PredictionRequestUpdate struct {
	TraceIDs     []string
	Status       judgmentstore.PredictionRequestStatus
	ErrorMessage *string
}

// FakePredictionStore is a deterministic prediction-store implementation for
// handler tests. PreservedIDs model queued or in-progress rows that a batch
// queue operation leaves unchanged.
type FakePredictionStore struct {
	Judged         map[string]bool
	Requests       map[string]judgmentstore.PredictionRequest
	Predictions    map[string]judgmentstore.Prediction
	JudgedErr      error
	RequestsErr    error
	PredictionsErr error
	QueueErr       error
	UpdateErr      error
	BatchTraceIDs  []string
	PredictionIDs  []string
	QueuedTraceIDs []string
	PreservedIDs   map[string]bool
	Updates        []PredictionRequestUpdate
}

func (f *FakePredictionStore) JudgedTraceIDs(
	_ context.Context,
	_ string,
	traceIDs []string,
) (map[string]bool, error) {
	f.BatchTraceIDs = append([]string(nil), traceIDs...)
	return f.Judged, f.JudgedErr
}

func (f *FakePredictionStore) GetPredictionRequests(
	_ context.Context,
	_ string,
	_ []string,
) (map[string]judgmentstore.PredictionRequest, error) {
	return f.Requests, f.RequestsErr
}

func (f *FakePredictionStore) GetPredictions(
	_ context.Context,
	_ string,
	traceIDs []string,
) (map[string]judgmentstore.Prediction, error) {
	f.PredictionIDs = append([]string(nil), traceIDs...)
	return f.Predictions, f.PredictionsErr
}
