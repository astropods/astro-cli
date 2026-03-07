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
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/admin/v1/admin.proto",
}
