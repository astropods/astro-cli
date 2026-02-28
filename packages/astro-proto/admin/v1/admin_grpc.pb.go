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
	ListImages(ctx context.Context, in *ListImagesRequest, opts ...grpc.CallOption) (*ListImagesResponse, error)
	DeleteDeployment(ctx context.Context, in *DeleteDeploymentRequest, opts ...grpc.CallOption) (*DeleteDeploymentResponse, error)
	RestartDeployment(ctx context.Context, in *RestartDeploymentRequest, opts ...grpc.CallOption) (*RestartDeploymentResponse, error)
	QueryDatabase(ctx context.Context, in *QueryDatabaseRequest, opts ...grpc.CallOption) (*QueryDatabaseResponse, error)
	GetSchema(ctx context.Context, in *GetSchemaRequest, opts ...grpc.CallOption) (*GetSchemaResponse, error)
	ListAccounts(ctx context.Context, in *ListAccountsRequest, opts ...grpc.CallOption) (*ListAccountsResponse, error)
	RenameAccount(ctx context.Context, in *RenameAccountRequest, opts ...grpc.CallOption) (*RenameAccountResponse, error)
	GetPodLogs(ctx context.Context, in *GetPodLogsRequest, opts ...grpc.CallOption) (*GetPodLogsResponse, error)
	GetPodEnv(ctx context.Context, in *GetPodEnvRequest, opts ...grpc.CallOption) (*GetPodEnvResponse, error)
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

func (c *adminServiceClient) ListImages(ctx context.Context, in *ListImagesRequest, opts ...grpc.CallOption) (*ListImagesResponse, error) {
	out := new(ListImagesResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/ListImages", in, out, opts...); err != nil {
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

func (c *adminServiceClient) QueryDatabase(ctx context.Context, in *QueryDatabaseRequest, opts ...grpc.CallOption) (*QueryDatabaseResponse, error) {
	out := new(QueryDatabaseResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/QueryDatabase", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetSchema(ctx context.Context, in *GetSchemaRequest, opts ...grpc.CallOption) (*GetSchemaResponse, error) {
	out := new(GetSchemaResponse)
	if err := c.cc.Invoke(ctx, "/admin.v1.AdminService/GetSchema", in, out, opts...); err != nil {
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

// AdminServiceServer is the server API for AdminService.
// Embed UnimplementedAdminServiceServer for forward compatibility.
type AdminServiceServer interface {
	ListDeployments(context.Context, *ListDeploymentsRequest) (*ListDeploymentsResponse, error)
	GetDeployment(context.Context, *GetDeploymentRequest) (*GetDeploymentResponse, error)
	GetClusterStatus(context.Context, *GetClusterStatusRequest) (*GetClusterStatusResponse, error)
	ListImages(context.Context, *ListImagesRequest) (*ListImagesResponse, error)
	DeleteDeployment(context.Context, *DeleteDeploymentRequest) (*DeleteDeploymentResponse, error)
	RestartDeployment(context.Context, *RestartDeploymentRequest) (*RestartDeploymentResponse, error)
	QueryDatabase(context.Context, *QueryDatabaseRequest) (*QueryDatabaseResponse, error)
	GetSchema(context.Context, *GetSchemaRequest) (*GetSchemaResponse, error)
	ListAccounts(context.Context, *ListAccountsRequest) (*ListAccountsResponse, error)
	RenameAccount(context.Context, *RenameAccountRequest) (*RenameAccountResponse, error)
	GetPodLogs(context.Context, *GetPodLogsRequest) (*GetPodLogsResponse, error)
	GetPodEnv(context.Context, *GetPodEnvRequest) (*GetPodEnvResponse, error)
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

func (UnimplementedAdminServiceServer) ListImages(context.Context, *ListImagesRequest) (*ListImagesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListImages not implemented")
}

func (UnimplementedAdminServiceServer) DeleteDeployment(context.Context, *DeleteDeploymentRequest) (*DeleteDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteDeployment not implemented")
}

func (UnimplementedAdminServiceServer) RestartDeployment(context.Context, *RestartDeploymentRequest) (*RestartDeploymentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestartDeployment not implemented")
}

func (UnimplementedAdminServiceServer) QueryDatabase(context.Context, *QueryDatabaseRequest) (*QueryDatabaseResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryDatabase not implemented")
}

func (UnimplementedAdminServiceServer) GetSchema(context.Context, *GetSchemaRequest) (*GetSchemaResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSchema not implemented")
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

func _AdminService_ListImages_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListImagesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).ListImages(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/ListImages"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).ListImages(ctx, req.(*ListImagesRequest))
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

func _AdminService_QueryDatabase_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryDatabaseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).QueryDatabase(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/QueryDatabase"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).QueryDatabase(ctx, req.(*QueryDatabaseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetSchema_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSchemaRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetSchema(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/admin.v1.AdminService/GetSchema"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetSchema(ctx, req.(*GetSchemaRequest))
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

// AdminService_ServiceDesc is the grpc.ServiceDesc for AdminService service.
var AdminService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "admin.v1.AdminService",
	HandlerType: (*AdminServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "ListDeployments", Handler: _AdminService_ListDeployments_Handler},
		{MethodName: "GetDeployment", Handler: _AdminService_GetDeployment_Handler},
		{MethodName: "GetClusterStatus", Handler: _AdminService_GetClusterStatus_Handler},
		{MethodName: "ListImages", Handler: _AdminService_ListImages_Handler},
		{MethodName: "DeleteDeployment", Handler: _AdminService_DeleteDeployment_Handler},
		{MethodName: "RestartDeployment", Handler: _AdminService_RestartDeployment_Handler},
		{MethodName: "QueryDatabase", Handler: _AdminService_QueryDatabase_Handler},
		{MethodName: "GetSchema", Handler: _AdminService_GetSchema_Handler},
		{MethodName: "ListAccounts", Handler: _AdminService_ListAccounts_Handler},
		{MethodName: "RenameAccount", Handler: _AdminService_RenameAccount_Handler},
		{MethodName: "GetPodLogs", Handler: _AdminService_GetPodLogs_Handler},
		{MethodName: "GetPodEnv", Handler: _AdminService_GetPodEnv_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/admin/v1/admin.proto",
}
