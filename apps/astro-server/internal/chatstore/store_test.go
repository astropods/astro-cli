package chatstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestUpsertAssistantProgress_InsertsAfterUserTurn(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	deploymentID := uuid.NewString()
	userID := "user_01"
	lockID := conversationAdvisoryLockID(convID)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(lockID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT deployment_id FROM deployment_chat_conversations`).
		WithArgs(convID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(deploymentID))
	mock.ExpectQuery(`SELECT id, role, COALESCE\(seq, 0\)`).
		WithArgs(convID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "seq"}).
			AddRow(uuid.NewString(), "user", 1))
	mock.ExpectExec(`INSERT INTO deployment_chat_messages`).
		WithArgs(sqlmock.AnyArg(), convID, "new reply", 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployment_chat_conversations SET updated_at`).
		WithArgs(convID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	store := New(db)
	gotID, err := store.UpsertAssistantProgress(
		context.Background(),
		deploymentID,
		userID,
		convID,
		"new reply",
	)
	if err != nil {
		t.Fatalf("UpsertAssistantProgress: %v", err)
	}
	if gotID == "" {
		t.Fatal("expected new assistant message id")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertAssistantProgress_UpdatesInFlightAssistant(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	prevAssistantID := uuid.NewString()
	deploymentID := uuid.NewString()
	userID := "user_01"
	lockID := conversationAdvisoryLockID(convID)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(lockID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT deployment_id FROM deployment_chat_conversations`).
		WithArgs(convID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(deploymentID))
	mock.ExpectQuery(`SELECT id, role, COALESCE\(seq, 0\)`).
		WithArgs(convID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "seq"}).
			AddRow(prevAssistantID, "assistant", 2))
	mock.ExpectExec(`UPDATE deployment_chat_messages SET content`).
		WithArgs("partial reply", prevAssistantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployment_chat_conversations SET updated_at`).
		WithArgs(convID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	store := New(db)
	gotID, err := store.UpsertAssistantProgress(
		context.Background(),
		deploymentID,
		userID,
		convID,
		"partial reply",
	)
	if err != nil {
		t.Fatalf("UpsertAssistantProgress: %v", err)
	}
	if gotID != prevAssistantID {
		t.Fatalf("expected in-flight update of %s, got %s", prevAssistantID, gotID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertConversation_ConflictReturnsError(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	deploymentID := uuid.NewString()
	accountID := uuid.NewString()
	userID := "user_01"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO deployment_chat_conversations`).
		WithArgs(convID, deploymentID, accountID, userID, "Title").
		WillReturnResult(sqlmock.NewResult(0, 0))

	store := New(db)
	err = store.UpsertConversation(
		context.Background(),
		accountID,
		deploymentID,
		userID,
		convID,
		"Title",
	)
	if !errors.Is(err, ErrConversationIDConflict) {
		t.Fatalf("expected ErrConversationIDConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceMessages_BlocksActiveAssistantStream(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	deploymentID := uuid.NewString()
	userID := "user_01"
	lockID := conversationAdvisoryLockID(convID)
	activeAt := time.Now().Add(-time.Minute)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(lockID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT deployment_id, assistant_stream_active_at`).
		WithArgs(convID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "assistant_stream_active_at"}).
			AddRow(deploymentID, activeAt))
	mock.ExpectRollback()

	store := New(db)
	err = store.ReplaceMessages(context.Background(), deploymentID, userID, convID, nil)
	if !errors.Is(err, ErrActiveAssistantStream) {
		t.Fatalf("expected ErrActiveAssistantStream, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDedupeMessages_DuplicateSeqKeepsLongerContent(t *testing.T) {
	t.Parallel()

	idShort := uuid.NewString()
	idLong := uuid.NewString()
	got := dedupeMessages([]Message{
		{ID: idShort, Role: "assistant", Content: "short", Seq: 2},
		{ID: idLong, Role: "assistant", Content: "much longer reply", Seq: 2},
	})
	if len(got) != 1 {
		t.Fatalf("expected one message, got %d", len(got))
	}
	if got[0].Content != "much longer reply" {
		t.Fatalf("expected longer duplicate seq to win, got %q", got[0].Content)
	}
}

func TestAppendUserMessage_ConversationIDConflict(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	deploymentID := uuid.NewString()
	accountID := uuid.NewString()
	userID := "user_02"
	lockID := conversationAdvisoryLockID(convID)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(lockID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT deployment_id, assistant_stream_active_at`).
		WithArgs(convID, userID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT 1 FROM deployment_chat_conversations WHERE id`).
		WithArgs(convID).
		WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	mock.ExpectRollback()

	store := New(db)
	err = store.AppendUserMessage(
		context.Background(),
		accountID,
		deploymentID,
		userID,
		convID,
		"Title",
		Message{ID: uuid.NewString(), Role: "user", Content: "hi"},
	)
	if !errors.Is(err, ErrConversationIDConflict) {
		t.Fatalf("expected ErrConversationIDConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendUserMessage_BlocksActiveAssistantStream(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	deploymentID := uuid.NewString()
	accountID := uuid.NewString()
	userID := "user_01"
	lockID := conversationAdvisoryLockID(convID)
	activeAt := time.Now().Add(-time.Minute)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(lockID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT deployment_id, assistant_stream_active_at`).
		WithArgs(convID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "assistant_stream_active_at"}).
			AddRow(deploymentID, activeAt))
	mock.ExpectRollback()

	store := New(db)
	err = store.AppendUserMessage(
		context.Background(),
		accountID,
		deploymentID,
		userID,
		convID,
		"Title",
		Message{ID: uuid.NewString(), Role: "user", Content: "hi"},
	)
	if !errors.Is(err, ErrActiveAssistantStream) {
		t.Fatalf("expected ErrActiveAssistantStream, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendMessage_BlocksActiveAssistantStream(t *testing.T) {
	t.Parallel()

	convID := uuid.NewString()
	deploymentID := uuid.NewString()
	userID := "user_01"
	lockID := conversationAdvisoryLockID(convID)
	activeAt := time.Now().Add(-time.Minute)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(lockID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT deployment_id, assistant_stream_active_at`).
		WithArgs(convID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "assistant_stream_active_at"}).
			AddRow(deploymentID, activeAt))
	mock.ExpectRollback()

	store := New(db)
	err = store.AppendMessage(
		context.Background(),
		deploymentID,
		userID,
		convID,
		Message{ID: uuid.NewString(), Role: "user", Content: "hi"},
	)
	if !errors.Is(err, ErrActiveAssistantStream) {
		t.Fatalf("expected ErrActiveAssistantStream, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDedupeMessages_CollapsesConsecutiveAssistants(t *testing.T) {
	t.Parallel()

	idA := uuid.NewString()
	idB := uuid.NewString()
	got := dedupeMessages([]Message{
		{ID: uuid.NewString(), Role: "user", Content: "hi", Seq: 1},
		{ID: idA, Role: "assistant", Content: "partial", Seq: 2},
		{ID: idB, Role: "assistant", Content: "partial and more", Seq: 3},
	})
	if len(got) != 2 {
		t.Fatalf("expected user + one assistant, got %d", len(got))
	}
	if got[1].ID != idB || got[1].Content != "partial and more" {
		t.Fatalf("expected longest consecutive assistant, got %+v", got[1])
	}
}
