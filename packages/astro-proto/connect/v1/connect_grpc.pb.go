// Code generated manually to match proto/connect/v1/connect.proto.
// Bidirectional streaming service for device connections.

package connectv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConnectServiceClient is the client API for ConnectService.
type ConnectServiceClient interface {
	Connect(ctx context.Context, opts ...grpc.CallOption) (ConnectService_ConnectClient, error)
}

type connectServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewConnectServiceClient(cc grpc.ClientConnInterface) ConnectServiceClient {
	return &connectServiceClient{cc}
}

func (c *connectServiceClient) Connect(ctx context.Context, opts ...grpc.CallOption) (ConnectService_ConnectClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConnectService_ServiceDesc.Streams[0], "/connect.v1.ConnectService/Connect", opts...)
	if err != nil {
		return nil, err
	}
	return &connectServiceConnectClient{stream}, nil
}

// ConnectService_ConnectClient is the client-side bidi stream.
type ConnectService_ConnectClient interface {
	Send(*ClientMessage) error
	Recv() (*ServerMessage, error)
	grpc.ClientStream
}

type connectServiceConnectClient struct {
	grpc.ClientStream
}

func (x *connectServiceConnectClient) Send(m *ClientMessage) error {
	return x.ClientStream.SendMsg(m) //nolint:staticcheck
}

func (x *connectServiceConnectClient) Recv() (*ServerMessage, error) {
	m := new(ServerMessage)
	if err := x.ClientStream.RecvMsg(m); err != nil { //nolint:staticcheck
		return nil, err
	}
	return m, nil
}

// ConnectServiceServer is the server API for ConnectService.
type ConnectServiceServer interface {
	Connect(ConnectService_ConnectServer) error
	mustEmbedUnimplementedConnectServiceServer()
}

// UnimplementedConnectServiceServer should be embedded to have forward compatible implementations.
type UnimplementedConnectServiceServer struct{}

func (UnimplementedConnectServiceServer) Connect(ConnectService_ConnectServer) error {
	return status.Errorf(codes.Unimplemented, "method Connect not implemented")
}

func (UnimplementedConnectServiceServer) mustEmbedUnimplementedConnectServiceServer() {}

// UnsafeConnectServiceServer may be embedded to opt out of forward compatibility.
type UnsafeConnectServiceServer interface {
	mustEmbedUnimplementedConnectServiceServer()
}

// ConnectService_ConnectServer is the server-side bidi stream.
type ConnectService_ConnectServer interface {
	Send(*ServerMessage) error
	Recv() (*ClientMessage, error)
	grpc.ServerStream
}

type connectServiceConnectServer struct {
	grpc.ServerStream
}

func (x *connectServiceConnectServer) Send(m *ServerMessage) error {
	return x.ServerStream.SendMsg(m) //nolint:staticcheck
}

func (x *connectServiceConnectServer) Recv() (*ClientMessage, error) {
	m := new(ClientMessage)
	if err := x.ServerStream.RecvMsg(m); err != nil { //nolint:staticcheck
		return nil, err
	}
	return m, nil
}

// RegisterConnectServiceServer registers the server implementation.
func RegisterConnectServiceServer(s grpc.ServiceRegistrar, srv ConnectServiceServer) {
	s.RegisterService(&ConnectService_ServiceDesc, srv)
}

func _ConnectService_Connect_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConnectServiceServer).Connect(&connectServiceConnectServer{stream})
}

// ConnectService_ServiceDesc is the grpc.ServiceDesc for ConnectService.
var ConnectService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "connect.v1.ConnectService",
	HandlerType: (*ConnectServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Connect",
			Handler:       _ConnectService_Connect_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "proto/connect/v1/connect.proto",
}
