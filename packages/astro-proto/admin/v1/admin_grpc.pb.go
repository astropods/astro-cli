// Code generated manually to match proto/admin/v1/admin.proto.
// Regenerate by running: buf generate (requires buf CLI)

package adminv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AdminServiceClient is the client API for AdminService.
type AdminServiceClient interface {
	ListDeployments(ctx context.Context, in *ListDeploymentsRequest, opts ...grpc.CallOption) (*ListDeploymentsResponse, error)
	GetDeployment(ctx context.Context, in *GetDeploymentRequest, opts ...grpc.CallOption) (*GetDeploymentResponse, error)
	GetClusterStatus(ctx context.Context, in *GetClusterStatusRequest, opts ...grpc.CallOption) (*GetClusterStatusResponse, error)
	DeleteDeployment(ctx context.Context, in *DeleteDeploymentRequest, opts ...grpc.CallOption) (*DeleteDeploymentResponse, error)
	RestartDeployment(ctx context.Context, in *RestartDeploymentRequest, opts ...grpc.CallOption) (*RestartDeploymentResponse, error)
	ListAccounts(ctx context.Context, in *ListAccountsRequest, opts ...grpc.CallOption) (*ListAccountsResponse, error)
	RenameAccount(ctx context.Context, in *RenameAccountRequest, opts ...grpc.CallOption) (*RenameAccountResponse, error)
	GetPodLogs(ctx context.Context, in *GetPodLogsRequest, opts ...grpc.CallOption) (*GetPodLogsResponse, error)
	GetPodEnv(ctx context.Context, in *GetPodEnvRequest, opts ...grpc.CallOption) (*GetPodEnvResponse, error)
	ListAgents(ctx context.Context, in *ListAgentsRequest, opts ...grpc.CallOption) (*ListAgentsResponse, error)
	GetAgentBuilds(ctx context.Context, in *GetAgentBuildsRequest, opts ...grpc.CallOption) (*GetAgentBuildsResponse, error)
	ProxyOpenMeter(ctx context.Context, in *OpenMeterProxyRequest, opts ...grpc.CallOption) (*OpenMeterProxyResponse, error)
	ProxyHTTP(ctx context.Context, in *HTTPProxyRequest, opts ...grpc.CallOption) (*HTTPProxyResponse, error)
	GetAuthConfig(ctx context.Context, in *GetAuthConfigRequest, opts ...grpc.CallOption) (*GetAuthConfigResponse, error)
	ListConnectedDevices(ctx context.Context, in *ListConnectedDevicesRequest, opts ...grpc.CallOption) (*ListConnectedDevicesResponse, error)
	SendCommand(ctx context.Context, in *SendCommandRequest, opts ...grpc.CallOption) (*SendCommandResponse, error)
	StartRiverUI(ctx context.Context, in *StartRiverUIRequest, opts ...grpc.CallOption) (*StartRiverUIResponse, error)
	StopRiverUI(ctx context.Context, in *StopRiverUIRequest, opts ...grpc.CallOption) (*StopRiverUIResponse, error)
	GetRiverUIStatus(ctx context.Context, in *GetRiverUIStatusRequest, opts ...grpc.CallOption) (*GetRiverUIStatusResponse, error)
	ListQuotaIncreaseRequests(ctx context.Context, in *ListQuotaIncreaseRequestsRequest, opts ...grpc.CallOption) (*ListQuotaIncreaseRequestsResponse, error)
	ApproveQuotaIncrease(ctx context.Context, in *ApproveQuotaIncreaseRequest, opts ...grpc.CallOption) (*ApproveQuotaIncreaseResponse, error)
	DenyQuotaIncrease(ctx context.Context, in *DenyQuotaIncreaseRequest, opts ...grpc.CallOption) (*DenyQuotaIncreaseResponse, error)
	GetDeploymentEvents(ctx context.Context, in *GetDeploymentEventsRequest, opts ...grpc.CallOption) (*GetDeploymentEventsResponse, error)
	WakeUpDeployment(ctx context.Context, in *WakeUpDeploymentRequest, opts ...grpc.CallOption) (*WakeUpDeploymentResponse, error)
	RollbackDeployment(ctx context.Context, in *RollbackDeploymentRequest, opts ...grpc.CallOption) (*RollbackDeploymentResponse, error)
	ReapplyDeployment(ctx context.Context, in *ReapplyDeploymentRequest, opts ...grpc.CallOption) (*ReapplyDeploymentResponse, error)
	GetDeploymentJobs(ctx context.Context, in *GetDeploymentJobsRequest, opts ...grpc.CallOption) (*GetDeploymentJobsResponse, error)
	RepairNormalizedSpec(ctx context.Context, in *RepairNormalizedSpecRequest, opts ...grpc.CallOption) (*RepairNormalizedSpecResponse, error)
	RefreshDriftReport(ctx context.Context, in *RefreshDriftReportRequest, opts ...grpc.CallOption) (*RefreshDriftReportResponse, error)
	BackfillResolvedKeys(ctx context.Context, in *BackfillResolvedKeysRequest, opts ...grpc.CallOption) (*BackfillResolvedKeysResponse, error)
	SetAdapters(ctx context.Context, in *SetAdaptersRequest, opts ...grpc.CallOption) (*SetAdaptersResponse, error)
}

type adminServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAdminServiceClient(cc grpc.ClientConnInterface) AdminServiceClient {
	return &adminServiceClient{cc}
}

func (c *adminServiceClient) ListDeployments(ctx context.Context, in *ListDeploymentsRequest, opts ...grpc.CallOption) (*ListDeploymentsResponse, error) {
	out := new(ListDeploymentsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListDeployments", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeployment(ctx context.Context, in *GetDeploymentRequest, opts ...grpc.CallOption) (*GetDeploymentResponse, error) {
	out := new(GetDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetClusterStatus(ctx context.Context, in *GetClusterStatusRequest, opts ...grpc.CallOption) (*GetClusterStatusResponse, error) {
	out := new(GetClusterStatusResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetClusterStatus", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DeleteDeployment(ctx context.Context, in *DeleteDeploymentRequest, opts ...grpc.CallOption) (*DeleteDeploymentResponse, error) {
	out := new(DeleteDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DeleteDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RestartDeployment(ctx context.Context, in *RestartDeploymentRequest, opts ...grpc.CallOption) (*RestartDeploymentResponse, error) {
	out := new(RestartDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RestartDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAccounts(ctx context.Context, in *ListAccountsRequest, opts ...grpc.CallOption) (*ListAccountsResponse, error) {
	out := new(ListAccountsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAccounts", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RenameAccount(ctx context.Context, in *RenameAccountRequest, opts ...grpc.CallOption) (*RenameAccountResponse, error) {
	out := new(RenameAccountResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RenameAccount", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetPodLogs(ctx context.Context, in *GetPodLogsRequest, opts ...grpc.CallOption) (*GetPodLogsResponse, error) {
	out := new(GetPodLogsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetPodLogs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetPodEnv(ctx context.Context, in *GetPodEnvRequest, opts ...grpc.CallOption) (*GetPodEnvResponse, error) {
	out := new(GetPodEnvResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetPodEnv", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListAgents(ctx context.Context, in *ListAgentsRequest, opts ...grpc.CallOption) (*ListAgentsResponse, error) {
	out := new(ListAgentsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListAgents", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAgentBuilds(ctx context.Context, in *GetAgentBuildsRequest, opts ...grpc.CallOption) (*GetAgentBuildsResponse, error) {
	out := new(GetAgentBuildsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAgentBuilds", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ProxyOpenMeter(ctx context.Context, in *OpenMeterProxyRequest, opts ...grpc.CallOption) (*OpenMeterProxyResponse, error) {
	out := new(OpenMeterProxyResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ProxyOpenMeter", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ProxyHTTP(ctx context.Context, in *HTTPProxyRequest, opts ...grpc.CallOption) (*HTTPProxyResponse, error) {
	out := new(HTTPProxyResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ProxyHTTP", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetAuthConfig(ctx context.Context, in *GetAuthConfigRequest, opts ...grpc.CallOption) (*GetAuthConfigResponse, error) {
	out := new(GetAuthConfigResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetAuthConfig", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListConnectedDevices(ctx context.Context, in *ListConnectedDevicesRequest, opts ...grpc.CallOption) (*ListConnectedDevicesResponse, error) {
	out := new(ListConnectedDevicesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListConnectedDevices", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) SendCommand(ctx context.Context, in *SendCommandRequest, opts ...grpc.CallOption) (*SendCommandResponse, error) {
	out := new(SendCommandResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/SendCommand", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StartRiverUI(ctx context.Context, in *StartRiverUIRequest, opts ...grpc.CallOption) (*StartRiverUIResponse, error) {
	out := new(StartRiverUIResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StartRiverUI", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) StopRiverUI(ctx context.Context, in *StopRiverUIRequest, opts ...grpc.CallOption) (*StopRiverUIResponse, error) {
	out := new(StopRiverUIResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/StopRiverUI", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetRiverUIStatus(ctx context.Context, in *GetRiverUIStatusRequest, opts ...grpc.CallOption) (*GetRiverUIStatusResponse, error) {
	out := new(GetRiverUIStatusResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetRiverUIStatus", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ListQuotaIncreaseRequests(ctx context.Context, in *ListQuotaIncreaseRequestsRequest, opts ...grpc.CallOption) (*ListQuotaIncreaseRequestsResponse, error) {
	out := new(ListQuotaIncreaseRequestsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListQuotaIncreaseRequests", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ApproveQuotaIncrease(ctx context.Context, in *ApproveQuotaIncreaseRequest, opts ...grpc.CallOption) (*ApproveQuotaIncreaseResponse, error) {
	out := new(ApproveQuotaIncreaseResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ApproveQuotaIncrease", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DenyQuotaIncrease(ctx context.Context, in *DenyQuotaIncreaseRequest, opts ...grpc.CallOption) (*DenyQuotaIncreaseResponse, error) {
	out := new(DenyQuotaIncreaseResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/DenyQuotaIncrease", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeploymentEvents(ctx context.Context, in *GetDeploymentEventsRequest, opts ...grpc.CallOption) (*GetDeploymentEventsResponse, error) {
	out := new(GetDeploymentEventsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeploymentEvents", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) WakeUpDeployment(ctx context.Context, in *WakeUpDeploymentRequest, opts ...grpc.CallOption) (*WakeUpDeploymentResponse, error) {
	out := new(WakeUpDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/WakeUpDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RollbackDeployment(ctx context.Context, in *RollbackDeploymentRequest, opts ...grpc.CallOption) (*RollbackDeploymentResponse, error) {
	out := new(RollbackDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RollbackDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) ReapplyDeployment(ctx context.Context, in *ReapplyDeploymentRequest, opts ...grpc.CallOption) (*ReapplyDeploymentResponse, error) {
	out := new(ReapplyDeploymentResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ReapplyDeployment", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetDeploymentJobs(ctx context.Context, in *GetDeploymentJobsRequest, opts ...grpc.CallOption) (*GetDeploymentJobsResponse, error) {
	out := new(GetDeploymentJobsResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetDeploymentJobs", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RepairNormalizedSpec(ctx context.Context, in *RepairNormalizedSpecRequest, opts ...grpc.CallOption) (*RepairNormalizedSpecResponse, error) {
	out := new(RepairNormalizedSpecResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RepairNormalizedSpec", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) RefreshDriftReport(ctx context.Context, in *RefreshDriftReportRequest, opts ...grpc.CallOption) (*RefreshDriftReportResponse, error) {
	out := new(RefreshDriftReportResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/RefreshDriftReport", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) BackfillResolvedKeys(ctx context.Context, in *BackfillResolvedKeysRequest, opts ...grpc.CallOption) (*BackfillResolvedKeysResponse, error) {
	out := new(BackfillResolvedKeysResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/BackfillResolvedKeys", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) SetAdapters(ctx context.Context, in *SetAdaptersRequest, opts ...grpc.CallOption) (*SetAdaptersResponse, error) {
	out := new(SetAdaptersResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/SetAdapters", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminServiceServer is the server API for AdminService.
// Embed UnimplementedAdminServiceServer for forward compatibility.
type AdminServiceServer interface {
	ListDeployments(context.Context, *ListDeploymentsRequest) (*ListDeploymentsResponse, error)
	GetDeployment(context.Context, *GetDeploymentRequest) (*GetDeploymentResponse, error)
	GetClusterStatus(context.Context, *GetClusterStatusRequest) (*GetClusterStatusResponse, error)
	DeleteDeployment(context.Context, *DeleteDeploymentRequest) (*DeleteDeploymentResponse, error)
	RestartDeployment(context.Context, *RestartDeploymentRequest) (*RestartDeploymentResponse, error)
	ListAccounts(context.Context, *ListAccountsRequest) (*ListAccountsResponse, error)
	RenameAccount(context.Context, *RenameAccountRequest) (*RenameAccountResponse, error)
	GetPodLogs(context.Context, *GetPodLogsRequest) (*GetPodLogsResponse, error)
	GetPodEnv(context.Context, *GetPodEnvRequest) (*GetPodEnvResponse, error)
	ListAgents(context.Context, *ListAgentsRequest) (*ListAgentsResponse, error)
	GetAgentBuilds(context.Context, *GetAgentBuildsRequest) (*GetAgentBuildsResponse, error)
	ProxyOpenMeter(context.Context, *OpenMeterProxyRequest) (*OpenMeterProxyResponse, error)
	ProxyHTTP(context.Context, *HTTPProxyRequest) (*HTTPProxyResponse, error)
	GetAuthConfig(context.Context, *GetAuthConfigRequest) (*GetAuthConfigResponse, error)
	ListConnectedDevices(context.Context, *ListConnectedDevicesRequest) (*ListConnectedDevicesResponse, error)
	SendCommand(context.Context, *SendCommandRequest) (*SendCommandResponse, error)
	StartRiverUI(context.Context, *StartRiverUIRequest) (*StartRiverUIResponse, error)
	StopRiverUI(context.Context, *StopRiverUIRequest) (*StopRiverUIResponse, error)
	GetRiverUIStatus(context.Context, *GetRiverUIStatusRequest) (*GetRiverUIStatusResponse, error)
	ListQuotaIncreaseRequests(context.Context, *ListQuotaIncreaseRequestsRequest) (*ListQuotaIncreaseRequestsResponse, error)
	ApproveQuotaIncrease(context.Context, *ApproveQuotaIncreaseRequest) (*ApproveQuotaIncreaseResponse, error)
	DenyQuotaIncrease(context.Context, *DenyQuotaIncreaseRequest) (*DenyQuotaIncreaseResponse, error)
	GetDeploymentEvents(context.Context, *GetDeploymentEventsRequest) (*GetDeploymentEventsResponse, error)
	WakeUpDeployment(context.Context, *WakeUpDeploymentRequest) (*WakeUpDeploymentResponse, error)
	RollbackDeployment(context.Context, *RollbackDeploymentRequest) (*RollbackDeploymentResponse, error)
	ReapplyDeployment(context.Context, *ReapplyDeploymentRequest) (*ReapplyDeploymentResponse, error)
	GetDeploymentJobs(context.Context, *GetDeploymentJobsRequest) (*GetDeploymentJobsResponse, error)
	RepairNormalizedSpec(context.Context, *RepairNormalizedSpecRequest) (*RepairNormalizedSpecResponse, error)
	RefreshDriftReport(context.Context, *RefreshDriftReportRequest) (*RefreshDriftReportResponse, error)
	BackfillResolvedKeys(context.Context, *BackfillResolvedKeysRequest) (*BackfillResolvedKeysResponse, error)
	SetAdapters(context.Context, *SetAdaptersRequest) (*SetAdaptersResponse, error)
	mustEmbedUnimplementedAdminServiceServer()
}

// UnimplementedAdminServiceServer must be embedded to have forward compatible implementations.
type UnimplementedAdminServiceServer struct{}

func (UnimplementedAdminServiceServer) ListDeployments(context.Context, *ListDeploymentsRequest) (*ListDeploymentsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListDeployments not implemented")
}

func (UnimplementedAdminServiceServer) GetDeployment(context.Context, *GetDeploymentRequest) (*GetDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeployment not implemented")
}

func (UnimplementedAdminServiceServer) GetClusterStatus(context.Context, *GetClusterStatusRequest) (*GetClusterStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetClusterStatus not implemented")
}

func (UnimplementedAdminServiceServer) DeleteDeployment(context.Context, *DeleteDeploymentRequest) (*DeleteDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteDeployment not implemented")
}

func (UnimplementedAdminServiceServer) RestartDeployment(context.Context, *RestartDeploymentRequest) (*RestartDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestartDeployment not implemented")
}

func (UnimplementedAdminServiceServer) ListAccounts(context.Context, *ListAccountsRequest) (*ListAccountsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAccounts not implemented")
}

func (UnimplementedAdminServiceServer) RenameAccount(context.Context, *RenameAccountRequest) (*RenameAccountResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RenameAccount not implemented")
}

func (UnimplementedAdminServiceServer) GetPodLogs(context.Context, *GetPodLogsRequest) (*GetPodLogsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPodLogs not implemented")
}

func (UnimplementedAdminServiceServer) GetPodEnv(context.Context, *GetPodEnvRequest) (*GetPodEnvResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPodEnv not implemented")
}

func (UnimplementedAdminServiceServer) ListAgents(context.Context, *ListAgentsRequest) (*ListAgentsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListAgents not implemented")
}

func (UnimplementedAdminServiceServer) GetAgentBuilds(context.Context, *GetAgentBuildsRequest) (*GetAgentBuildsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAgentBuilds not implemented")
}

func (UnimplementedAdminServiceServer) ProxyOpenMeter(context.Context, *OpenMeterProxyRequest) (*OpenMeterProxyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ProxyOpenMeter not implemented")
}

func (UnimplementedAdminServiceServer) ProxyHTTP(context.Context, *HTTPProxyRequest) (*HTTPProxyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ProxyHTTP not implemented")
}

func (UnimplementedAdminServiceServer) GetAuthConfig(context.Context, *GetAuthConfigRequest) (*GetAuthConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAuthConfig not implemented")
}

func (UnimplementedAdminServiceServer) ListConnectedDevices(context.Context, *ListConnectedDevicesRequest) (*ListConnectedDevicesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListConnectedDevices not implemented")
}

func (UnimplementedAdminServiceServer) SendCommand(context.Context, *SendCommandRequest) (*SendCommandResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendCommand not implemented")
}

func (UnimplementedAdminServiceServer) StartRiverUI(context.Context, *StartRiverUIRequest) (*StartRiverUIResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartRiverUI not implemented")
}

func (UnimplementedAdminServiceServer) StopRiverUI(context.Context, *StopRiverUIRequest) (*StopRiverUIResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StopRiverUI not implemented")
}

func (UnimplementedAdminServiceServer) GetRiverUIStatus(context.Context, *GetRiverUIStatusRequest) (*GetRiverUIStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRiverUIStatus not implemented")
}

func (UnimplementedAdminServiceServer) ListQuotaIncreaseRequests(context.Context, *ListQuotaIncreaseRequestsRequest) (*ListQuotaIncreaseRequestsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListQuotaIncreaseRequests not implemented")
}

func (UnimplementedAdminServiceServer) ApproveQuotaIncrease(context.Context, *ApproveQuotaIncreaseRequest) (*ApproveQuotaIncreaseResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ApproveQuotaIncrease not implemented")
}

func (UnimplementedAdminServiceServer) DenyQuotaIncrease(context.Context, *DenyQuotaIncreaseRequest) (*DenyQuotaIncreaseResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DenyQuotaIncrease not implemented")
}

func (UnimplementedAdminServiceServer) GetDeploymentEvents(context.Context, *GetDeploymentEventsRequest) (*GetDeploymentEventsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeploymentEvents not implemented")
}

func (UnimplementedAdminServiceServer) WakeUpDeployment(context.Context, *WakeUpDeploymentRequest) (*WakeUpDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method WakeUpDeployment not implemented")
}

func (UnimplementedAdminServiceServer) RollbackDeployment(context.Context, *RollbackDeploymentRequest) (*RollbackDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RollbackDeployment not implemented")
}

func (UnimplementedAdminServiceServer) ReapplyDeployment(context.Context, *ReapplyDeploymentRequest) (*ReapplyDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReapplyDeployment not implemented")
}

func (UnimplementedAdminServiceServer) GetDeploymentJobs(context.Context, *GetDeploymentJobsRequest) (*GetDeploymentJobsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetDeploymentJobs not implemented")
}

func (UnimplementedAdminServiceServer) RepairNormalizedSpec(context.Context, *RepairNormalizedSpecRequest) (*RepairNormalizedSpecResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RepairNormalizedSpec not implemented")
}

func (UnimplementedAdminServiceServer) RefreshDriftReport(context.Context, *RefreshDriftReportRequest) (*RefreshDriftReportResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RefreshDriftReport not implemented")
}

func (UnimplementedAdminServiceServer) BackfillResolvedKeys(context.Context, *BackfillResolvedKeysRequest) (*BackfillResolvedKeysResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method BackfillResolvedKeys not implemented")
}

func (UnimplementedAdminServiceServer) SetAdapters(context.Context, *SetAdaptersRequest) (*SetAdaptersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SetAdapters not implemented")
}

func (UnimplementedAdminServiceServer) mustEmbedUnimplementedAdminServiceServer() {}

// UnsafeAdminServiceServer may be embedded to opt out of forward compatibility.
type UnsafeAdminServiceServer interface {
	mustEmbedUnimplementedAdminServiceServer()
}

// RegisterAdminServiceServer registers the server implementation.
func RegisterAdminServiceServer(s grpc.ServiceRegistrar, srv AdminServiceServer) {
	s.RegisterService(&AdminService_ServiceDesc, srv)
}

func _AdminService_ListDeployments_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListDeploymentsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListDeployments(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListDeployments"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListDeployments(ctx, req.(*ListDeploymentsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeployment(ctx, req.(*GetDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetClusterStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetClusterStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetClusterStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetClusterStatus"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetClusterStatus(ctx, req.(*GetClusterStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DeleteDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DeleteDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DeleteDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DeleteDeployment(ctx, req.(*DeleteDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RestartDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RestartDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RestartDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RestartDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RestartDeployment(ctx, req.(*RestartDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAccounts_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAccountsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAccounts(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAccounts"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAccounts(ctx, req.(*ListAccountsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RenameAccount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RenameAccountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RenameAccount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RenameAccount"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RenameAccount(ctx, req.(*RenameAccountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetPodLogs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPodLogsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetPodLogs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetPodLogs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetPodLogs(ctx, req.(*GetPodLogsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetPodEnv_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPodEnvRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetPodEnv(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetPodEnv"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetPodEnv(ctx, req.(*GetPodEnvRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListAgents_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListAgentsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListAgents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListAgents"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListAgents(ctx, req.(*ListAgentsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAgentBuilds_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAgentBuildsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAgentBuilds(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAgentBuilds"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAgentBuilds(ctx, req.(*GetAgentBuildsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ProxyOpenMeter_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(OpenMeterProxyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ProxyOpenMeter(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ProxyOpenMeter"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ProxyOpenMeter(ctx, req.(*OpenMeterProxyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ProxyHTTP_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HTTPProxyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ProxyHTTP(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ProxyHTTP"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ProxyHTTP(ctx, req.(*HTTPProxyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetAuthConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAuthConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetAuthConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetAuthConfig"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetAuthConfig(ctx, req.(*GetAuthConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListConnectedDevices_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListConnectedDevicesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListConnectedDevices(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListConnectedDevices"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListConnectedDevices(ctx, req.(*ListConnectedDevicesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_SendCommand_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendCommandRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).SendCommand(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/SendCommand"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).SendCommand(ctx, req.(*SendCommandRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StartRiverUI_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartRiverUIRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StartRiverUI(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StartRiverUI"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StartRiverUI(ctx, req.(*StartRiverUIRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_StopRiverUI_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopRiverUIRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).StopRiverUI(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/StopRiverUI"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).StopRiverUI(ctx, req.(*StopRiverUIRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetRiverUIStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRiverUIStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetRiverUIStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetRiverUIStatus"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetRiverUIStatus(ctx, req.(*GetRiverUIStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ListQuotaIncreaseRequests_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListQuotaIncreaseRequestsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListQuotaIncreaseRequests(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListQuotaIncreaseRequests"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListQuotaIncreaseRequests(ctx, req.(*ListQuotaIncreaseRequestsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ApproveQuotaIncrease_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ApproveQuotaIncreaseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ApproveQuotaIncrease(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ApproveQuotaIncrease"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ApproveQuotaIncrease(ctx, req.(*ApproveQuotaIncreaseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DenyQuotaIncrease_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DenyQuotaIncreaseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DenyQuotaIncrease(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/DenyQuotaIncrease"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DenyQuotaIncrease(ctx, req.(*DenyQuotaIncreaseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeploymentEvents_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentEventsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeploymentEvents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeploymentEvents"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeploymentEvents(ctx, req.(*GetDeploymentEventsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_WakeUpDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(WakeUpDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).WakeUpDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/WakeUpDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).WakeUpDeployment(ctx, req.(*WakeUpDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RollbackDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RollbackDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RollbackDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RollbackDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RollbackDeployment(ctx, req.(*RollbackDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_ReapplyDeployment_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReapplyDeploymentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ReapplyDeployment(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ReapplyDeployment"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ReapplyDeployment(ctx, req.(*ReapplyDeploymentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetDeploymentJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetDeploymentJobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetDeploymentJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetDeploymentJobs"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetDeploymentJobs(ctx, req.(*GetDeploymentJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RepairNormalizedSpec_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RepairNormalizedSpecRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RepairNormalizedSpec(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RepairNormalizedSpec"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RepairNormalizedSpec(ctx, req.(*RepairNormalizedSpecRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_RefreshDriftReport_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RefreshDriftReportRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).RefreshDriftReport(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/RefreshDriftReport"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).RefreshDriftReport(ctx, req.(*RefreshDriftReportRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_BackfillResolvedKeys_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BackfillResolvedKeysRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).BackfillResolvedKeys(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/BackfillResolvedKeys"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).BackfillResolvedKeys(ctx, req.(*BackfillResolvedKeysRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_SetAdapters_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SetAdaptersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).SetAdapters(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/SetAdapters"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).SetAdapters(ctx, req.(*SetAdaptersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// AdminService_ServiceDesc is the grpc.ServiceDesc for AdminService service.
var AdminService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "admin.v1.AdminService",
	HandlerType: (*AdminServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "ListDeployments", Handler: _AdminService_ListDeployments_Handler},
		{MethodName: "GetDeployment", Handler: _AdminService_GetDeployment_Handler},
		{MethodName: "GetClusterStatus", Handler: _AdminService_GetClusterStatus_Handler},
		{MethodName: "DeleteDeployment", Handler: _AdminService_DeleteDeployment_Handler},
		{MethodName: "RestartDeployment", Handler: _AdminService_RestartDeployment_Handler},
		{MethodName: "ListAccounts", Handler: _AdminService_ListAccounts_Handler},
		{MethodName: "RenameAccount", Handler: _AdminService_RenameAccount_Handler},
		{MethodName: "GetPodLogs", Handler: _AdminService_GetPodLogs_Handler},
		{MethodName: "GetPodEnv", Handler: _AdminService_GetPodEnv_Handler},
		{MethodName: "ListAgents", Handler: _AdminService_ListAgents_Handler},
		{MethodName: "GetAgentBuilds", Handler: _AdminService_GetAgentBuilds_Handler},
		{MethodName: "ProxyOpenMeter", Handler: _AdminService_ProxyOpenMeter_Handler},
		{MethodName: "ProxyHTTP", Handler: _AdminService_ProxyHTTP_Handler},
		{MethodName: "GetAuthConfig", Handler: _AdminService_GetAuthConfig_Handler},
		{MethodName: "ListConnectedDevices", Handler: _AdminService_ListConnectedDevices_Handler},
		{MethodName: "SendCommand", Handler: _AdminService_SendCommand_Handler},
		{MethodName: "StartRiverUI", Handler: _AdminService_StartRiverUI_Handler},
		{MethodName: "StopRiverUI", Handler: _AdminService_StopRiverUI_Handler},
		{MethodName: "GetRiverUIStatus", Handler: _AdminService_GetRiverUIStatus_Handler},
		{MethodName: "ListQuotaIncreaseRequests", Handler: _AdminService_ListQuotaIncreaseRequests_Handler},
		{MethodName: "ApproveQuotaIncrease", Handler: _AdminService_ApproveQuotaIncrease_Handler},
		{MethodName: "DenyQuotaIncrease", Handler: _AdminService_DenyQuotaIncrease_Handler},
		{MethodName: "GetDeploymentEvents", Handler: _AdminService_GetDeploymentEvents_Handler},
		{MethodName: "WakeUpDeployment", Handler: _AdminService_WakeUpDeployment_Handler},
		{MethodName: "RollbackDeployment", Handler: _AdminService_RollbackDeployment_Handler},
		{MethodName: "ReapplyDeployment", Handler: _AdminService_ReapplyDeployment_Handler},
		{MethodName: "GetDeploymentJobs", Handler: _AdminService_GetDeploymentJobs_Handler},
		{MethodName: "RepairNormalizedSpec", Handler: _AdminService_RepairNormalizedSpec_Handler},
		{MethodName: "RefreshDriftReport", Handler: _AdminService_RefreshDriftReport_Handler},
		{MethodName: "BackfillResolvedKeys", Handler: _AdminService_BackfillResolvedKeys_Handler},
		{MethodName: "SetAdapters", Handler: _AdminService_SetAdapters_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/admin/v1/admin.proto",
}
