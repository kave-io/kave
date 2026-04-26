package control

import (
	"context"

	"github.com/kave-io/kave/core/store"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// RBACServiceImpl implements controlv1.RBACServiceServer.
type RBACServiceImpl struct {
	controlv1.UnimplementedRBACServiceServer

	appStore store.AppStore
}

// NewRBACService creates a new RBACService server.
func NewRBACService(appStore store.AppStore) *RBACServiceImpl {
	return &RBACServiceImpl{
		appStore: appStore,
	}
}

// RegisterRBACService registers the RBACService with gRPC.
func (s *RBACServiceImpl) RegisterRBACService(srv *grpc.Server) {
	controlv1.RegisterRBACServiceServer(srv, s)
}

func (s *RBACServiceImpl) CreateRole(ctx context.Context, req *controlv1.CreateRoleRequest) (*controlv1.Role, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) ListRoles(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListRolesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) GetRole(ctx context.Context, req *controlv1.GetRoleRequest) (*controlv1.Role, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) DeleteRole(ctx context.Context, req *controlv1.DeleteRoleRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) CreateBinding(ctx context.Context, req *controlv1.CreateBindingRequest) (*controlv1.Binding, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) ListBindings(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListBindingsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) DeleteBinding(ctx context.Context, req *controlv1.DeleteBindingRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *RBACServiceImpl) TestPermission(ctx context.Context, req *controlv1.TestPermissionRequest) (*controlv1.TestPermissionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}
