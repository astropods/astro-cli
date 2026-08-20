package langfuse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestComputeFastHash(t *testing.T) {
	// Reference: sha256(secretKey + hex(sha256(salt)))
	ref := func(key, salt string) string {
		saltDigest := sha256.Sum256([]byte(salt))
		saltHex := hex.EncodeToString(saltDigest[:])
		final := sha256.Sum256([]byte(key + saltHex))
		return hex.EncodeToString(final[:])
	}

	tests := []struct {
		name string
		key  string
		salt string
	}{
		{"known inputs", "mykey", "mysalt"},
		{"empty salt", "mykey", ""},
		{"empty key", "", "mysalt"},
		{"both empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeFastHash(tt.key, tt.salt)
			want := ref(tt.key, tt.salt)
			if got != want {
				t.Errorf("computeFastHash(%q, %q) = %s, want %s", tt.key, tt.salt, got, want)
			}
			if len(got) != 64 {
				t.Errorf("expected 64-char hex, got length %d", len(got))
			}
		})
	}
}

func TestGenerateCUID_Format(t *testing.T) {
	t.Run("length is 24", func(t *testing.T) {
		id := generateCUID()
		if len(id) != 24 {
			t.Fatalf("expected length 24, got %d (%q)", len(id), id)
		}
	})

	t.Run("starts with lowercase letter", func(t *testing.T) {
		for i := 0; i < 200; i++ {
			id := generateCUID()
			if id[0] < 'a' || id[0] > 'z' {
				t.Fatalf("expected first char a-z, got %q in %q", id[0], id)
			}
		}
	})

	t.Run("all lowercase alphanumeric", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := generateCUID()
			for j, c := range id {
				if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
					t.Fatalf("char %d (%q) in %q is not lowercase alphanumeric", j, string(c), id)
				}
			}
		}
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			id := generateCUID()
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate CUID after %d calls: %q", i, id)
			}
			seen[id] = struct{}{}
		}
	})
}

func newProvisionerMock(t *testing.T) (*Provisioner, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Provisioner{langfuseDB: db, orgID: "org-1"}, mock
}

func TestDeleteProjectHardDeletesAPIKeys(t *testing.T) {
	// Langfuse's api_keys table has no deleted_at column, and its auth path
	// reads the table unfiltered, so only a row delete revokes the credentials.
	p, mock := newProvisionerMock(t)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_keys WHERE project_id = \$1`).
		WithArgs("project-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE projects SET deleted_at = \$1 WHERE id = \$2 AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), "project-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := p.DeleteProject(context.Background(), "project-1"); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProjectIsIdempotent(t *testing.T) {
	p, mock := newProvisionerMock(t)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_keys`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE projects`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := p.DeleteProject(context.Background(), "project-1"); err != nil {
		t.Fatalf("DeleteProject() on already-purged project error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProjectRollsBackAPIKeyFailure(t *testing.T) {
	p, mock := newProvisionerMock(t)

	cause := errors.New("api key delete failed")
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_keys`).WillReturnError(cause)
	mock.ExpectRollback()

	err := p.DeleteProject(context.Background(), "project-1")
	if !errors.Is(err, cause) {
		t.Fatalf("DeleteProject() error = %v, want %v", err, cause)
	}
	if !strings.Contains(err.Error(), "delete api keys") {
		t.Fatalf("DeleteProject() error = %v, want it wrapped with %q", err, "delete api keys")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProjectRollsBackProjectFailure(t *testing.T) {
	p, mock := newProvisionerMock(t)

	cause := errors.New("project update failed")
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_keys`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE projects`).WillReturnError(cause)
	mock.ExpectRollback()

	err := p.DeleteProject(context.Background(), "project-1")
	if !errors.Is(err, cause) {
		t.Fatalf("DeleteProject() error = %v, want %v", err, cause)
	}
	if !strings.Contains(err.Error(), "delete project") {
		t.Fatalf("DeleteProject() error = %v, want it wrapped with %q", err, "delete project")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProjectBeginFailure(t *testing.T) {
	p, mock := newProvisionerMock(t)

	cause := errors.New("no connection")
	mock.ExpectBegin().WillReturnError(cause)

	err := p.DeleteProject(context.Background(), "project-1")
	if !errors.Is(err, cause) {
		t.Fatalf("DeleteProject() error = %v, want %v", err, cause)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
