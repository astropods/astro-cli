package clusterstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestValidateID(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"us-east-1-managed", true},
		{"eu", true},
		{"prod-us-east-1-managed", true},
		{"a1", true},
		{"", false},
		{"a", false},
		{"-leading-dash", false},
		{"trailing-dash-", false},
		{"UPPER", false},
		{"with_underscore", false},
		{"with.dot", false},
	}
	for _, tc := range cases {
		err := ValidateID(tc.id)
		if (err == nil) != tc.valid {
			t.Errorf("ValidateID(%q): valid=%v but err=%v", tc.id, tc.valid, err)
		}
	}
}

func TestRegister_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("INSERT INTO clusters").
		WithArgs(
			"us-east-1-managed", "us-east-1",
			"prod-managed-eks", "https://eks.example",
			true,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Register(context.Background(), &Cluster{
		ID:                 "us-east-1-managed",
		Region:             "us-east-1",
		EKSClusterName:     "prod-managed-eks",
		EKSClusterEndpoint: "https://eks.example",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRegister_DuplicateReturnsAlreadyExists(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("INSERT INTO clusters").
		WillReturnError(&pq.Error{Code: pgUniqueViolation, Constraint: "clusters_pkey"})

	err := store.Register(context.Background(), &Cluster{
		ID:                 "us-east-1-managed",
		Region:             "us-east-1",
		EKSClusterName:     "prod-managed-eks",
		EKSClusterEndpoint: "https://eks.example",
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestRegister_RejectsInvalidID(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	err := store.Register(context.Background(), &Cluster{ID: "BAD"})
	if err == nil {
		t.Error("expected error for invalid id")
	}
}

func TestRegister_RejectsMissingRequiredFields(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	cases := []struct {
		name    string
		cluster *Cluster
	}{
		{"missing region", &Cluster{ID: "us-east-1", EKSClusterName: "n", EKSClusterEndpoint: "https://e"}},
		{"missing eks name", &Cluster{ID: "us-east-1", Region: "us-east-1", EKSClusterEndpoint: "https://e"}},
		{"missing endpoint", &Cluster{ID: "us-east-1", Region: "us-east-1", EKSClusterName: "n"}},
	}
	for _, tc := range cases {
		if err := store.Register(context.Background(), tc.cluster); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestGet_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnRows(clusterRows().AddRow(
			"us-east-1-managed", "us-east-1",
			"prod-managed-eks", "https://eks.example",
			true, now, now,
		))

	c, err := store.Get(context.Background(), "us-east-1-managed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID != "us-east-1-managed" || c.Region != "us-east-1" || !c.Enabled {
		t.Errorf("got %+v", c)
	}
}

func TestGet_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectQuery("SELECT .+ FROM clusters WHERE id = \\$1").
		WithArgs("missing").
		WillReturnRows(clusterRows())

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestList_All(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM clusters ORDER BY region ASC, id ASC").
		WillReturnRows(clusterRows().
			AddRow("a", "ap-southeast-2", "eks-a", "https://a", false, now, now).
			AddRow("b", "us-east-1", "eks-b", "https://b", true, now, now))

	cs, err := store.List(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(cs))
	}
}

func TestList_EnabledOnly(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM clusters WHERE enabled = true ORDER BY region ASC, id ASC").
		WillReturnRows(clusterRows().
			AddRow("b", "us-east-1", "eks-b", "https://b", true, now, now))

	cs, err := store.List(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs) != 1 || !cs[0].Enabled {
		t.Errorf("unexpected list: %+v", cs)
	}
}

func TestSetEnabled_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("UPDATE clusters SET enabled = \\$1").
		WithArgs(false, "us-east-1-managed").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetEnabled(context.Background(), "us-east-1-managed", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetEnabled_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("UPDATE clusters SET enabled = \\$1").
		WithArgs(true, "missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.SetEnabled(context.Background(), "missing", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeregister_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Deregister(context.Background(), "us-east-1-managed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeregister_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.Deregister(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeregister_InUse(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: "deployments_cluster_id_fkey"})

	if err := store.Deregister(context.Background(), "us-east-1-managed"); !errors.Is(err, ErrInUse) {
		t.Errorf("expected ErrInUse, got %v", err)
	}
}

// clusterRows returns a sqlmock.Rows with the column projection used by
// baseSelect. Test rows can be appended via .AddRow.
func clusterRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "region", "eks_cluster_name", "eks_cluster_endpoint",
		"enabled", "created_at", "updated_at",
	})
}
