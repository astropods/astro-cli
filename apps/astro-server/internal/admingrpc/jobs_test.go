package admingrpc

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

var riverJobCols = []string{
	"id", "kind", "queue", "state", "attempt", "max_attempts",
	"created_at", "attempted_at", "finalized_at", "scheduled_at",
	"args", "errors", "priority",
}

func riverJobRowVals(id int64, kind, queue, state string) []driver.Value {
	now := time.Now()
	return []driver.Value{id, kind, queue, state, 1, 3, now, nil, nil, now, []byte(`{}`), nil, 1}
}

type mockJobsQueue struct {
	cancelCalls []int64
	retryCalls  []int64
	pauseCalls  []string
	resumeCalls []string
	cancelErr   error
	retryErr    error
	retryNoops  map[int64]bool
	pauseErr    error
	resumeErr   error
}

func (m *mockJobsQueue) InsertUndeployJob(context.Context, string, string) error { return nil }
func (m *mockJobsQueue) InsertWakeUpJob(context.Context, string, string) error   { return nil }
func (m *mockJobsQueue) InsertDeployJob(context.Context, string, string) error   { return nil }
func (m *mockJobsQueue) InsertMigrateDeploymentClusterJob(context.Context, string, string, string) error {
	return nil
}
func (m *mockJobsQueue) InsertBillingProvision(context.Context, string) error { return nil }
func (m *mockJobsQueue) InsertBillingResume(context.Context, string) error    { return nil }
func (m *mockJobsQueue) TriggerJob(context.Context, string, json.RawMessage) (int64, error) {
	return 0, nil
}
func (m *mockJobsQueue) CancelJob(_ context.Context, id int64) error {
	m.cancelCalls = append(m.cancelCalls, id)
	return m.cancelErr
}
func (m *mockJobsQueue) RetryJob(_ context.Context, id int64) (bool, error) {
	m.retryCalls = append(m.retryCalls, id)
	if m.retryErr != nil {
		return false, m.retryErr
	}
	return !m.retryNoops[id], nil
}
func (m *mockJobsQueue) PauseQueue(_ context.Context, name string) error {
	m.pauseCalls = append(m.pauseCalls, name)
	return m.pauseErr
}
func (m *mockJobsQueue) ResumeQueue(_ context.Context, name string) error {
	m.resumeCalls = append(m.resumeCalls, name)
	return m.resumeErr
}

func TestGetJobStates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT state, COUNT").WillReturnRows(
		sqlmock.NewRows([]string{"state", "count"}).
			AddRow("available", int64(5)).
			AddRow("running", int64(2)).
			AddRow("completed", int64(100)).
			AddRow("discarded", int64(3)),
	)

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.GetJobStates(context.Background(), &adminv1.GetJobStatesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Available != 5 {
		t.Errorf("Available = %d, want 5", resp.Available)
	}
	if resp.Running != 2 {
		t.Errorf("Running = %d, want 2", resp.Running)
	}
	if resp.Completed != 100 {
		t.Errorf("Completed = %d, want 100", resp.Completed)
	}
	if resp.Discarded != 3 {
		t.Errorf("Discarded = %d, want 3", resp.Discarded)
	}
}

func TestGetJobStates_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT state, COUNT").WillReturnError(errors.New("connection refused"))

	s := &Server{db: db, log: logger.New("error", "json")}
	_, err = s.GetJobStates(context.Background(), &adminv1.GetJobStatesRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("status.Code(err) = %v, want %v (err=%v)", status.Code(err), codes.Internal, err)
	}
}

func TestListAdminQueues(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	pausedAt := sql.NullTime{Time: now, Valid: true}

	mock.ExpectQuery(`(?s)SELECT q.name.*WHERE state IN \('available', 'running'\).*GROUP BY queue`).WillReturnRows(
		sqlmock.NewRows([]string{"name", "paused_at", "updated_at", "available", "running"}).
			AddRow("default", sql.NullTime{}, now, int64(3), int64(1)).
			AddRow("deploy", pausedAt, now, int64(0), int64(0)),
	)

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.ListAdminQueues(context.Background(), &adminv1.ListAdminQueuesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Queues) != 2 {
		t.Fatalf("queues = %d, want 2", len(resp.Queues))
	}
	if resp.Queues[0].Name != "default" {
		t.Errorf("queue[0].Name = %q, want %q", resp.Queues[0].Name, "default")
	}
	if resp.Queues[0].CountAvailable != 3 {
		t.Errorf("queue[0].CountAvailable = %d, want 3", resp.Queues[0].CountAvailable)
	}
	if resp.Queues[0].PausedAt != "" {
		t.Errorf("queue[0].PausedAt should be empty for active queue, got %q", resp.Queues[0].PausedAt)
	}
	if resp.Queues[1].PausedAt == "" {
		t.Errorf("queue[1].PausedAt should be set for paused queue")
	}
}

func TestListAdminQueues_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT q.name").WillReturnError(errors.New("connection refused"))

	s := &Server{db: db, log: logger.New("error", "json")}
	_, err = s.ListAdminQueues(context.Background(), &adminv1.ListAdminQueuesRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("status.Code(err) = %v, want %v (err=%v)", status.Code(err), codes.Internal, err)
	}
}

func TestListJobs_NoFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT id, kind").WillReturnRows(
		sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(1, "deploy", "default", "completed")...).
			AddRow(riverJobRowVals(2, "reconcile", "default", "available")...),
	)

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.ListJobs(context.Background(), &adminv1.ListJobsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(resp.Jobs))
	}
	if resp.Jobs[0].ID != 1 || resp.Jobs[0].Kind != "deploy" {
		t.Errorf("job[0] = %+v, want id=1 kind=deploy", resp.Jobs[0])
	}
}

func TestListJobs_StateFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT id, kind").
		WithArgs("running", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(10, "deploy", "default", "running")...))

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.ListJobs(context.Background(), &adminv1.ListJobsRequest{State: "running"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 1 || resp.Jobs[0].State != "running" {
		t.Errorf("unexpected jobs: %+v", resp.Jobs)
	}
}

func TestListJobs_BeforeIDPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("id <").
		WithArgs(int64(10), 2).
		WillReturnRows(sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(9, "deploy", "default", "completed")...).
			AddRow(riverJobRowVals(8, "deploy", "default", "completed")...))

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.ListJobs(context.Background(), &adminv1.ListJobsRequest{BeforeID: 10, Limit: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 1 || resp.Jobs[0].ID != 9 {
		t.Fatalf("jobs = %+v, want only job 9", resp.Jobs)
	}
	if !resp.HasMore || resp.NextBeforeID != 9 {
		t.Fatalf("pagination = has_more:%v next_before_id:%d, want true/9", resp.HasMore, resp.NextBeforeID)
	}
}

func TestListJobs_AnchorIDPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("id =").
		WithArgs(int64(50), 1).
		WillReturnRows(sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(50, "deploy", "default", "completed")...))
	mock.ExpectQuery("id >").
		WithArgs(int64(50), 1).
		WillReturnRows(sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(55, "deploy", "default", "completed")...))
	mock.ExpectQuery("id <").
		WithArgs(int64(50), 2).
		WillReturnRows(sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(49, "deploy", "default", "completed")...).
			AddRow(riverJobRowVals(48, "deploy", "default", "completed")...))

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.ListJobs(context.Background(), &adminv1.ListJobsRequest{AnchorID: 50, Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 3 || resp.Jobs[0].ID != 55 || resp.Jobs[1].ID != 50 || resp.Jobs[2].ID != 49 {
		t.Fatalf("jobs = %+v, want [55, 50, 49]", resp.Jobs)
	}
	if !resp.HasMore || resp.NextBeforeID != 49 {
		t.Fatalf("pagination = has_more:%v next_before_id:%d, want true/49", resp.HasMore, resp.NextBeforeID)
	}
}

func TestListJobs_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT id, kind").WillReturnError(errors.New("connection refused"))

	s := &Server{db: db, log: logger.New("error", "json")}
	_, err = s.ListJobs(context.Background(), &adminv1.ListJobsRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("status.Code(err) = %v, want %v (err=%v)", status.Code(err), codes.Internal, err)
	}
}

func TestGetJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT id, kind").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(riverJobCols).
			AddRow(riverJobRowVals(42, "reconcile", "default", "completed")...))

	s := &Server{db: db, log: logger.New("error", "json")}
	resp, err := s.GetJob(context.Background(), &adminv1.GetJobRequest{ID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Job.ID != 42 || resp.Job.Kind != "reconcile" {
		t.Errorf("job = %+v, want id=42 kind=reconcile", resp.Job)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT id, kind").
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows(riverJobCols))

	s := &Server{db: db, log: logger.New("error", "json")}
	_, err = s.GetJob(context.Background(), &adminv1.GetJobRequest{ID: 99})
	if err == nil {
		t.Fatal("expected error for missing job, got nil")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("status.Code(err) = %v, want %v (err=%v)", status.Code(err), codes.NotFound, err)
	}
}

func TestCancelJobs(t *testing.T) {
	q := &mockJobsQueue{}
	s := &Server{queue: q, log: logger.New("error", "json")}

	resp, err := s.CancelJobs(context.Background(), &adminv1.CancelJobsRequest{IDs: []int64{1, 2, 3}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Cancelled != 3 {
		t.Errorf("Cancelled = %d, want 3", resp.Cancelled)
	}
	if len(q.cancelCalls) != 3 {
		t.Errorf("cancel calls = %d, want 3", len(q.cancelCalls))
	}
}

func TestCancelJobs_PartialFailure(t *testing.T) {
	q := &mockJobsQueue{cancelErr: errors.New("already finalized")}
	s := &Server{queue: q, log: logger.New("error", "json")}

	resp, err := s.CancelJobs(context.Background(), &adminv1.CancelJobsRequest{IDs: []int64{1, 2}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Cancelled != 0 {
		t.Errorf("Cancelled = %d, want 0 on total failure", resp.Cancelled)
	}
}

func TestRetryJobs(t *testing.T) {
	q := &mockJobsQueue{}
	s := &Server{queue: q, log: logger.New("error", "json")}

	resp, err := s.RetryJobs(context.Background(), &adminv1.RetryJobsRequest{IDs: []int64{5, 6}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Retried != 2 {
		t.Errorf("Retried = %d, want 2", resp.Retried)
	}
	if len(q.retryCalls) != 2 || q.retryCalls[0] != 5 || q.retryCalls[1] != 6 {
		t.Errorf("retry calls = %v, want [5 6]", q.retryCalls)
	}
}

func TestRetryJobs_IgnoresNoopRetry(t *testing.T) {
	q := &mockJobsQueue{retryNoops: map[int64]bool{6: true}}
	s := &Server{queue: q, log: logger.New("error", "json")}

	resp, err := s.RetryJobs(context.Background(), &adminv1.RetryJobsRequest{IDs: []int64{5, 6}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Retried != 1 {
		t.Errorf("Retried = %d, want 1", resp.Retried)
	}
	if len(q.retryCalls) != 2 || q.retryCalls[0] != 5 || q.retryCalls[1] != 6 {
		t.Errorf("retry calls = %v, want [5 6]", q.retryCalls)
	}
}

func TestPauseQueue(t *testing.T) {
	q := &mockJobsQueue{}
	s := &Server{queue: q, log: logger.New("error", "json")}

	_, err := s.PauseQueue(context.Background(), &adminv1.PauseQueueRequest{Name: "deploy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.pauseCalls) != 1 || q.pauseCalls[0] != "deploy" {
		t.Errorf("pause calls = %v, want [deploy]", q.pauseCalls)
	}
}

func TestPauseQueue_Error(t *testing.T) {
	q := &mockJobsQueue{pauseErr: errors.New("queue not found")}
	s := &Server{queue: q, log: logger.New("error", "json")}

	_, err := s.PauseQueue(context.Background(), &adminv1.PauseQueueRequest{Name: "unknown"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResumeQueue(t *testing.T) {
	q := &mockJobsQueue{}
	s := &Server{queue: q, log: logger.New("error", "json")}

	_, err := s.ResumeQueue(context.Background(), &adminv1.ResumeQueueRequest{Name: "deploy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.resumeCalls) != 1 || q.resumeCalls[0] != "deploy" {
		t.Errorf("resume calls = %v, want [deploy]", q.resumeCalls)
	}
}
