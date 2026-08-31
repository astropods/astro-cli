package evaldatasetstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM eval_datasets").
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "deployment_id", "account_id", "langfuse_dataset_name",
			"created_at", "updated_at",
		}).AddRow("dataset-1", "dep-1", "acct-1", "eval-dep-1", now, now))

	got, err := NewStore(db).GetByID(context.Background(), "dataset-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ID != "dataset-1" || got.DeploymentID != "dep-1" || got.AccountID != "acct-1" {
		t.Fatalf("GetByID = %+v", got)
	}
}

func TestGetByIDMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("FROM eval_datasets").
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "deployment_id", "account_id", "langfuse_dataset_name",
			"created_at", "updated_at",
		}))

	got, err := NewStore(db).GetByID(context.Background(), "dataset-1")
	if err != nil || got != nil {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}
}

func TestGetByIDError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("FROM eval_datasets").
		WillReturnError(errors.New("query failed"))

	_, err = NewStore(db).GetByID(context.Background(), "dataset-1")
	if err == nil || !strings.Contains(err.Error(), "query failed") {
		t.Fatalf("GetByID error = %v", err)
	}
}
