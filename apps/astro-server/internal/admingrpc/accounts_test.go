package admingrpc

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func TestSetAccountCluster_Clear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, log: logger.New("error", "json")}

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "",
	})
	if err != nil {
		t.Fatalf("SetAccountCluster: %v", err)
	}
	if resp.Status != "updated" {
		t.Fatalf("status = %q, want updated", resp.Status)
	}
}

func TestSetAccountCluster_DisabledCluster(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	cs := clusterstore.New(db)
	s := &Server{db: db, clusterStore: cs, log: logger.New("error", "json")}

	now := "2026-05-19T00:00:00Z"
	mock.ExpectQuery("SELECT .+ FROM clusters").
		WithArgs("staging-disabled").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "enabled", "created_at", "updated_at",
		}).AddRow("staging-disabled", "us-east-1", "eks", "https://eks.example", false, now, now))

	_, err = s.SetAccountCluster(context.Background(), &adminv1.SetAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "staging-disabled",
	})
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
}
