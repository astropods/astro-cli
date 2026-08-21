package admingrpc

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// An orphaned deployment cannot be re-applied where it sits, so re-apply moves
// it to the account's default cluster instead.
func TestReapplyDeployment_EnqueuesMigrationWhenOrphaned(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{
		db:          db,
		deployStore: deploymentstore.NewStore(db),
		queue:       q,
		log:         logger.New("error", "json"),
		bindings: &fakeClusterBindings{
			list: []account.ClusterBinding{{ClusterID: "cluster-a", Region: "region-a", IsDefault: true}},
		},
	}

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, "cluster-b",
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil, nil,
			))

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
	if call.deploymentID != "dep-1" || call.targetClusterID != "cluster-a" || call.sourceClusterID != "cluster-b" {
		t.Fatalf("unexpected migrate call: %+v", call)
	}
}

// A deployment on an allowed cluster re-applies in place, with no migration.
func TestReapplyDeployment_NoMigrationWhenAllowed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{
		db:          db,
		deployStore: deploymentstore.NewStore(db),
		queue:       q,
		log:         logger.New("error", "json"),
		bindings: &fakeClusterBindings{
			list: []account.ClusterBinding{{ClusterID: "cluster-a", Region: "region-a", IsDefault: true}},
		},
	}

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows(deploymentFullColumns).
			AddRow(
				"dep-1", "acct-1", nil, "agent-a", "build-1", "astro-ns-0", "Agent A",
				`{"target":{"runtime":"kubernetes"}}`, nil, nil, "cluster-a",
				"active", nil, nil, time.Now(), 1,
				time.Now(), nil, nil, nil,
			))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := s.ReapplyDeployment(context.Background(), &adminv1.ReapplyDeploymentRequest{
		DeploymentId: "dep-1",
	})
	if err != nil {
		t.Fatalf("ReapplyDeployment: %v", err)
	}
	if resp.ClusterPlacementUpdated {
		t.Error("allowed cluster should not trigger a placement change")
	}
	if len(q.migrateCalls) != 0 {
		t.Fatalf("migrate calls = %d, want 0", len(q.migrateCalls))
	}
}
