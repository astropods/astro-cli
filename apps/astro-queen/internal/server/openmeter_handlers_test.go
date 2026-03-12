package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"google.golang.org/grpc"
)

// mockAdminClient implements adminv1.AdminServiceClient with only ProxyOpenMeter.
type mockAdminClient struct {
	proxyFn func(ctx context.Context, req *adminv1.OpenMeterProxyRequest) (*adminv1.OpenMeterProxyResponse, error)
}

func (m *mockAdminClient) ProxyOpenMeter(ctx context.Context, in *adminv1.OpenMeterProxyRequest, _ ...grpc.CallOption) (*adminv1.OpenMeterProxyResponse, error) {
	return m.proxyFn(ctx, in)
}

func (m *mockAdminClient) ListDeployments(context.Context, *adminv1.ListDeploymentsRequest, ...grpc.CallOption) (*adminv1.ListDeploymentsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetDeployment(context.Context, *adminv1.GetDeploymentRequest, ...grpc.CallOption) (*adminv1.GetDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetClusterStatus(context.Context, *adminv1.GetClusterStatusRequest, ...grpc.CallOption) (*adminv1.GetClusterStatusResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) DeleteDeployment(context.Context, *adminv1.DeleteDeploymentRequest, ...grpc.CallOption) (*adminv1.DeleteDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) RestartDeployment(context.Context, *adminv1.RestartDeploymentRequest, ...grpc.CallOption) (*adminv1.RestartDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ListAccounts(context.Context, *adminv1.ListAccountsRequest, ...grpc.CallOption) (*adminv1.ListAccountsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) RenameAccount(context.Context, *adminv1.RenameAccountRequest, ...grpc.CallOption) (*adminv1.RenameAccountResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetPodLogs(context.Context, *adminv1.GetPodLogsRequest, ...grpc.CallOption) (*adminv1.GetPodLogsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetPodEnv(context.Context, *adminv1.GetPodEnvRequest, ...grpc.CallOption) (*adminv1.GetPodEnvResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ListAgents(context.Context, *adminv1.ListAgentsRequest, ...grpc.CallOption) (*adminv1.ListAgentsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetAgentBuilds(context.Context, *adminv1.GetAgentBuildsRequest, ...grpc.CallOption) (*adminv1.GetAgentBuildsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ListConnectedDevices(context.Context, *adminv1.ListConnectedDevicesRequest, ...grpc.CallOption) (*adminv1.ListConnectedDevicesResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) SendCommand(context.Context, *adminv1.SendCommandRequest, ...grpc.CallOption) (*adminv1.SendCommandResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ProxyHTTP(context.Context, *adminv1.HTTPProxyRequest, ...grpc.CallOption) (*adminv1.HTTPProxyResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetAuthConfig(context.Context, *adminv1.GetAuthConfigRequest, ...grpc.CallOption) (*adminv1.GetAuthConfigResponse, error) {
	panic("not implemented")
}

func TestOMReverseProxy(t *testing.T) {
	tests := []struct {
		name       string
		reqPath    string
		proxyFn    func(ctx context.Context, req *adminv1.OpenMeterProxyRequest) (*adminv1.OpenMeterProxyResponse, error)
		wantStatus int
		wantBody   string
	}{
		{
			name:    "successful proxy",
			reqPath: "/api/openmeter/api/v1/meters",
			proxyFn: func(_ context.Context, req *adminv1.OpenMeterProxyRequest) (*adminv1.OpenMeterProxyResponse, error) {
				return &adminv1.OpenMeterProxyResponse{
					StatusCode: 200,
					Headers:    map[string]string{"Content-Type": "application/json"},
					Body:       []byte(`{"meters":[]}`),
				}, nil
			},
			wantStatus: 200,
			wantBody:   `{"meters":[]}`,
		},
		{
			name:    "gRPC error returns 502",
			reqPath: "/api/openmeter/api/v1/meters",
			proxyFn: func(_ context.Context, _ *adminv1.OpenMeterProxyRequest) (*adminv1.OpenMeterProxyResponse, error) {
				return nil, fmt.Errorf("upstream unreachable")
			},
			wantStatus: 502,
		},
		{
			name:    "path stripping with query string",
			reqPath: "/api/openmeter/api/v1/meters?q=1",
			proxyFn: func(_ context.Context, req *adminv1.OpenMeterProxyRequest) (*adminv1.OpenMeterProxyResponse, error) {
				if req.Path != "/api/v1/meters?q=1" {
					return nil, fmt.Errorf("path = %q, want /api/v1/meters?q=1", req.Path)
				}
				return &adminv1.OpenMeterProxyResponse{
					StatusCode: 200,
					Body:       []byte("ok"),
				}, nil
			},
			wantStatus: 200,
			wantBody:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&mockAdminClient{proxyFn: tt.proxyFn}, nil, 0, nil)

			req := httptest.NewRequest("GET", tt.reqPath, nil)
			rec := httptest.NewRecorder()

			srv.omReverseProxy(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" {
				got := strings.TrimSpace(rec.Body.String())
				if !strings.Contains(got, tt.wantBody) {
					t.Errorf("body = %q, want to contain %q", got, tt.wantBody)
				}
			}
		})
	}
}
