package deploymentstore

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestGetMessagingURLsContextHonorsCancellation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	mock.ExpectQuery(`(?s)SELECT sc.deployment_id, di.hostname, di.tls_enabled.*FROM deployment_sidecars`).
		WithArgs(pq.Array([]string{"deployment-1"})).
		WillDelayFor(time.Second).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "hostname", "tls_enabled"}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	_, err = store.GetMessagingURLsContext(ctx, []string{"deployment-1"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("query ignored cancellation and took %s", elapsed)
	}
}
