package admingrpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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

			srv := New(nil, nil, nil, nil, omURL, "", nil, "", "")
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
