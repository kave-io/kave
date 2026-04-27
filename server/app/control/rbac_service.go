package control

import (
	"context"
	"strings"
	"time"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/authctx"
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
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name required")
	}

	now := time.Now().UnixMilli()
	role := &control.Role{
		ID:          ids.New("role"),
		OrgID:       id.OrgID,
		Name:        req.Name,
		Permissions: req.Permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.appStore.InsertRole(ctx, role); err != nil {
		return nil, err
	}

	return roleToProto(role), nil
}

func (s *RBACServiceImpl) ListRoles(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListRolesResponse, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	result, err := s.appStore.ListRoles(ctx, id.OrgID, store.Page{Limit: 1000})
	if err != nil {
		return nil, err
	}

	resp := &controlv1.ListRolesResponse{}
	for _, role := range result.Items {
		resp.Roles = append(resp.Roles, roleToProto(role))
	}
	return resp, nil
}

func (s *RBACServiceImpl) GetRole(ctx context.Context, req *controlv1.GetRoleRequest) (*controlv1.Role, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	role, err := s.appStore.GetRole(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, status.Error(codes.NotFound, "role not found")
	}
	if role.OrgID != id.OrgID {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}

	return roleToProto(role), nil
}

func (s *RBACServiceImpl) DeleteRole(ctx context.Context, req *controlv1.DeleteRoleRequest) (*emptypb.Empty, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	role, err := s.appStore.GetRole(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, status.Error(codes.NotFound, "role not found")
	}
	if role.OrgID != id.OrgID {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}

	if err := s.appStore.DeleteRole(ctx, req.Id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *RBACServiceImpl) CreateBinding(ctx context.Context, req *controlv1.CreateBindingRequest) (*controlv1.Binding, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	if req.RoleId == "" || req.Subject == "" {
		return nil, status.Error(codes.InvalidArgument, "role_id and subject required")
	}

	now := time.Now().UnixMilli()
	binding := &control.Binding{
		ID:        ids.New("bnd"),
		OrgID:     id.OrgID,
		RoleID:    req.RoleId,
		Subject:   req.Subject,
		Scope:     req.Scope,
		CreatedAt: now,
	}

	if err := s.appStore.InsertBinding(ctx, binding); err != nil {
		return nil, err
	}

	return bindingToProto(binding), nil
}

func (s *RBACServiceImpl) ListBindings(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListBindingsResponse, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	result, err := s.appStore.ListBindings(ctx, id.OrgID, store.Page{Limit: 1000})
	if err != nil {
		return nil, err
	}

	resp := &controlv1.ListBindingsResponse{}
	for _, binding := range result.Items {
		resp.Bindings = append(resp.Bindings, bindingToProto(binding))
	}
	return resp, nil
}

func (s *RBACServiceImpl) DeleteBinding(ctx context.Context, req *controlv1.DeleteBindingRequest) (*emptypb.Empty, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	binding, err := s.appStore.GetBinding(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, status.Error(codes.NotFound, "binding not found")
	}
	if binding.OrgID != id.OrgID {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}

	if err := s.appStore.DeleteBinding(ctx, req.Id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *RBACServiceImpl) TestPermission(ctx context.Context, req *controlv1.TestPermissionRequest) (*controlv1.TestPermissionResponse, error) {
	id, ok := authctx.From(ctx)
	if !ok || id.OrgID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	subject := req.Subject
	if subject == "" {
		subject = "user:" + id.UserID
	}

	result, err := s.appStore.ListBindings(ctx, id.OrgID, store.Page{Limit: 10000})
	if err != nil {
		return nil, err
	}

	for _, binding := range result.Items {
		if binding.Subject != "*" && binding.Subject != subject {
			continue
		}

		role, err := s.appStore.GetRole(ctx, binding.RoleID)
		if err != nil || role == nil {
			continue
		}

		for _, perm := range role.Permissions {
			if perm == "*" || perm == "*:*" {
				return &controlv1.TestPermissionResponse{
					Allowed: true,
					Reason:  "wildcard permission",
				}, nil
			}

			parts := strings.Split(perm, ":")
			if len(parts) == 2 {
				resourceMatch := parts[0] == "*" || parts[0] == req.Resource
				actionMatch := parts[1] == "*" || parts[1] == req.Action
				if resourceMatch && actionMatch {
					return &controlv1.TestPermissionResponse{
						Allowed: true,
						Reason:  "matched permission: " + perm,
					}, nil
				}
			}
		}
	}

	return &controlv1.TestPermissionResponse{
		Allowed: false,
		Reason:  "no matching permission",
	}, nil
}
