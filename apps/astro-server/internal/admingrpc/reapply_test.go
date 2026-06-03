package admingrpc

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func TestReapplyDeployment_EnqueuesMigrationOnPlacementMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	eu := "eu"
	s := &Server{
		db:          db,
		deployStore: deploymentstore.NewStore(db),
		queue:       q,
		log:         logger.New("error", "json"),
	}

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, nil,
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil,
			))
	mock.ExpectQuery("SELECT COALESCE\\(cluster_id").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"cluster_id"}).AddRow(eu))

	resp, err := s.ReapplyDeployment(context.Background(), &adminv1.ReapplyDeploymentRequest{
		DeploymentId: "dep-1",
	})
	if err != nil {
		t.Fatalf("ReapplyDeployment: %v", err)
	}
	if !resp.ClusterPlacementUpdated {
		t.Fatal("expected cluster placement updated")
	}
	if len(q.migrateCalls) != 1 {
		t.Fatalf("migrate calls = %d, want 1", len(q.migrateCalls))
	}
	call := q.migrateCalls[0]
	if call.deploymentID != "dep-1" || call.targetClusterID != eu || call.sourceClusterID != "" {
		t.Fatalf("unexpected migrate call: %+v", call)
	}
}
