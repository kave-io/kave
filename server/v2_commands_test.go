package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev2 "github.com/kave-io/kave/core/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

type v2BootstrapApplyFake struct {
	request corev2.ApplyRequest
	result  corev2.ApplyResult
	err     error
	calls   int
}

func (f *v2BootstrapApplyFake) Apply(_ context.Context, req corev2.ApplyRequest) (corev2.ApplyResult, error) {
	f.calls++
	f.request = req
	return f.result, f.err
}

type v2BootstrapKeysFake struct {
	issueRequest  v2postgres.IssueServiceKeyRequest
	issueResult   v2postgres.IssuedServiceKey
	issueErr      error
	issueCalls    int
	revokeRequest v2postgres.RevokeServiceKeyRequest
	revokeErr     error
	revokeCalls   int
}

func (f *v2BootstrapKeysFake) Issue(_ context.Context, req v2postgres.IssueServiceKeyRequest) (v2postgres.IssuedServiceKey, error) {
	f.issueCalls++
	f.issueRequest = req
	result := f.issueResult
	if result.ID != "" && result.Prefix == "" {
		result.Prefix = corev2.RawServiceKeyPrefix + req.LookupPrefix
	}
	return result, f.issueErr
}

func (f *v2BootstrapKeysFake) Revoke(_ context.Context, req v2postgres.RevokeServiceKeyRequest) (v2postgres.RevokeServiceKeyResult, error) {
	f.revokeCalls++
	f.revokeRequest = req
	return v2postgres.RevokeServiceKeyResult{ServiceKeyID: string(req.ServiceKeyID), Revoked: f.revokeErr == nil}, f.revokeErr
}

type failingV2BootstrapOutput struct {
	*os.File
	writeErr error
}

func (f *failingV2BootstrapOutput) Write([]byte) (int, error) { return 0, f.writeErr }

func TestLoadV2BootstrapConfigRequiresExplicitValidatedInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "initial-admin.key")
	values := map[string]string{
		v2BootstrapRuntimeDSNEnv:  "postgres:///kave?host=/var/run/postgresql",
		v2BootstrapRuntimeRoleEnv: "kave_v2_runtime",
		v2BootstrapAccountEnv:     "account/acme",
		v2BootstrapApplicationEnv: "simorq",
		v2BootstrapEnvironmentEnv: "production",
		v2BootstrapKeyNameEnv:     "initial-admin",
		v2BootstrapOutputEnv:      path,
	}
	getenv := func(name string) string { return values[name] }

	config, err := loadV2BootstrapConfig(getenv)
	if err != nil {
		t.Fatalf("loadV2BootstrapConfig() error = %v", err)
	}
	if config.Namespace.Account != "account/acme" || config.Namespace.Application != "simorq" || config.OutputPath != path {
		t.Fatalf("config = %+v", config)
	}

	for _, test := range []struct {
		name  string
		field string
		value string
	}{
		{name: "missing", field: v2BootstrapAccountEnv, value: ""},
		{name: "unsafe role", field: v2BootstrapRuntimeRoleEnv, value: "Kave Owner"},
		{name: "unsafe key name", field: v2BootstrapKeyNameEnv, value: "admin/key"},
		{name: "relative output", field: v2BootstrapOutputEnv, value: "admin.key"},
		{name: "unclean output", field: v2BootstrapOutputEnv, value: t.TempDir() + "/child/../admin.key"},
		{name: "whitespace", field: v2BootstrapEnvironmentEnv, value: " production"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prior := values[test.field]
			values[test.field] = test.value
			t.Cleanup(func() { values[test.field] = prior })
			if _, err := loadV2BootstrapConfig(getenv); err == nil {
				t.Fatal("loadV2BootstrapConfig() succeeded")
			}
		})
	}
}

func TestExecuteV2BootstrapWritesRawKeyOnceWithMode0600(t *testing.T) {
	config := testV2BootstrapConfig(t)
	apply := &v2BootstrapApplyFake{result: corev2.ApplyResult{NamespaceID: "namespace/prod", Revision: 1, Applied: true}}
	keys := &v2BootstrapKeysFake{issueResult: v2postgres.IssuedServiceKey{ID: "key/admin", Created: true}}

	result, err := executeV2Bootstrap(context.Background(), config, apply, keys, createExclusiveV2BootstrapOutput)
	if err != nil {
		t.Fatalf("executeV2Bootstrap() error = %v", err)
	}
	if result.NamespaceID != "namespace/prod" || result.ServiceKeyID != "key/admin" {
		t.Fatalf("result = %+v", result)
	}
	if apply.calls != 1 || keys.issueCalls != 1 || keys.revokeCalls != 0 {
		t.Fatalf("calls apply/issue/revoke = %d/%d/%d", apply.calls, keys.issueCalls, keys.revokeCalls)
	}
	if !apply.request.Caller.Bootstrap || apply.request.Caller.NamespaceID != "" || apply.request.Caller.ServiceKeyID != "offline-bootstrap" {
		t.Fatalf("offline caller = %+v", apply.request.Caller)
	}
	if apply.request.Prune || apply.request.DryRun || apply.request.Manifest.Namespace != config.Namespace {
		t.Fatalf("bootstrap apply = %+v", apply.request)
	}
	if keys.issueRequest.Scope != (v2postgres.Scope{AccountID: "account/acme", NamespaceID: "namespace/prod"}) {
		t.Fatalf("key scope = %+v", keys.issueRequest.Scope)
	}
	if keys.issueRequest.Name != "initial-admin" || len(keys.issueRequest.Operations) != 6 ||
		keys.issueRequest.Operations[0] != corev2.OperationConfigApply ||
		keys.issueRequest.Operations[2] != corev2.OperationKeysManage ||
		keys.issueRequest.Operations[5] != corev2.OperationAuditRead {
		t.Fatalf("issued key authority = %+v", keys.issueRequest)
	}
	if err := corev2.ValidateServiceKeyVerifier(keys.issueRequest.LookupPrefix, keys.issueRequest.SecretHash); err != nil {
		t.Fatalf("generated verifier = %v", err)
	}
	if apply.request.IdempotencyKey == "" || keys.issueRequest.IdempotencyKey == "" || apply.request.IdempotencyKey == keys.issueRequest.IdempotencyKey {
		t.Fatalf("idempotency keys apply/key = %q/%q", apply.request.IdempotencyKey, keys.issueRequest.IdempotencyKey)
	}

	content, err := os.ReadFile(config.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	rawKey := strings.TrimSuffix(string(content), "\n")
	material, err := corev2.ParseServiceKeyMaterial(rawKey)
	if err != nil || material.LookupPrefix != keys.issueRequest.LookupPrefix || !bytes.Equal(material.SecretHash[:], keys.issueRequest.SecretHash) {
		t.Fatalf("output material = %+v, %v; request=%+v", material, err, keys.issueRequest)
	}
	info, err := os.Stat(config.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("output mode = %v", info.Mode())
	}

	secondApply := &v2BootstrapApplyFake{result: apply.result}
	secondKeys := &v2BootstrapKeysFake{issueResult: keys.issueResult}
	if _, err := executeV2Bootstrap(context.Background(), config, secondApply, secondKeys, createExclusiveV2BootstrapOutput); err == nil || !errors.Is(err, os.ErrExist) {
		t.Fatalf("repeated execute error = %v", err)
	}
	if secondApply.calls != 0 || secondKeys.issueCalls != 0 {
		t.Fatalf("repeat changed database: apply/issue = %d/%d", secondApply.calls, secondKeys.issueCalls)
	}
}

func TestCreateExclusiveV2BootstrapOutputRejectsWritableDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	path := filepath.Join(directory, "admin.key")

	if _, err := createExclusiveV2BootstrapOutput(path); err == nil || !strings.Contains(err.Error(), "not group- or world-writable") {
		t.Fatalf("createExclusiveV2BootstrapOutput() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe directory output exists: %v", err)
	}
}

func TestExecuteV2BootstrapReplayKeepsLocallyGeneratedCredential(t *testing.T) {
	config := testV2BootstrapConfig(t)
	apply := &v2BootstrapApplyFake{result: corev2.ApplyResult{NamespaceID: "namespace/prod", Revision: 1, Applied: true}}
	keys := &v2BootstrapKeysFake{issueResult: v2postgres.IssuedServiceKey{ID: "key/admin", Created: false}}

	result, err := executeV2Bootstrap(context.Background(), config, apply, keys, createExclusiveV2BootstrapOutput)
	if err != nil || result.ServiceKeyID != "key/admin" {
		t.Fatalf("executeV2Bootstrap() = %+v, %v", result, err)
	}
	content, readErr := os.ReadFile(config.OutputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	material, parseErr := corev2.ParseServiceKeyMaterial(strings.TrimSuffix(string(content), "\n"))
	if parseErr != nil || material.LookupPrefix != keys.issueRequest.LookupPrefix {
		t.Fatalf("preserved replay material = %+v, %v", material, parseErr)
	}
}

func TestExecuteV2BootstrapPreservesMaterialOnInconsistentIssueMetadata(t *testing.T) {
	config := testV2BootstrapConfig(t)
	apply := &v2BootstrapApplyFake{result: corev2.ApplyResult{NamespaceID: "namespace/prod", Revision: 1, Applied: true}}
	keys := &v2BootstrapKeysFake{issueResult: v2postgres.IssuedServiceKey{Created: true}}

	_, err := executeV2Bootstrap(context.Background(), config, apply, keys, createExclusiveV2BootstrapOutput)
	if err == nil || !strings.Contains(err.Error(), "inconsistent metadata") {
		t.Fatalf("executeV2Bootstrap() error = %v", err)
	}
	if _, statErr := os.Stat(config.OutputPath); statErr != nil {
		t.Fatalf("generated material was not preserved: %v", statErr)
	}
}

func TestExecuteV2BootstrapDoesNotMutateDatabaseWhenMaterialWriteFails(t *testing.T) {
	config := testV2BootstrapConfig(t)
	apply := &v2BootstrapApplyFake{result: corev2.ApplyResult{NamespaceID: "namespace/prod", Revision: 1, Applied: true}}
	keys := &v2BootstrapKeysFake{issueResult: v2postgres.IssuedServiceKey{ID: "key/admin", Created: true}}
	writeErr := errors.New("disk rejected write")
	create := func(path string) (v2BootstrapOutput, error) {
		output, err := createExclusiveV2BootstrapOutput(path)
		if err != nil {
			return nil, err
		}
		return &failingV2BootstrapOutput{File: output.(*os.File), writeErr: writeErr}, nil
	}

	_, err := executeV2Bootstrap(context.Background(), config, apply, keys, create)
	if !errors.Is(err, writeErr) {
		t.Fatalf("executeV2Bootstrap() error = %v", err)
	}
	if apply.calls != 0 || keys.issueCalls != 0 || keys.revokeCalls != 0 {
		t.Fatalf("database calls apply/issue/revoke = %d/%d/%d", apply.calls, keys.issueCalls, keys.revokeCalls)
	}
	if _, statErr := os.Stat(config.OutputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed output remains before database mutation: %v", statErr)
	}
}

func TestExecuteV2BootstrapPreservesOutputWhenIssueOutcomeIsAmbiguous(t *testing.T) {
	config := testV2BootstrapConfig(t)
	apply := &v2BootstrapApplyFake{result: corev2.ApplyResult{NamespaceID: "namespace/prod", Revision: 1, Applied: true}}
	issueErr := errors.New("database response lost")
	keys := &v2BootstrapKeysFake{
		issueErr: issueErr,
	}

	_, err := executeV2Bootstrap(context.Background(), config, apply, keys, createExclusiveV2BootstrapOutput)
	if !errors.Is(err, issueErr) || !strings.Contains(err.Error(), "was preserved") {
		t.Fatalf("executeV2Bootstrap() error = %v", err)
	}
	info, statErr := os.Stat(config.OutputPath)
	if statErr != nil {
		t.Fatalf("preserved output stat = %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("preserved output mode = %v", info.Mode())
	}
	content, readErr := os.ReadFile(config.OutputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	material, parseErr := corev2.ParseServiceKeyMaterial(strings.TrimSuffix(string(content), "\n"))
	if parseErr != nil || material.LookupPrefix != keys.issueRequest.LookupPrefix || !bytes.Equal(material.SecretHash[:], keys.issueRequest.SecretHash) {
		t.Fatalf("ambiguous output/request material mismatch: %+v, %v / %+v", material, parseErr, keys.issueRequest)
	}
}

func testV2BootstrapConfig(t *testing.T) v2BootstrapConfig {
	t.Helper()
	return v2BootstrapConfig{
		RuntimeDSN:  "postgres:///kave?host=/var/run/postgresql",
		RuntimeRole: "kave_v2_runtime",
		Namespace: corev2.Namespace{
			Account: "account/acme", Application: "simorq", Environment: "production",
		},
		KeyName:    "initial-admin",
		OutputPath: filepath.Join(t.TempDir(), "initial-admin.key"),
	}
}
