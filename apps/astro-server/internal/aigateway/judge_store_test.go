package aigateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var judgeKeyColumns = []string{
	"account_id", "key_id", "encrypted_api_key", "encrypted_data_key", "nonce",
	"issued_at", "created_at", "updated_at",
}

func TestJudgeStoreGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns).AddRow(
			"acct-1", "vk-1", "ciphertext", []byte("data-key"), []byte("nonce"), now, now, now,
		))

	key, err := NewJudgeStore(db).Get(context.Background(), "acct-1")
	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "acct-1", key.AccountID)
	assert.Equal(t, "vk-1", key.KeyID)
	assert.Equal(t, "ciphertext", key.EncryptedAPIKey)
	assert.Equal(t, []byte("data-key"), key.EncryptedDataKey)
	assert.Equal(t, []byte("nonce"), key.Nonce)
	assert.WithinDuration(t, now, key.IssuedAt, time.Second)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestJudgeStoreGetMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(judgeKeyColumns))

	key, err := NewJudgeStore(db).Get(context.Background(), "acct-1")
	require.NoError(t, err)
	assert.Nil(t, key)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestJudgeStoreGetHonorsCanceledContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	key, err := NewJudgeStore(db).Get(ctx, "acct-1")
	assert.Nil(t, key)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestJudgeStoreSaveDeleteAndList(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectExec("INSERT INTO account_llm_judge_keys").
		WithArgs("acct-1", "vk-1", "plaintext-dev", []byte(nil), []byte(nil), now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-1"))
	mock.ExpectExec("DELETE FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewJudgeStore(db)
	require.NoError(t, store.Save(context.Background(), &JudgeKey{
		AccountID:       "acct-1",
		KeyID:           "vk-1",
		EncryptedAPIKey: "plaintext-dev",
		IssuedAt:        now,
	}))
	ids, err := store.ListKeyIDsByAccount(context.Background(), "acct-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"vk-1"}, ids)
	require.NoError(t, store.Delete(context.Background(), "acct-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestJudgeStoreErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func(*JudgeStore) error
		set  func(sqlmock.Sqlmock)
		want string
	}{
		{
			name: "get",
			run: func(s *JudgeStore) error {
				_, err := s.Get(context.Background(), "acct-1")
				return err
			},
			set: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("FROM account_llm_judge_keys").WillReturnError(errors.New("read failed"))
			},
			want: "read failed",
		},
		{
			name: "save",
			run: func(s *JudgeStore) error {
				return s.Save(context.Background(), &JudgeKey{AccountID: "acct-1"})
			},
			set: func(m sqlmock.Sqlmock) {
				m.ExpectExec("INSERT INTO account_llm_judge_keys").WillReturnError(errors.New("write failed"))
			},
			want: "write failed",
		},
		{
			name: "delete",
			run:  func(s *JudgeStore) error { return s.Delete(context.Background(), "acct-1") },
			set: func(m sqlmock.Sqlmock) {
				m.ExpectExec("DELETE FROM account_llm_judge_keys").WillReturnError(errors.New("delete failed"))
			},
			want: "delete failed",
		},
		{
			name: "list",
			run: func(s *JudgeStore) error {
				_, err := s.ListKeyIDsByAccount(context.Background(), "acct-1")
				return err
			},
			set: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").WillReturnError(errors.New("list failed"))
			},
			want: "list failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			tt.set(mock)
			err = tt.run(NewJudgeStore(db))
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.want), err.Error())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
