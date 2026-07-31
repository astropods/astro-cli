package judgmentstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestGetPredictionStatusCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("WITH prediction_states AS").
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"queued", "in_progress", "completed", "failed",
		}).AddRow(2, 3, 4, 5))

	counts, err := NewStore(db).GetPredictionStatusCounts(
		context.Background(),
		"dataset-1",
	)
	if err != nil {
		t.Fatalf("GetPredictionStatusCounts: %v", err)
	}
	if counts != (PredictionStatusCounts{
		Queued: 2, InProgress: 3, Completed: 4, Failed: 5,
	}) {
		t.Fatalf("GetPredictionStatusCounts = %+v", counts)
	}
}

func TestGetPredictionStatusCountsReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("WITH prediction_states AS").
		WithArgs("dataset-1").
		WillReturnError(errors.New("read failed"))

	_, err = NewStore(db).GetPredictionStatusCounts(
		context.Background(),
		"dataset-1",
	)
	if err == nil {
		t.Fatal("GetPredictionStatusCounts error = nil")
	}
}

func TestGetPredictionRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, time.July, 27, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	mock.ExpectQuery("FROM eval_dataset_prediction_requests").
		WithArgs("dataset-1", pq.Array([]string{"trace-1", "trace-2"})).
		WillReturnRows(sqlmock.NewRows([]string{
			"trace_id", "status", "error_message", "created_at", "updated_at",
		}).
			AddRow("trace-1", "in_progress", nil, createdAt, updatedAt).
			AddRow("trace-2", "failed", "Prediction quota exceeded.", createdAt, updatedAt))

	got, err := NewStore(db).GetPredictionRequests(
		context.Background(),
		"dataset-1",
		[]string{"trace-1", "trace-2"},
	)
	if err != nil {
		t.Fatalf("GetPredictionRequests: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetPredictionRequests len = %d, want 2", len(got))
	}
	if got["trace-1"].Status != PredictionRequestInProgress || got["trace-1"].ErrorMessage != nil {
		t.Fatalf("trace-1 request = %+v", got["trace-1"])
	}
	failed := got["trace-2"]
	if failed.Status != PredictionRequestFailed || failed.ErrorMessage == nil || *failed.ErrorMessage != "Prediction quota exceeded." {
		t.Fatalf("trace-2 request = %+v", failed)
	}
	if !failed.CreatedAt.Equal(createdAt) || !failed.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("trace-2 timestamps = %v/%v", failed.CreatedAt, failed.UpdatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetPredictionRequestsEmptyTraceIDsDoesNotQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	got, err := NewStore(db).GetPredictionRequests(context.Background(), "dataset-1", nil)
	if err != nil {
		t.Fatalf("GetPredictionRequests: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetPredictionRequests = %+v, want empty map", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetPredictionRequestsReturnsSQLErrors(t *testing.T) {
	t.Run("query", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectQuery("FROM eval_dataset_prediction_requests").
			WillReturnError(errors.New("query failed"))

		_, err = NewStore(db).GetPredictionRequests(context.Background(), "dataset-1", []string{"trace-1"})
		if err == nil || !strings.Contains(err.Error(), "query failed") {
			t.Fatalf("GetPredictionRequests error = %v", err)
		}
	})

	t.Run("scan", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectQuery("FROM eval_dataset_prediction_requests").
			WillReturnRows(sqlmock.NewRows([]string{
				"trace_id", "status", "error_message", "created_at",
			}).AddRow("trace-1", "queued", nil, time.Now()))

		_, err = NewStore(db).GetPredictionRequests(context.Background(), "dataset-1", []string{"trace-1"})
		if err == nil || !strings.Contains(err.Error(), "scan") {
			t.Fatalf("GetPredictionRequests error = %v", err)
		}
	})

	t.Run("iteration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		now := time.Now()
		mock.ExpectQuery("FROM eval_dataset_prediction_requests").
			WillReturnRows(sqlmock.NewRows([]string{
				"trace_id", "status", "error_message", "created_at", "updated_at",
			}).AddRow("trace-1", "queued", nil, now, now).
				RowError(0, errors.New("iteration failed")))

		_, err = NewStore(db).GetPredictionRequests(context.Background(), "dataset-1", []string{"trace-1"})
		if err == nil || !strings.Contains(err.Error(), "iteration failed") {
			t.Fatalf("GetPredictionRequests error = %v", err)
		}
	})
}

func TestQueuePredictionRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	traceIDs := []string{"trace-1", "trace-2", "trace-3"}
	mock.ExpectQuery(`(?s)INSERT INTO eval_dataset_prediction_requests.*unnest.*ON CONFLICT.*status = 'queued'.*RETURNING trace_id`).
		WithArgs("dataset-1", pq.Array(traceIDs)).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id"}).
			AddRow("trace-1").
			AddRow("trace-3"))

	queued, err := NewStore(db).QueuePredictionRequests(context.Background(), "dataset-1", traceIDs)
	if err != nil {
		t.Fatalf("QueuePredictionRequests: %v", err)
	}
	if fmt.Sprint(queued) != "[trace-1 trace-3]" {
		t.Fatalf("queued = %v", queued)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestQueuePredictionRequestsEmptyDoesNotQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	queued, err := NewStore(db).QueuePredictionRequests(context.Background(), "dataset-1", nil)
	if err != nil {
		t.Fatalf("QueuePredictionRequests: %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued = %v", queued)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected query: %v", err)
	}
}

func TestUpdatePredictionRequest(t *testing.T) {
	tests := []struct {
		name         string
		status       PredictionRequestStatus
		errorMessage *string
	}{
		{name: "in progress", status: PredictionRequestInProgress},
		{name: "completed", status: PredictionRequestCompleted},
		{name: "failed without message", status: PredictionRequestFailed},
		{name: "failed with message", status: PredictionRequestFailed, errorMessage: stringPointer("Prediction failed.")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			var messageArg any
			if tt.errorMessage != nil {
				messageArg = *tt.errorMessage
			}
			mock.ExpectExec("UPDATE eval_dataset_prediction_requests").
				WithArgs("dataset-1", "trace-1", string(tt.status), messageArg).
				WillReturnResult(sqlmock.NewResult(0, 1))

			err = NewStore(db).UpdatePredictionRequest(
				context.Background(),
				"dataset-1",
				"trace-1",
				tt.status,
				tt.errorMessage,
			)
			if err != nil {
				t.Fatalf("UpdatePredictionRequest: %v", err)
			}
		})
	}
}

func TestUpdatePredictionRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	message := "Failed to enqueue prediction. Try again."
	traceIDs := []string{"trace-1", "trace-2"}
	mock.ExpectExec("UPDATE eval_dataset_prediction_requests").
		WithArgs("dataset-1", pq.Array(traceIDs), string(PredictionRequestFailed), message).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err = NewStore(db).UpdatePredictionRequests(
		context.Background(),
		"dataset-1",
		traceIDs,
		PredictionRequestFailed,
		&message,
	)
	if err != nil {
		t.Fatalf("UpdatePredictionRequests: %v", err)
	}
}

func TestUpdatePredictionRequestErrors(t *testing.T) {
	t.Run("invalid status", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		err = NewStore(db).UpdatePredictionRequest(
			context.Background(),
			"dataset-1",
			"trace-1",
			PredictionRequestStatus("unknown"),
			nil,
		)
		if err == nil || !strings.Contains(err.Error(), "invalid status") {
			t.Fatalf("UpdatePredictionRequest error = %v", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectExec("UPDATE eval_dataset_prediction_requests").
			WillReturnError(errors.New("update failed"))

		err = NewStore(db).UpdatePredictionRequest(
			context.Background(),
			"dataset-1",
			"trace-1",
			PredictionRequestFailed,
			nil,
		)
		if err == nil || !strings.Contains(err.Error(), "update failed") {
			t.Fatalf("UpdatePredictionRequest error = %v", err)
		}
	})

	t.Run("rows affected", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectExec("UPDATE eval_dataset_prediction_requests").
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows failed")))

		err = NewStore(db).UpdatePredictionRequest(
			context.Background(),
			"dataset-1",
			"trace-1",
			PredictionRequestFailed,
			nil,
		)
		if err == nil || !strings.Contains(err.Error(), "rows failed") {
			t.Fatalf("UpdatePredictionRequest error = %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectExec("UPDATE eval_dataset_prediction_requests").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = NewStore(db).UpdatePredictionRequest(
			context.Background(),
			"dataset-1",
			"trace-1",
			PredictionRequestFailed,
			nil,
		)
		if err == nil || !strings.Contains(err.Error(), "request not found") {
			t.Fatalf("UpdatePredictionRequest error = %v", err)
		}
	})
}

func TestDeletePredictionRequestIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec("DELETE FROM eval_dataset_prediction_requests").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := NewStore(db).DeletePredictionRequest(context.Background(), "dataset-1", "trace-1"); err != nil {
		t.Fatalf("DeletePredictionRequest: %v", err)
	}
}

func TestDeletePredictionRequestReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec("DELETE FROM eval_dataset_prediction_requests").
		WillReturnError(errors.New("delete failed"))

	err = NewStore(db).DeletePredictionRequest(context.Background(), "dataset-1", "trace-1")
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("DeletePredictionRequest error = %v", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
