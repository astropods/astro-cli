package admingrpc

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// The orphan check reads only deployments and account_clusters, so no query
// spends a positional argument on it and every mismatch query keeps $1 for its
// LIMIT. A row with no cluster recorded is left alone rather than resolved.
func TestOrphanedDeploymentPredicateTakesNoArgument(t *testing.T) {
	if strings.Contains(orphanedDeploymentPredicate, "$") {
		t.Errorf("predicate should take no positional argument: %s", orphanedDeploymentPredicate)
	}
	if !strings.Contains(orphanedDeploymentPredicate, "d.cluster_id IS NOT NULL") {
		t.Errorf("predicate should skip rows with no cluster recorded: %s", orphanedDeploymentPredicate)
	}
	for _, q := range []string{clusterMigrationEventsMismatchQuery, clusterMigrationJobsMismatchQuery} {
		if !strings.Contains(q, "LIMIT $1") {
			t.Errorf("mismatch query should keep LIMIT $1: %s", q)
		}
		if strings.Contains(q, "$2") {
			t.Errorf("mismatch query should need no second argument: %s", q)
		}
	}
}

func TestCountPlacementMismatchesTakesNoArgument(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	s := &Server{db: db, log: logger.New("error", "json")}
	got, err := s.countPlacementMismatches(context.Background())
	if err != nil {
		t.Fatalf("countPlacementMismatches: %v", err)
	}
	if got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
