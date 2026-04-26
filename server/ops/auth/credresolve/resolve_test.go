package credresolve_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
)

func TestResolve_Env_Found(t *testing.T) {
	t.Setenv("KAVE_TEST_KEY", "secret-value")
	cred := &controlmodel.ConnectorCredential{ID: "c1", Source: controlmodel.CredentialSourceEnv, EnvVar: "KAVE_TEST_KEY"}
	got, err := credresolve.Resolve(context.Background(), cred, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "secret-value" {
		t.Fatalf("got %q", got)
	}
}

func TestResolve_Env_Unset(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{ID: "c1", Source: controlmodel.CredentialSourceEnv, EnvVar: "KAVE_TEST_UNSET_XYZ"}
	if _, err := credresolve.Resolve(context.Background(), cred, nil); err == nil {
		t.Fatal("expected err for unset env")
	}
}

func TestResolve_Env_MissingVarName(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{ID: "c1", Source: controlmodel.CredentialSourceEnv}
	if _, err := credresolve.Resolve(context.Background(), cred, nil); err == nil {
		t.Fatal("expected err for missing env var name")
	}
}

func TestResolve_Passthrough_Sentinel(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{ID: "c1", Source: controlmodel.CredentialSourcePassthrough}
	_, err := credresolve.Resolve(context.Background(), cred, nil)
	if !errors.Is(err, credresolve.ErrPassthrough) {
		t.Fatalf("want ErrPassthrough, got %v", err)
	}
}

func TestResolve_Vault_NoClient_Disabled(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{ID: "c1", Source: controlmodel.CredentialSourceVault, VaultRef: "x/y"}
	_, err := credresolve.Resolve(context.Background(), cred, nil)
	if !errors.Is(err, credresolve.ErrSourceDisabled) {
		t.Fatalf("want ErrSourceDisabled, got %v", err)
	}
}

func TestResolve_NilCredential(t *testing.T) {
	if _, err := credresolve.Resolve(context.Background(), nil, nil); err == nil {
		t.Fatal("expected err")
	}
}

func TestResolve_NoSource(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{ID: "c1"}
	if _, err := credresolve.Resolve(context.Background(), cred, nil); err == nil {
		t.Fatal("expected err for missing source")
	}
}

func TestResolve_UnsupportedSource(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{ID: "c1", Source: "fictional"}
	if _, err := credresolve.Resolve(context.Background(), cred, nil); err == nil {
		t.Fatal("expected err")
	}
}

// ── VaultResolver ────────────────────────────────────────────────────────────

func TestVaultResolver_KVv2Response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "root-token" {
			http.Error(w, `{"errors":["forbidden"]}`, http.StatusForbidden)
			return
		}
		if r.URL.Path != "/v1/secret/data/kave/myapp" {
			http.Error(w, `{"errors":["not found"]}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"top-secret"}}}`))
	}))
	defer srv.Close()

	v := &credresolve.VaultResolver{Addr: srv.URL, Token: "root-token", Mount: "secret/data/kave"}
	got, err := v.Resolve(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "top-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestVaultResolver_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"errors":["denied"]}`, http.StatusForbidden)
	}))
	defer srv.Close()

	v := &credresolve.VaultResolver{Addr: srv.URL, Token: "x", Mount: "secret"}
	if _, err := v.Resolve(context.Background(), "x"); err == nil {
		t.Fatal("expected err")
	}
}

func TestVaultResolver_MissingValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"data":{}}}`))
	}))
	defer srv.Close()

	v := &credresolve.VaultResolver{Addr: srv.URL, Token: "x", Mount: "secret"}
	if _, err := v.Resolve(context.Background(), "x"); err == nil {
		t.Fatal("expected err for missing value")
	}
}

func TestVaultResolver_DisabledWhenUnconfigured(t *testing.T) {
	v := &credresolve.VaultResolver{}
	if _, err := v.Resolve(context.Background(), "x"); !errors.Is(err, credresolve.ErrSourceDisabled) {
		t.Fatalf("want ErrSourceDisabled, got %v", err)
	}
}

func TestVaultResolver_NilReceiver(t *testing.T) {
	var v *credresolve.VaultResolver
	if _, err := v.Resolve(context.Background(), "x"); !errors.Is(err, credresolve.ErrSourceDisabled) {
		t.Fatalf("want ErrSourceDisabled, got %v", err)
	}
}
