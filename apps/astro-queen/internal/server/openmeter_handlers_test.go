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
func (m *mockAdminClient) StartRiverUI(context.Context, *adminv1.StartRiverUIRequest, ...grpc.CallOption) (*adminv1.StartRiverUIResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) StopRiverUI(context.Context, *adminv1.StopRiverUIRequest, ...grpc.CallOption) (*adminv1.StopRiverUIResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetRiverUIStatus(context.Context, *adminv1.GetRiverUIStatusRequest, ...grpc.CallOption) (*adminv1.GetRiverUIStatusResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ListQuotaIncreaseRequests(context.Context, *adminv1.ListQuotaIncreaseRequestsRequest, ...grpc.CallOption) (*adminv1.ListQuotaIncreaseRequestsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ApproveQuotaIncrease(context.Context, *adminv1.ApproveQuotaIncreaseRequest, ...grpc.CallOption) (*adminv1.ApproveQuotaIncreaseResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) DenyQuotaIncrease(context.Context, *adminv1.DenyQuotaIncreaseRequest, ...grpc.CallOption) (*adminv1.DenyQuotaIncreaseResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetDeploymentEvents(context.Context, *adminv1.GetDeploymentEventsRequest, ...grpc.CallOption) (*adminv1.GetDeploymentEventsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) WakeUpDeployment(context.Context, *adminv1.WakeUpDeploymentRequest, ...grpc.CallOption) (*adminv1.WakeUpDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) StopDeployment(context.Context, *adminv1.StopDeploymentRequest, ...grpc.CallOption) (*adminv1.StopDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) RollbackDeployment(context.Context, *adminv1.RollbackDeploymentRequest, ...grpc.CallOption) (*adminv1.RollbackDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ReapplyDeployment(context.Context, *adminv1.ReapplyDeploymentRequest, ...grpc.CallOption) (*adminv1.ReapplyDeploymentResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) GetDeploymentJobs(context.Context, *adminv1.GetDeploymentJobsRequest, ...grpc.CallOption) (*adminv1.GetDeploymentJobsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) RepairNormalizedSpec(context.Context, *adminv1.RepairNormalizedSpecRequest, ...grpc.CallOption) (*adminv1.RepairNormalizedSpecResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) RefreshDriftReport(context.Context, *adminv1.RefreshDriftReportRequest, ...grpc.CallOption) (*adminv1.RefreshDriftReportResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) BackfillResolvedKeys(context.Context, *adminv1.BackfillResolvedKeysRequest, ...grpc.CallOption) (*adminv1.BackfillResolvedKeysResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) SetAdapters(context.Context, *adminv1.SetAdaptersRequest, ...grpc.CallOption) (*adminv1.SetAdaptersResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) TriggerOpenMeterBackfill(context.Context, *adminv1.TriggerOpenMeterBackfillRequest, ...grpc.CallOption) (*adminv1.TriggerOpenMeterBackfillResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) ListFeedback(context.Context, *adminv1.ListFeedbackRequest, ...grpc.CallOption) (*adminv1.ListFeedbackResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) RegisterCluster(context.Context, *adminv1.RegisterClusterRequest, ...grpc.CallOption) (*adminv1.RegisterClusterResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) EnableCluster(context.Context, *adminv1.EnableClusterRequest, ...grpc.CallOption) (*adminv1.EnableClusterResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) DisableCluster(context.Context, *adminv1.DisableClusterRequest, ...grpc.CallOption) (*adminv1.DisableClusterResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) DeregisterCluster(context.Context, *adminv1.DeregisterClusterRequest, ...grpc.CallOption) (*adminv1.DeregisterClusterResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ListClusters(context.Context, *adminv1.ListClustersRequest, ...grpc.CallOption) (*adminv1.ListClustersResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) SetAccountCluster(context.Context, *adminv1.SetAccountClusterRequest, ...grpc.CallOption) (*adminv1.SetAccountClusterResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) UpdateCluster(context.Context, *adminv1.UpdateClusterRequest, ...grpc.CallOption) (*adminv1.UpdateClusterResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) CheckClusterHealth(context.Context, *adminv1.CheckClusterHealthRequest, ...grpc.CallOption) (*adminv1.CheckClusterHealthResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) InvalidateAccountCaches(context.Context, *adminv1.InvalidateAccountCachesRequest, ...grpc.CallOption) (*adminv1.InvalidateCachesResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) ListClusterMigrations(context.Context, *adminv1.ListClusterMigrationsRequest, ...grpc.CallOption) (*adminv1.ListClusterMigrationsResponse, error) {
	panic("not implemented")
}
func (m *mockAdminClient) InvalidateAllCaches(context.Context, *adminv1.InvalidateAllCachesRequest, ...grpc.CallOption) (*adminv1.InvalidateCachesResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) ListJobKinds(context.Context, *adminv1.ListJobKindsRequest, ...grpc.CallOption) (*adminv1.ListJobKindsResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) TriggerJob(context.Context, *adminv1.TriggerJobRequest, ...grpc.CallOption) (*adminv1.TriggerJobResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) GetJobStates(context.Context, *adminv1.GetJobStatesRequest, ...grpc.CallOption) (*adminv1.GetJobStatesResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) ListAdminQueues(context.Context, *adminv1.ListAdminQueuesRequest, ...grpc.CallOption) (*adminv1.ListAdminQueuesResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) ListJobs(context.Context, *adminv1.ListJobsRequest, ...grpc.CallOption) (*adminv1.ListJobsResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) GetJob(context.Context, *adminv1.GetJobRequest, ...grpc.CallOption) (*adminv1.GetJobResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) CancelJobs(context.Context, *adminv1.CancelJobsRequest, ...grpc.CallOption) (*adminv1.CancelJobsResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) RetryJobs(context.Context, *adminv1.RetryJobsRequest, ...grpc.CallOption) (*adminv1.RetryJobsResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) PauseQueue(context.Context, *adminv1.PauseQueueRequest, ...grpc.CallOption) (*adminv1.PauseQueueResponse, error) {
	panic("not implemented")
}

func (m *mockAdminClient) ResumeQueue(context.Context, *adminv1.ResumeQueueRequest, ...grpc.CallOption) (*adminv1.ResumeQueueResponse, error) {
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
			srv := New(&mockAdminClient{proxyFn: tt.proxyFn}, nil, 0, nil, "test")

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
