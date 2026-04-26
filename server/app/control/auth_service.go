package control

import (
	"context"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/authhash"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/store"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AuthServiceImpl implements controlv1.AuthServiceServer.
type AuthServiceImpl struct {
	controlv1.UnimplementedAuthServiceServer

	appStore store.AppStore
	tokens   *serverauth.TokenManager
}

// NewAuthService creates a new AuthService server.
func NewAuthService(appStore store.AppStore, tokens *serverauth.TokenManager) *AuthServiceImpl {
	return &AuthServiceImpl{
		appStore: appStore,
		tokens:   tokens,
	}
}

// RegisterAuthService registers the AuthService with gRPC.
func (s *AuthServiceImpl) RegisterAuthService(srv *grpc.Server) {
	controlv1.RegisterAuthServiceServer(srv, s)
}

// Register handles user registration.
func (s *AuthServiceImpl) Register(ctx context.Context, req *controlv1.RegisterRequest) (*controlv1.AuthResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password required")
	}

	now := nowMS()
	// Ensure default org
	org, _ := s.appStore.GetOrg(ctx, "default")
	if org == nil {
		_ = s.appStore.CreateOrg(ctx, &control.Organization{
			ID:        "default",
			Name:      "Default",
			Slug:      "default",
			Plan:      "free",
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	// Create user
	hash, err := authhash.HashPassword(req.Password)
	if err != nil {
		return nil, status.Error(codes.Internal, "hashing failed")
	}

	user := &control.User{
		ID:           ids.New("usr"),
		OrgID:        "default",
		Email:        req.Email,
		Name:         req.Name,
		Status:       "active",
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.appStore.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	// Create session
	sessionID := ids.New("ses")
	var token string
	var tokenHash []byte

	if s.tokens != nil && s.tokens.Enabled() {
		sessionToken, sessErr := s.tokens.IssueSession(user.ID, sessionID, user.OrgID)
		if sessErr != nil {
			return nil, sessErr
		}
		token = sessionToken
		// Hash the token for storage
		hashBytes, _ := authhash.HashPassword(token)
		tokenHash = hashBytes
	} else {
		token = sessionID
		tokenHash, _ = authhash.HashPassword(token)
	}

	if err := s.appStore.InsertSession(ctx, &control.Session{
		ID:        sessionID,
		OrgID:     user.OrgID,
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: now + 24*60*60*1000,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	// Bootstrap RBAC
	if result, _ := s.appStore.ListRoles(ctx, user.OrgID, store.Page{Limit: 1}); result.Items == nil || len(result.Items) == 0 {
		_ = s.appStore.InsertRole(ctx, &control.Role{
			ID:          "admin",
			OrgID:       user.OrgID,
			Name:        "admin",
			Permissions: []string{"*:*"},
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		_ = s.appStore.InsertBinding(ctx, &control.Binding{
			ID:        ids.New("bnd"),
			OrgID:     user.OrgID,
			RoleID:    "admin",
			Subject:   "user:" + user.ID,
			Scope:     "*:*",
			CreatedAt: now,
		})
	}

	return &controlv1.AuthResponse{
		Token: token,
		User:  userToProto(user),
	}, nil
}

// Login handles user login.
func (s *AuthServiceImpl) Login(ctx context.Context, req *controlv1.LoginRequest) (*controlv1.AuthResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password required")
	}

	user, err := s.appStore.GetUserByEmail(ctx, "default", req.Email)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	if !authhash.VerifyPassword(user.PasswordHash, req.Password) {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	// Create session
	now := nowMS()
	sessionID := ids.New("ses")
	var token string
	var tokenHash []byte

	if s.tokens != nil && s.tokens.Enabled() {
		var err error
		token, err = s.tokens.IssueSession(user.ID, sessionID, user.OrgID)
		if err != nil {
			return nil, err
		}
		hashBytes, _ := authhash.HashPassword(token)
		tokenHash = hashBytes
	} else {
		token = sessionID
		tokenHash, _ = authhash.HashPassword(token)
	}

	if err := s.appStore.InsertSession(ctx, &control.Session{
		ID:        sessionID,
		OrgID:     user.OrgID,
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: now + 24*60*60*1000,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	_ = s.appStore.UpdateUser(ctx, user.ID, &control.UserUpdate{LastLoginAt: &now})

	return &controlv1.AuthResponse{
		Token: token,
		User:  userToProto(user),
	}, nil
}

// Logout revokes a session.
func (s *AuthServiceImpl) Logout(ctx context.Context, _ *controlv1.LogoutRequest) (*emptypb.Empty, error) {
	// Extract session from context; for now just return success
	return &emptypb.Empty{}, nil
}

// Whoami returns the authenticated user.
func (s *AuthServiceImpl) Whoami(ctx context.Context, _ *emptypb.Empty) (*controlv1.User, error) {
	// Extract user from context (set by interceptor)
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	user, err := s.appStore.GetUser(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return userToProto(user), nil
}

// ChangePassword changes the user's password.
func (s *AuthServiceImpl) ChangePassword(ctx context.Context, req *controlv1.ChangePasswordRequest) (*emptypb.Empty, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	user, err := s.appStore.GetUser(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	if !authhash.VerifyPassword(user.PasswordHash, req.OldPassword) {
		return nil, status.Error(codes.Unauthenticated, "invalid old password")
	}

	newHash, err := authhash.HashPassword(req.NewPassword)
	if err != nil {
		return nil, status.Error(codes.Internal, "hashing failed")
	}

	if err := s.appStore.UpdateUser(ctx, userID, &control.UserUpdate{PasswordHash: &newHash}); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// ListSessions lists sessions for the user.
func (s *AuthServiceImpl) ListSessions(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListSessionsResponse, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	result, err := s.appStore.ListSessions(ctx, userID, store.Page{Limit: 100})
	if err != nil {
		return nil, err
	}

	resp := &controlv1.ListSessionsResponse{}
	for _, sess := range result.Items {
		resp.Sessions = append(resp.Sessions, &controlv1.Session{
			Id:        sess.ID,
			UserId:    sess.UserID,
			CreatedAt: sess.CreatedAt,
			ExpiresAt: sess.ExpiresAt,
		})
	}
	return resp, nil
}

// RevokeSession revokes a session.
func (s *AuthServiceImpl) RevokeSession(ctx context.Context, req *controlv1.RevokeSessionRequest) (*emptypb.Empty, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id required")
	}

	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	if err := s.appStore.RevokeSession(ctx, req.SessionId, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// CreateAPIToken creates an API token.
func (s *AuthServiceImpl) CreateAPIToken(ctx context.Context, req *controlv1.CreateAPITokenRequest) (*controlv1.CreateAPITokenResponse, error) {
	// TODO: implement
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

// ListAPITokens lists API tokens.
func (s *AuthServiceImpl) ListAPITokens(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListAPITokensResponse, error) {
	// TODO: implement
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

// RevokeAPIToken revokes an API token.
func (s *AuthServiceImpl) RevokeAPIToken(ctx context.Context, req *controlv1.RevokeAPITokenRequest) (*emptypb.Empty, error) {
	// TODO: implement
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

// CreateAgentToken creates an agent token.
func (s *AuthServiceImpl) CreateAgentToken(ctx context.Context, req *controlv1.CreateAgentTokenRequest) (*controlv1.CreateAgentTokenResponse, error) {
	// TODO: implement
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

// ListAgentTokens lists agent tokens.
func (s *AuthServiceImpl) ListAgentTokens(ctx context.Context, _ *emptypb.Empty) (*controlv1.ListAgentTokensResponse, error) {
	// TODO: implement
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

// RevokeAgentToken revokes an agent token.
func (s *AuthServiceImpl) RevokeAgentToken(ctx context.Context, req *controlv1.RevokeAgentTokenRequest) (*emptypb.Empty, error) {
	// TODO: implement
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

// ── Mappers ────────────────────────────────────────────────────────────────

func userToProto(u *control.User) *controlv1.User {
	if u == nil {
		return nil
	}

	// Map status string to proto enum
	var status controlv1.UserStatus
	switch u.Status {
	case "active":
		status = controlv1.UserStatus_USER_STATUS_ACTIVE
	case "suspended":
		status = controlv1.UserStatus_USER_STATUS_SUSPENDED
	default:
		status = controlv1.UserStatus_USER_STATUS_UNSPECIFIED
	}

	return &controlv1.User{
		Id:          u.ID,
		OrgId:       u.OrgID,
		Email:       u.Email,
		Name:        u.Name,
		Status:      status,
		CreatedAtMs: u.CreatedAt,
		UpdatedAtMs: u.UpdatedAt,
	}
}
