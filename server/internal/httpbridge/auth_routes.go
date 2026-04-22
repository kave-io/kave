package httpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/authhash"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/store"
	appcasbin "github.com/kave-io/kave/server/internal/infra/casbin"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authEnvelope struct {
	Token string `json:"token,omitempty"`
	User  any    `json:"user,omitempty"`
}

func registerAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			Email    string `json:"email"`
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Email == "" || req.Password == "" {
			return Outcome{Kind: "Auth"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		now := nowMS()
		org, _ := app.GetOrg(ctx, "default")
		if org == nil {
			_ = app.CreateOrg(ctx, &control.Organization{ID: "default", Name: "Default", Slug: "default", Plan: "free", CreatedAt: now, UpdatedAt: now})
		}
		user := &control.User{
			ID:           ids.New("usr"),
			OrgID:        "default",
			Email:        req.Email,
			Name:         req.Name,
			Status:       "active",
			PasswordHash: must(authhash.HashPassword(req.Password)),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := app.CreateUser(ctx, user); err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		token, hash, err := authhash.GenerateToken("ks_")
		if err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		if err := app.InsertSession(ctx, &control.Session{
			ID:        ids.New("ses"),
			OrgID:     user.OrgID,
			UserID:    user.ID,
			TokenHash: hash,
			ExpiresAt: now + 24*60*60*1000,
			CreatedAt: now,
		}); err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		if isFirstBootstrap(ctx, app, user.OrgID) {
			_ = app.InsertRole(ctx, &control.Role{
				ID:          "admin",
				OrgID:       user.OrgID,
				Name:        "admin",
				Permissions: []string{"*:*"},
				CreatedAt:   now,
				UpdatedAt:   now,
			})
			_ = app.InsertBinding(ctx, &control.Binding{
				ID:        ids.New("bnd"),
				OrgID:     user.OrgID,
				RoleID:    "admin",
				Subject:   "user:" + user.ID,
				Scope:     "*:*",
				CreatedAt: now,
			})
		}
		return Outcome{Kind: "Auth", Data: authEnvelope{Token: token, User: user}, Status: 201}, nil
	}
}

func loginAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "Auth"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		user, err := app.GetUserByEmail(ctx, "default", req.Email)
		if err != nil || user == nil {
			return Outcome{Kind: "Auth"}, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		if !authhash.VerifyPassword(user.PasswordHash, req.Password) {
			return Outcome{Kind: "Auth"}, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		token, hash, err := authhash.GenerateToken("ks_")
		if err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		now := nowMS()
		if err := app.InsertSession(ctx, &control.Session{
			ID:        ids.New("ses"),
			OrgID:     user.OrgID,
			UserID:    user.ID,
			TokenHash: hash,
			ExpiresAt: now + 24*60*60*1000,
			CreatedAt: now,
		}); err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		_ = app.UpdateUser(ctx, user.ID, &control.UserUpdate{LastLoginAt: &now})
		return Outcome{Kind: "Auth", Data: authEnvelope{Token: token, User: user}}, nil
	}
}

func logoutAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		if sess != nil {
			_ = app.RevokeSession(ctx, sess.ID, "system")
		}
		return Outcome{Kind: "Auth", Data: map[string]any{"status": "ok"}}, nil
	}
}

func whoamiAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Auth"}, status.Error(codes.Unauthenticated, "missing session")
		}
		user, _ := app.GetUser(ctx, sess.UserID)
		return Outcome{Kind: "Auth", Data: user}, nil
	}
}

func changePasswordAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "Auth"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Auth"}, status.Error(codes.Unauthenticated, "missing session")
		}
		user, _ := app.GetUser(ctx, sess.UserID)
		if user == nil || !authhash.VerifyPassword(user.PasswordHash, req.OldPassword) {
			return Outcome{Kind: "Auth"}, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		hash, err := authhash.HashPassword(req.NewPassword)
		if err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		if err := app.UpdateUser(ctx, user.ID, &control.UserUpdate{PasswordHash: &hash}); err != nil {
			return Outcome{Kind: "Auth"}, err
		}
		return Outcome{Kind: "Auth", Data: map[string]any{"status": "ok"}}, nil
	}
}

func listSessionsAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "SessionList"}, err
		}
		if sess == nil {
			return Outcome{Kind: "SessionList"}, status.Error(codes.Unauthenticated, "missing session")
		}
		resp, err := app.ListSessions(ctx, sess.UserID, store.Page{Limit: 100})
		if err != nil {
			return Outcome{Kind: "SessionList"}, err
		}
		return Outcome{Kind: "SessionList", Data: resp.Items}, nil
	}
}

func revokeSessionAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Session"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Session"}, status.Error(codes.Unauthenticated, "missing session")
		}
		_ = app.RevokeSession(ctx, path["id"], sess.UserID)
		return Outcome{Kind: "Session", Data: map[string]any{"status": "ok"}}, nil
	}
}

func createAPITokenAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "APIToken"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "APIToken"}, err
		}
		if sess == nil {
			return Outcome{Kind: "APIToken"}, status.Error(codes.Unauthenticated, "missing session")
		}
		token, hash, err := authhash.GenerateToken("kpat_")
		if err != nil {
			return Outcome{Kind: "APIToken"}, err
		}
		now := nowMS()
		apiTok := &control.APIToken{
			ID:        ids.New("pat"),
			OrgID:     sess.OrgID,
			UserID:    sess.UserID,
			Name:      req.Name,
			TokenHash: hash,
			Scopes:    req.Scopes,
			CreatedAt: now,
		}
		if err := app.InsertAPIToken(ctx, apiTok); err != nil {
			return Outcome{Kind: "APIToken"}, err
		}
		return Outcome{Kind: "APIToken", Data: authEnvelope{Token: token, User: apiTok}, Status: 201}, nil
	}
}

func listAPITokensAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "APITokenList"}, err
		}
		if sess == nil {
			return Outcome{Kind: "APITokenList"}, status.Error(codes.Unauthenticated, "missing session")
		}
		resp, err := app.ListAPITokens(ctx, sess.UserID, store.Page{Limit: 100})
		if err != nil {
			return Outcome{Kind: "APITokenList"}, err
		}
		return Outcome{Kind: "APITokenList", Data: resp.Items}, nil
	}
}

func revokeAPITokenAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "APIToken"}, err
		}
		if sess == nil {
			return Outcome{Kind: "APIToken"}, status.Error(codes.Unauthenticated, "missing session")
		}
		_ = app.RevokeAPIToken(ctx, path["id"], sess.UserID, "")
		return Outcome{Kind: "APIToken", Data: map[string]any{"status": "ok"}}, nil
	}
}

func createAgentTokenAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			AgentID string   `json:"agent_id"`
			Name    string   `json:"name"`
			Scopes  []string `json:"scopes"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "AgentToken"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "AgentToken"}, err
		}
		if sess == nil {
			return Outcome{Kind: "AgentToken"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if req.AgentID == "" {
			return Outcome{Kind: "AgentToken"}, status.Error(codes.InvalidArgument, "agent_id is required")
		}
		token, hash, err := authhash.GenerateToken("kat_")
		if err != nil {
			return Outcome{Kind: "AgentToken"}, err
		}
		now := nowMS()
		tok := &control.AgentToken{
			ID:        ids.New("agt"),
			OrgID:     sess.OrgID,
			AgentID:   req.AgentID,
			Name:      req.Name,
			TokenHash: hash,
			Scopes:    req.Scopes,
			CreatedAt: now,
		}
		if err := app.InsertAgentToken(ctx, tok); err != nil {
			return Outcome{Kind: "AgentToken"}, err
		}
		return Outcome{Kind: "AgentToken", Data: authEnvelope{Token: token, User: tok}, Status: 201}, nil
	}
}

func listAgentTokensAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		agentID := query.Get("agent_id")
		if agentID == "" {
			return Outcome{Kind: "AgentTokenList"}, status.Error(codes.InvalidArgument, "agent_id is required")
		}
		resp, err := app.ListAgentTokens(ctx, agentID, store.Page{Limit: 100})
		if err != nil {
			return Outcome{Kind: "AgentTokenList"}, err
		}
		return Outcome{Kind: "AgentTokenList", Data: resp.Items}, nil
	}
}

func revokeAgentTokenAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		_ = app.RevokeAgentToken(ctx, path["id"], "system", "")
		return Outcome{Kind: "AgentToken", Data: map[string]any{"status": "ok"}}, nil
	}
}

func createRoleAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Role"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Role"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if ok, err := canManageRBAC(ctx, app, sess); err != nil {
			return Outcome{Kind: "Role"}, err
		} else if !ok {
			return Outcome{Kind: "Role"}, status.Error(codes.PermissionDenied, "rbac access denied")
		}
		var role control.Role
		if err := json.Unmarshal(body, &role); err != nil {
			return Outcome{Kind: "Role"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		if role.ID == "" {
			role.ID = ids.New("role")
		}
		role.OrgID = sess.OrgID
		now := nowMS()
		role.CreatedAt = now
		role.UpdatedAt = now
		if err := app.InsertRole(ctx, &role); err != nil {
			return Outcome{Kind: "Role"}, err
		}
		return Outcome{Kind: "Role", Data: role, Status: 201}, nil
	}
}

func listRolesAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "RoleList"}, err
		}
		if sess == nil {
			return Outcome{Kind: "RoleList"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if ok, err := canManageRBAC(ctx, app, sess); err != nil {
			return Outcome{Kind: "RoleList"}, err
		} else if !ok {
			return Outcome{Kind: "RoleList"}, status.Error(codes.PermissionDenied, "rbac access denied")
		}
		resp, err := app.ListRoles(ctx, sess.OrgID, store.Page{Limit: 100})
		if err != nil {
			return Outcome{Kind: "RoleList"}, err
		}
		return Outcome{Kind: "RoleList", Data: resp.Items}, nil
	}
}

func deleteRoleAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Role"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Role"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if ok, err := canManageRBAC(ctx, app, sess); err != nil {
			return Outcome{Kind: "Role"}, err
		} else if !ok {
			return Outcome{Kind: "Role"}, status.Error(codes.PermissionDenied, "rbac access denied")
		}
		_ = app.DeleteRole(ctx, path["id"])
		return Outcome{Kind: "Role", Data: map[string]any{"status": "ok"}}, nil
	}
}

func createBindingAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Binding"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Binding"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if ok, err := canManageRBAC(ctx, app, sess); err != nil {
			return Outcome{Kind: "Binding"}, err
		} else if !ok {
			return Outcome{Kind: "Binding"}, status.Error(codes.PermissionDenied, "rbac access denied")
		}
		var binding control.Binding
		if err := json.Unmarshal(body, &binding); err != nil {
			return Outcome{Kind: "Binding"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		if binding.ID == "" {
			binding.ID = ids.New("bnd")
		}
		binding.OrgID = sess.OrgID
		binding.CreatedAt = nowMS()
		if err := app.InsertBinding(ctx, &binding); err != nil {
			return Outcome{Kind: "Binding"}, err
		}
		return Outcome{Kind: "Binding", Data: binding, Status: 201}, nil
	}
}

func listBindingsAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "BindingList"}, err
		}
		if sess == nil {
			return Outcome{Kind: "BindingList"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if ok, err := canManageRBAC(ctx, app, sess); err != nil {
			return Outcome{Kind: "BindingList"}, err
		} else if !ok {
			return Outcome{Kind: "BindingList"}, status.Error(codes.PermissionDenied, "rbac access denied")
		}
		resp, err := app.ListBindings(ctx, sess.OrgID, store.Page{Limit: 100})
		if err != nil {
			return Outcome{Kind: "BindingList"}, err
		}
		return Outcome{Kind: "BindingList", Data: resp.Items}, nil
	}
}

func deleteBindingAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Binding"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Binding"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if ok, err := canManageRBAC(ctx, app, sess); err != nil {
			return Outcome{Kind: "Binding"}, err
		} else if !ok {
			return Outcome{Kind: "Binding"}, status.Error(codes.PermissionDenied, "rbac access denied")
		}
		_ = app.DeleteBinding(ctx, path["id"])
		return Outcome{Kind: "Binding", Data: map[string]any{"status": "ok"}}, nil
	}
}

func testPermissionAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			Subject string `json:"subject"`
			Object  string `json:"object"`
			Action  string `json:"action"`
			Scope   string `json:"scope"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "Permission"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "Permission"}, err
		}
		if sess == nil {
			return Outcome{Kind: "Permission"}, status.Error(codes.Unauthenticated, "missing session")
		}
		if req.Subject == "" {
			req.Subject = "user:" + sess.UserID
		}
		allowed, err := permissionAllowed(ctx, app, sess.OrgID, req.Subject, req.Object, req.Action, req.Scope)
		if err != nil {
			return Outcome{Kind: "Permission"}, err
		}
		return Outcome{Kind: "Permission", Data: map[string]any{"allowed": allowed, "subject": req.Subject, "object": req.Object, "action": req.Action, "scope": req.Scope}}, nil
	}
}

func sessionFromRequest(ctx context.Context, app store.AppStore, tokenOverride *string) (*control.Session, error) {
	headers := requestHeaders(ctx)
	token := ""
	if tokenOverride != nil {
		token = *tokenOverride
	} else if headers != nil {
		auth := headers.Get("Authorization")
		token = strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			token = ""
		}
	}
	if token == "" {
		return nil, nil
	}
	sess, err := app.GetSessionByHash(ctx, string(authhash.HashToken(token)))
	if err != nil || sess == nil {
		return nil, err
	}
	if sess.RevokedAt != nil {
		return nil, status.Error(codes.Unauthenticated, "session revoked")
	}
	if sess.ExpiresAt > 0 && sess.ExpiresAt <= nowMS() {
		return nil, status.Error(codes.Unauthenticated, "session expired")
	}
	_ = app.TouchSession(ctx, sess.ID)
	return sess, nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func nowMS() int64 {
	return time.Now().UnixMilli()
}

func isFirstBootstrap(ctx context.Context, app store.AppStore, orgID string) bool {
	roles, err := app.ListRoles(ctx, orgID, store.Page{Limit: 1})
	if err != nil || len(roles.Items) > 0 {
		return false
	}
	bindings, err := app.ListBindings(ctx, orgID, store.Page{Limit: 1})
	if err != nil || len(bindings.Items) > 0 {
		return false
	}
	return true
}

func canManageRBAC(ctx context.Context, app store.AppStore, sess *control.Session) (bool, error) {
	return permissionAllowed(ctx, app, sess.OrgID, "user:"+sess.UserID, string(appcasbin.ResourceRBAC), string(appcasbin.ActionManage), "*:*")
}

func permissionAllowed(ctx context.Context, app store.AppStore, orgID, subject, object, action, scope string) (bool, error) {
	bindings, err := app.ListBindings(ctx, orgID, store.Page{Limit: 1000})
	if err != nil {
		return false, err
	}
	rolesByID := make(map[string]*control.Role, len(bindings.Items))
	for _, binding := range bindings.Items {
		if !subjectMatches(binding.Subject, subject) {
			continue
		}
		if !scopeMatches(binding.Scope, scope) {
			continue
		}
		role, ok := rolesByID[binding.RoleID]
		if !ok {
			role, err = app.GetRole(ctx, binding.RoleID)
			if err != nil {
				return false, err
			}
			if role == nil {
				continue
			}
			rolesByID[binding.RoleID] = role
		}
		if roleAllows(role, object, action) {
			return true, nil
		}
	}
	return false, nil
}

func subjectMatches(bindingSubject, subject string) bool {
	return bindingSubject == "*" || bindingSubject == subject
}

func scopeMatches(bindingScope, scope string) bool {
	return bindingScope == "" || bindingScope == "*" || bindingScope == scope || bindingScope == "*:*"
}

func roleAllows(role *control.Role, object, action string) bool {
	for _, perm := range role.Permissions {
		if perm == "*" || perm == "*:*" {
			return true
		}
		parts := strings.SplitN(perm, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if (parts[0] == "*" || parts[0] == object) && (parts[1] == "*" || parts[1] == action) {
			return true
		}
	}
	return false
}

// credentialTestAuth returns metadata about whether Resolve succeeds, never the secret.
func credentialTestAuth(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		sess, err := sessionFromRequest(ctx, app, nil)
		if err != nil {
			return Outcome{Kind: "CredentialTest"}, err
		}
		if sess == nil {
			return Outcome{Kind: "CredentialTest"}, status.Error(codes.Unauthenticated, "missing session")
		}
		_ = sess
		cred, err := app.GetCredential(ctx, path["id"])
		if err != nil {
			return Outcome{Kind: "CredentialTest"}, err
		}
		if cred == nil {
			return Outcome{Kind: "CredentialTest"}, status.Error(codes.NotFound, "credential not found")
		}
		start := time.Now()
		_, resolveErr := credresolve.Resolve(ctx, cred, credresolve.NoopVault{})
		latency := time.Since(start).Milliseconds()
		resp := map[string]any{
			"ok":         resolveErr == nil || errors.Is(resolveErr, credresolve.ErrPassthrough),
			"latency_ms": latency,
		}
		if resolveErr != nil && !errors.Is(resolveErr, credresolve.ErrPassthrough) {
			resp["error"] = resolveErr.Error()
		}
		return Outcome{Kind: "CredentialTest", Data: resp}, nil
	}
}
