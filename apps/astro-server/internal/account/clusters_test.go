package account

import (
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

func clusterRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"cluster_id", "region", "is_default"})
}

func TestListClusters_MaterializesThePrimary(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(clusterRows())
	mock.ExpectExec("INSERT INTO account_clusters").
		WithArgs("acct-1", "cluster-default").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(clusterRows().AddRow("cluster-default", "region-a", true))

	got, err := NewClusterBindings(db, clusterid.New("cluster-default")).List("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ClusterID != "cluster-default" || !got[0].IsDefault {
		t.Fatalf("got %+v, want cluster-default flagged default", got)
	}
	if got[0].Region != "region-a" {
		t.Errorf("region = %q, want region-a", got[0].Region)
	}
	if !IsAllowed("cluster-default", got) {
		t.Error("the primary should be allowed once bound")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListClusters_ExistingBindingsAreNotTouched(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(clusterRows().AddRow("cluster-a", "region-a", true))

	got, err := NewClusterBindings(db, clusterid.New("cluster-default")).List("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ClusterID != "cluster-a" {
		t.Fatalf("got %+v, want only cluster-a", got)
	}
	if IsAllowed("cluster-default", got) {
		t.Error("a bound account's set is exhaustive")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListClusters_UnregisteredPrimaryLeavesTheAccountUnbound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(clusterRows())
	mock.ExpectExec("INSERT INTO account_clusters").
		WithArgs("acct-1", "missing").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(clusterRows())

	got, err := NewClusterBindings(db, clusterid.New("missing")).List("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want empty", got)
	}
	if !IsAllowed("missing", got) {
		t.Error("an account with no bindings is unrestricted")
	}
}

func TestListClusters_NoDefaultConfigured(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(clusterRows())

	got, err := NewClusterBindings(db, clusterid.New("")).List("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want empty", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListClusters_DefaultFirst(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"cluster_id", "region", "is_default"}).
			AddRow("cluster-a", "region-a", true).
			AddRow("cluster-b", "region-b", false))

	got, err := NewClusterBindings(db, clusterid.New("cluster-default")).List("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if DefaultClusterID(got) != "cluster-a" {
		t.Errorf("default = %q, want cluster-a", DefaultClusterID(got))
	}
}

func TestDefaultClusterID_WithoutAFlaggedDefault(t *testing.T) {
	sorted := []ClusterBinding{
		{ClusterID: "cluster-a", Region: "region-a"},
		{ClusterID: "cluster-b", Region: "region-b"},
	}
	reversed := []ClusterBinding{sorted[1], sorted[0]}

	if got := DefaultClusterID(sorted); got != "cluster-a" {
		t.Errorf("default = %q, want cluster-a", got)
	}
	if got := DefaultClusterID(reversed); got != "cluster-a" {
		t.Errorf("default = %q, want cluster-a regardless of order", got)
	}
	if got := DefaultClusterID(nil); got != "" {
		t.Errorf("default = %q, want empty", got)
	}
}

func TestAddCluster_RejectsUnknownCluster(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT true FROM clusters").
		WithArgs("nope").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if err := NewClusterBindings(db, clusterid.New("cluster-default")).Add("acct-1", "nope", false); err == nil {
		t.Fatal("expected an error for an unknown cluster")
	}
}

func expectAddCluster(mock sqlmock.Sqlmock, clusterID string, accountHasDefault, clusterIsDefault bool) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT true FROM clusters").
		WithArgs(clusterID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO account_clusters").
		WithArgs("acct-1", "cluster-default").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("COALESCE").
		WithArgs("acct-1", clusterID).
		WillReturnRows(sqlmock.NewRows([]string{"account_has_default", "cluster_is_default"}).
			AddRow(accountHasDefault, clusterIsDefault))
}

func TestAddCluster_DefaultFlag(t *testing.T) {
	tests := []struct {
		name              string
		setDefault        bool
		accountHasDefault bool
		clusterIsDefault  bool
		wantDefault       bool
	}{
		{name: "an explicit default is honored", setDefault: true, accountHasDefault: true, wantDefault: true},
		{name: "the first binding takes the flag unasked", wantDefault: true},
		{name: "a second binding does not steal the flag", accountHasDefault: true, wantDefault: false},
		{name: "re-adding the current default never clears it", accountHasDefault: true, clusterIsDefault: true, wantDefault: true},
		{name: "a set left with no default gets one back", accountHasDefault: false, wantDefault: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			expectAddCluster(mock, "cluster-a", tc.accountHasDefault, tc.clusterIsDefault)
			if tc.wantDefault {
				mock.ExpectExec("UPDATE account_clusters SET is_default = false").
					WithArgs("acct-1").
					WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec("INSERT INTO account_clusters").
				WithArgs("acct-1", "cluster-a", tc.wantDefault).
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()

			if err := NewClusterBindings(db, clusterid.New("cluster-default")).Add("acct-1", "cluster-a", tc.setDefault); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestRemoveCluster_RefusesWhileDeploymentsAttached(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count").
		WithArgs("acct-1", "cluster-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectRollback()

	err := NewClusterBindings(db, clusterid.New("cluster-default")).Remove("acct-1", "cluster-a")
	if !errors.Is(err, ErrClusterInUse) {
		t.Fatalf("err = %v, want ErrClusterInUse", err)
	}
}

func TestRemoveCluster_DeletesWhenUnused(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count").
		WithArgs("acct-1", "cluster-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("DELETE FROM account_clusters").
		WithArgs("acct-1", "cluster-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE account_clusters SET is_default = true").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewClusterBindings(db, clusterid.New("cluster-default")).Remove("acct-1", "cluster-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSetDefaultCluster_RejectsUnboundCluster(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE account_clusters SET is_default = false").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE account_clusters SET is_default = true").
		WithArgs("acct-1", "cluster-c").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := NewClusterBindings(db, clusterid.New("cluster-default")).SetDefault("acct-1", "cluster-c")
	if !errors.Is(err, ErrClusterNotAllowed) {
		t.Fatalf("err = %v, want ErrClusterNotAllowed", err)
	}
}

func TestIsAllowed(t *testing.T) {
	allowed := []ClusterBinding{{ClusterID: "cluster-a"}}

	if !IsAllowed("cluster-a", allowed) {
		t.Error("bound cluster should be allowed")
	}
	if IsAllowed("cluster-b", allowed) {
		t.Error("unbound cluster should be rejected")
	}
	if IsAllowed("cluster-primary", allowed) {
		t.Error("an unbound primary should be rejected too")
	}
	withPrimary := []ClusterBinding{{ClusterID: "cluster-primary"}, {ClusterID: "cluster-a"}}
	if !IsAllowed("cluster-primary", withPrimary) {
		t.Error("the primary is allowed once it is bound")
	}

	if !IsAllowed("cluster-b", nil) {
		t.Error("an account with no bindings should not be blocked")
	}
}
