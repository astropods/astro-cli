// Package judgmentstoretest provides sqlmock-based helpers for tests that
// exercise judgmentstore.Store from other packages. It keeps the judgment and
// reason query expectations in one place so they stay in sync as the
// eval_dataset_judgments / eval_dataset_judgment_reasons schema evolves.
package judgmentstoretest

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

// ReasonColumns is the column list returned when clearing reasons.
var ReasonColumns = []string{"dimension_key", "dimension_value"}

func ExpectSetVerdict(mock sqlmock.Sqlmock, datasetID, traceID string, next, prev judgmentstore.Verdict, cleared, inserted []judgmentstore.Reason) {
	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs(datasetID, traceID, string(next)).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow(string(prev)))
	if next != prev {
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
	mock.ExpectCommit()
}

func ExpectSetVerdictMissing(mock sqlmock.Sqlmock, datasetID, traceID string, next judgmentstore.Verdict) {
	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs(datasetID, traceID, string(next)).
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}))
	mock.ExpectRollback()
}
