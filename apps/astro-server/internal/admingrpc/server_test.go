package admingrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

func TestProxyOpenMeter(t *testing.T) {
	tests := []struct {
		name       string
		omURL      string // empty = no upstream
		req        *adminv1.OpenMeterProxyRequest
		handler    http.HandlerFunc // upstream handler
		wantStatus int32
		wantBody   string
		wantErr    bool
	}{
		{
			name: "GET with path and query string",
			req: &adminv1.OpenMeterProxyRequest{
				Method:  "GET",
				Path:    "/api/v1/meters?window=DAY",
				Headers: map[string]string{"Accept": "application/json"},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("method = %q, want GET", r.Method)
				}
				if got := r.URL.RequestURI(); got != "/api/v1/meters?window=DAY" {
					t.Errorf("path = %q, want /api/v1/meters?window=DAY", got)
				}
				if got := r.Header.Get("Accept"); got != "application/json" {
					t.Errorf("Accept header = %q, want application/json", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"meters":[]}`)) //nolint:errcheck
			},
			wantStatus: 200,
			wantBody:   `{"meters":[]}`,
		},
		{
			name: "POST with JSON body",
			req: &adminv1.OpenMeterProxyRequest{
				Method:  "POST",
				Path:    "/api/v1/events",
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    []byte(`{"type":"test"}`),
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("method = %q, want POST", r.Method)
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"type":"test"}` {
					t.Errorf("body = %q, want %q", body, `{"type":"test"}`)
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
			},
			wantStatus: 201,
			wantBody:   `{"ok":true}`,
		},
		{
			name: "upstream non-200 status propagated",
			req: &adminv1.OpenMeterProxyRequest{
				Method: "GET",
				Path:   "/api/v1/missing",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"not found"}`)) //nolint:errcheck
			},
			wantStatus: 404,
			wantBody:   `{"error":"not found"}`,
		},
		{
			name:  "empty openMeterURL returns error",
			omURL: "", // explicitly empty
			req: &adminv1.OpenMeterProxyRequest{
				Method: "GET",
				Path:   "/api/v1/meters",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var omURL string
			if tt.handler != nil {
				ts := httptest.NewServer(tt.handler)
				defer ts.Close()
				omURL = ts.URL
			} else {
				omURL = tt.omURL
			}

			srv := New(nil, nil, nil, nil, nil, omURL, "", nil, "", "")
			resp, err := srv.ProxyOpenMeter(context.Background(), tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if got := string(resp.Body); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

// deploymentRow returns sqlmock rows for GetDeploymentByID matching deploymentstore scan order.
func deploymentRow(id, accountID, namespace string) *sqlmock.Rows {
	now := time.Now()
	rev := 1
	return sqlmock.NewRows([]string{
		"id", "account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at",
	}).AddRow(
		id, accountID, "my-agent", "build-1", namespace,
		"My Agent", json.RawMessage(`{}`), []byte(nil), (*string)(nil),
		"active", (*string)(nil), json.RawMessage(nil), now, &rev,
		now, (*time.Time)(nil),
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
	srv := New(nil, deployStore, nil, lokiClient, nil, "", "", nil, "", "")

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
	srv := New(nil, nil, nil, nil, nil, "", "", nil, "", "")
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
	srv := New(nil, deployStore, nil, nil, nil, "", "", nil, "", "")

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
	srv := New(nil, deployStore, nil, nil, nil, "", "", nil, "", "")

	_, err := srv.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{
		DeploymentId: "dep-1",
		// Pod intentionally omitted
	})
	if err == nil {
		t.Fatal("expected error when pod omitted with no Loki backend, got nil")
	}
}
