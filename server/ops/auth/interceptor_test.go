package auth_test

import (
	"context"
	"errors"
	"testing"

	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/server/internal/authctx"
	appcasbin "github.com/kave-io/kave/server/internal/infra/casbin"
	serverauth "github.com/kave-io/kave/server/ops/auth"
)

func newAction() *coreruntime.Action {
	a := &coreruntime.Action{}
	a.Connector = "openai"
	a.Method = "chat.completions"
	return a
}

func TestInterceptor_Anonymous_Disallowed(t *testing.T) {
	i := serverauth.NewInterceptor(nil, false, false)
	_, err := i.Before(context.Background(), newAction())
	if !errors.Is(err, serverauth.ErrUnauthenticated) {
		t.Fatalf("want ErrUnauthenticated, got %v", err)
	}
}

func TestInterceptor_Anonymous_Allowed(t *testing.T) {
	i := serverauth.NewInterceptor(nil, true, false)
	got, err := i.Before(context.Background(), newAction())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil {
		t.Fatal("expected action")
	}
}

func TestInterceptor_Invalid_AlwaysRejected(t *testing.T) {
	i := serverauth.NewInterceptor(nil, true, true)
	ctx := authctx.With(context.Background(), authctx.Identity{Kind: authctx.KindInvalid})
	_, err := i.Before(ctx, newAction())
	if !errors.Is(err, serverauth.ErrUnauthenticated) {
		t.Fatalf("want ErrUnauthenticated, got %v", err)
	}
}

func TestInterceptor_AgentToken_DecoratesAction(t *testing.T) {
	i := serverauth.NewInterceptor(nil, false, false)
	id := authctx.Identity{
		Kind:      authctx.KindAgent,
		AgentID:   "agn_123",
		ProjectID: "prj_1",
		EnvID:     "env_dev",
	}
	ctx := authctx.With(context.Background(), id)
	got, err := i.Before(ctx, newAction())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.AgentID != "agn_123" || got.ProjectID != "prj_1" || got.EnvID != "env_dev" {
		t.Fatalf("decoration missing: %+v", got)
	}
}

func TestInterceptor_AgentToken_DoesNotOverwrite(t *testing.T) {
	i := serverauth.NewInterceptor(nil, false, false)
	id := authctx.Identity{Kind: authctx.KindAgent, AgentID: "agn_token", ProjectID: "prj_token"}
	ctx := authctx.With(context.Background(), id)
	a := newAction()
	a.AgentID = "agn_explicit"
	a.ProjectID = "prj_explicit"
	got, _ := i.Before(ctx, a)
	if got.AgentID != "agn_explicit" || got.ProjectID != "prj_explicit" {
		t.Fatalf("must not overwrite explicit values: %+v", got)
	}
}

func TestInterceptor_User_NilCasbin_PassThrough(t *testing.T) {
	i := serverauth.NewInterceptor(nil, false, false)
	ctx := authctx.With(context.Background(), authctx.Identity{Kind: authctx.KindUser, UserID: "u1"})
	if _, err := i.Before(ctx, newAction()); err != nil {
		t.Fatalf("nil casbin should pass-through, got %v", err)
	}
}

func TestInterceptor_User_CasbinDeny(t *testing.T) {
	eng, err := appcasbin.NewEnforcer(appcasbin.Config{})
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}
	i := serverauth.NewInterceptor(eng, false, false)
	ctx := authctx.With(context.Background(), authctx.Identity{Kind: authctx.KindUser, UserID: "u1", OrgID: "org1"})
	_, err = i.Before(ctx, newAction())
	if err == nil {
		t.Fatal("want unauthorized, got nil")
	}
	if !errors.Is(err, serverauth.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestInterceptor_User_CasbinAllow(t *testing.T) {
	eng, err := appcasbin.NewEnforcer(appcasbin.Config{})
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}
	// Seed a permissive policy on the raw enforcer: user u1 acts as role admin in org1; admin can do everything.
	raw := eng.Raw()
	if _, err := raw.AddGroupingPolicy("user:u1", "admin", "org1"); err != nil {
		t.Fatalf("g: %v", err)
	}
	if _, err := raw.AddPolicy("admin", "org1", "openai.chat.completions", "chat.completions"); err != nil {
		t.Fatalf("p: %v", err)
	}

	i := serverauth.NewInterceptor(eng, false, false)
	ctx := authctx.With(context.Background(), authctx.Identity{Kind: authctx.KindUser, UserID: "u1", OrgID: "org1"})
	if _, err := i.Before(ctx, newAction()); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
}

func TestInterceptor_NilAction(t *testing.T) {
	i := serverauth.NewInterceptor(nil, false, false)
	got, err := i.Before(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("nil action passthrough: got=%v err=%v", got, err)
	}
}
