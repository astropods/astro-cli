package chatstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestAppendUserMessage_RejectsConversationOwnedByAnotherUser(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)

	convID := "7def4844-10b5-47b1-a860-b0a52b31e65e"
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs(conversationAdvisoryLockID("dep-1", convID)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Caller user_id does not match the active row.
	mock.ExpectQuery("SELECT assistant_stream_active_at").
		WithArgs("dep-1", convID, "user-b").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT user_id FROM deployment_chat_conversations").
		WithArgs("dep-1", convID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user-a"))
	mock.ExpectRollback()

	err = store.AppendUserMessage(
		context.Background(),
		"acct-1", "dep-1", "user-b", convID, "hello",
		Message{Role: "user", Content: "hello"},
	)
	if !errors.Is(err, ErrConversationIDConflict) {
		t.Fatalf("expected ErrConversationIDConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
