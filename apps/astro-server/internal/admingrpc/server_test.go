package admingrpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// deploymentRow returns sqlmock rows for GetDeploymentByID matching deploymentstore scan order.
func deploymentRow(id, accountID, namespace string) *sqlmock.Rows {
	now := time.Now()
	rev := 1
	return sqlmock.NewRows([]string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	}).AddRow(
		id, accountID, nil, "my-agent", "build-1", namespace,
		"My Agent", json.RawMessage(`{}`), []byte(nil), (*string)(nil), nil,
		"active", (*string)(nil), json.RawMessage(nil), now, &rev,
		now, (*time.Time)(nil), nil, nil,
	)
}

func TestGetPodLogs_LokiPath(t *testing.T) {
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod"},
					"values": [
						["1000000000", "hello from loki"],
						["2000000000", "second line"]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer lokiSrv.Close()

	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	mock.ExpectQuery(`SELECT`).WillReturnRows(deploymentRow("dep-1", "acct-1", "astro-abc-0"))

	deployStore := deploymentstore.NewStore(db)
	lokiClient := loki.New(lokiSrv.URL)
	srv := New(nil, deployStore, nil, lokiClient, nil, "", nil, nil, nil, nil, nil)

	resp, err := srv.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{
		DeploymentId: "dep-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Logs != "hello from loki\nsecond line\n" {
		t.Errorf("logs = %q, want %q", resp.Logs, "hello from loki\nsecond line\n")
	}
}

func TestGetPodLogs_MissingDeploymentID(t *testing.T) {
	srv := New(nil, nil, nil, nil, nil, "", nil, nil, nil, nil, nil)
	_, err := srv.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{})
	if err == nil {
		t.Fatal("expected error for missing deployment_id, got nil")
	}
}

func TestGetPodLogs_NoBackendConfigured(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	mock.ExpectQuery(`SELECT`).WillReturnRows(deploymentRow("dep-1", "acct-1", "astro-abc-0"))

	deployStore := deploymentstore.NewStore(db)
	srv := New(nil, deployStore, nil, nil, nil, "", nil, nil, nil, nil, nil)

	_, err := srv.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{
		DeploymentId: "dep-1",
		Pod:          "my-pod",
	})
	if err == nil {
		t.Fatal("expected error when no backend configured, got nil")
	}
}

func TestGetPodLogs_K8sFallback_PodRequired(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	mock.ExpectQuery(`SELECT`).WillReturnRows(deploymentRow("dep-1", "acct-1", "astro-abc-0"))

	deployStore := deploymentstore.NewStore(db)
	// k8sClient is nil but lokiClient is also nil — pod should be required
	srv := New(nil, deployStore, nil, nil, nil, "", nil, nil, nil, nil, nil)

	_, err := srv.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{
		DeploymentId: "dep-1",
		// Pod intentionally omitted
	})
	if err == nil {
		t.Fatal("expected error when pod omitted with no Loki backend, got nil")
	}
}
