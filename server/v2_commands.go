package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	corev2 "github.com/kave-io/kave/core/v2"
	v2kernel "github.com/kave-io/kave/server/internal/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

var v2RoleName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// runV2Migrate is deliberately a terminating command. The serving path never
// reads or retains schema-owner credentials. The login in the DSN must be able
// to SET ROLE to a dedicated NOLOGIN owner; runtime-role grants are converged
// while that owner is active.
func runV2Migrate() error {
	dsn := os.Getenv("KAVE_MIGRATION_POSTGRES_DSN")
	ownerRole := os.Getenv("KAVE_MIGRATION_OWNER_ROLE")
	runtimeRole := os.Getenv("KAVE_RUNTIME_POSTGRES_ROLE")
	if dsn == "" || ownerRole == "" || runtimeRole == "" {
		return errors.New("KAVE_MIGRATION_POSTGRES_DSN, KAVE_MIGRATION_OWNER_ROLE, and KAVE_RUNTIME_POSTGRES_ROLE are required")
	}
	if !v2RoleName.MatchString(ownerRole) || !v2RoleName.MatchString(runtimeRole) || ownerRole == runtimeRole {
		return errors.New("owner/runtime roles must be distinct lowercase PostgreSQL identifiers")
	}
	if err := v2kernel.ValidatePostgresDSN(dsn); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse migration DSN: %w", err)
	}
	role := pgx.Identifier{ownerRole}.Sanitize()
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, "SET ROLE "+role); err != nil {
			return fmt.Errorf("set dedicated V2 owner role: %w", err)
		}
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect migration database: %w", err)
	}
	if err := v2kernel.Prepare(ctx, pool, runtimeRole); err != nil {
		return err
	}
	fmt.Printf("Kave schema migrated; owner=%s runtime=%s\n", ownerRole, runtimeRole)
	return nil
}

const (
	v2BootstrapRuntimeDSNEnv  = "KAVE_RUNTIME_POSTGRES_DSN"
	v2BootstrapRuntimeRoleEnv = "KAVE_RUNTIME_POSTGRES_ROLE"
	v2BootstrapAccountEnv     = "KAVE_BOOTSTRAP_ACCOUNT"
	v2BootstrapApplicationEnv = "KAVE_BOOTSTRAP_APPLICATION"
	v2BootstrapEnvironmentEnv = "KAVE_BOOTSTRAP_ENVIRONMENT"
	v2BootstrapKeyNameEnv     = "KAVE_BOOTSTRAP_KEY_NAME"
	v2BootstrapOutputEnv      = "KAVE_BOOTSTRAP_OUTPUT"
)

type v2BootstrapConfig struct {
	RuntimeDSN  string
	RuntimeRole string
	Namespace   corev2.Namespace
	KeyName     corev2.Ref
	OutputPath  string
}

type v2BootstrapApplyStore interface {
	Apply(context.Context, corev2.ApplyRequest) (corev2.ApplyResult, error)
}

type v2BootstrapKeyAdmin interface {
	Issue(context.Context, v2postgres.IssueServiceKeyRequest) (v2postgres.IssuedServiceKey, error)
}

type v2BootstrapOutput interface {
	io.Writer
	Sync() error
	Close() error
}

type v2BootstrapResult struct {
	NamespaceID  corev2.Ref
	ServiceKeyID string
}

type v2BootstrapOutputFactory func(string) (v2BootstrapOutput, error)

// runV2Bootstrap is a one-shot, offline control-plane operation. It connects
// as the exact least-privilege runtime login, not as the migration owner, and
// never installs a standing bootstrap credential in an HTTP handler.
func runV2Bootstrap() error {
	config, err := loadV2BootstrapConfig(os.Getenv)
	if err != nil {
		return err
	}
	if err := v2kernel.ValidatePostgresDSN(config.RuntimeDSN); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := v2kernel.OpenRuntimePool(ctx, config.RuntimeDSN, config.RuntimeRole)
	if err != nil {
		return err
	}
	defer pool.Close()

	apply, err := v2postgres.NewApplyStore(pool)
	if err != nil {
		return fmt.Errorf("create V2 bootstrap apply store: %w", err)
	}
	keys, err := v2postgres.NewServiceKeyAdmin(pool)
	if err != nil {
		return fmt.Errorf("create V2 bootstrap key store: %w", err)
	}
	result, err := executeV2Bootstrap(ctx, config, apply, keys, createExclusiveV2BootstrapOutput)
	if err != nil {
		return err
	}

	fmt.Printf("Kave bootstrap complete; namespace=%s service_key=%s output=%s\n",
		result.NamespaceID, result.ServiceKeyID, config.OutputPath)
	return nil
}

func loadV2BootstrapConfig(getenv func(string) string) (v2BootstrapConfig, error) {
	if getenv == nil {
		return v2BootstrapConfig{}, errors.New("V2 bootstrap environment reader is unavailable")
	}
	required := []string{
		v2BootstrapRuntimeDSNEnv,
		v2BootstrapRuntimeRoleEnv,
		v2BootstrapAccountEnv,
		v2BootstrapApplicationEnv,
		v2BootstrapEnvironmentEnv,
		v2BootstrapKeyNameEnv,
		v2BootstrapOutputEnv,
	}
	values := make(map[string]string, len(required))
	for _, name := range required {
		value := getenv(name)
		if value == "" {
			return v2BootstrapConfig{}, fmt.Errorf("%s is required", name)
		}
		if strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 {
			return v2BootstrapConfig{}, fmt.Errorf("%s must not contain surrounding whitespace or NUL", name)
		}
		values[name] = value
	}

	config := v2BootstrapConfig{
		RuntimeDSN:  values[v2BootstrapRuntimeDSNEnv],
		RuntimeRole: values[v2BootstrapRuntimeRoleEnv],
		Namespace: corev2.Namespace{
			Account:     corev2.Ref(values[v2BootstrapAccountEnv]),
			Application: corev2.Ref(values[v2BootstrapApplicationEnv]),
			Environment: corev2.Ref(values[v2BootstrapEnvironmentEnv]),
		},
		KeyName:    corev2.Ref(values[v2BootstrapKeyNameEnv]),
		OutputPath: values[v2BootstrapOutputEnv],
	}
	if !v2RoleName.MatchString(config.RuntimeRole) {
		return v2BootstrapConfig{}, errors.New("KAVE_RUNTIME_POSTGRES_ROLE must be a lowercase PostgreSQL identifier")
	}
	if err := config.Namespace.Validate(); err != nil {
		return v2BootstrapConfig{}, fmt.Errorf("invalid V2 bootstrap namespace: %w", err)
	}
	if err := config.KeyName.ValidateName("bootstrap.service_key_name", true); err != nil {
		return v2BootstrapConfig{}, err
	}
	if !filepath.IsAbs(config.OutputPath) || filepath.Clean(config.OutputPath) != config.OutputPath {
		return v2BootstrapConfig{}, errors.New("KAVE_BOOTSTRAP_OUTPUT must be a clean absolute path")
	}
	if len(config.OutputPath) > 4096 || strings.ContainsAny(config.OutputPath, "\r\n") {
		return v2BootstrapConfig{}, errors.New("KAVE_BOOTSTRAP_OUTPUT must be at most 4096 bytes and contain no line breaks")
	}
	if filepath.Dir(config.OutputPath) == config.OutputPath {
		return v2BootstrapConfig{}, errors.New("KAVE_BOOTSTRAP_OUTPUT must name a file, not a filesystem root")
	}
	return config, nil
}

func executeV2Bootstrap(
	ctx context.Context,
	config v2BootstrapConfig,
	apply v2BootstrapApplyStore,
	keys v2BootstrapKeyAdmin,
	createOutput v2BootstrapOutputFactory,
) (v2BootstrapResult, error) {
	if apply == nil || keys == nil || createOutput == nil {
		return v2BootstrapResult{}, errors.New("V2 bootstrap dependencies are unavailable")
	}

	// Reserve the final pathname before changing the database. O_EXCL makes a
	// repeated invocation preserve a previously delivered credential rather
	// than truncating it and then discovering an idempotent database replay.
	output, err := createOutput(config.OutputPath)
	if err != nil {
		return v2BootstrapResult{}, err
	}
	outputOpen := true
	removeReservedOutput := func() error {
		var closeErr error
		if outputOpen {
			closeErr = output.Close()
			outputOpen = false
		}
		removeErr := os.Remove(config.OutputPath)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		var syncErr error
		if removeErr == nil {
			syncErr = syncV2BootstrapParent(config.OutputPath)
		}
		return errors.Join(closeErr, removeErr, syncErr)
	}

	// Persist recipient-generated raw material before any database mutation.
	// If the eventual commit response is lost, this file still contains the
	// exact credential whose verifier was submitted and an idempotent replay can
	// confirm it without asking the server to recover a secret.
	material, err := corev2.GenerateServiceKeyMaterial(nil)
	if err != nil {
		return v2BootstrapResult{}, errors.Join(err, removeReservedOutput())
	}
	raw := append([]byte(material.RawKey), '\n')
	material.RawKey = ""
	defer clear(raw)
	if written, writeErr := output.Write(raw); writeErr != nil || written != len(raw) {
		if writeErr == nil {
			writeErr = io.ErrShortWrite
		}
		return v2BootstrapResult{}, errors.Join(fmt.Errorf("write V2 bootstrap output: %w", writeErr), removeReservedOutput())
	}
	if err := output.Sync(); err != nil {
		return v2BootstrapResult{}, errors.Join(fmt.Errorf("sync V2 bootstrap output: %w", err), removeReservedOutput())
	}
	if err := output.Close(); err != nil {
		outputOpen = false
		return v2BootstrapResult{}, errors.Join(fmt.Errorf("close V2 bootstrap output: %w", err), removeReservedOutput())
	}
	outputOpen = false
	if err := syncV2BootstrapParent(config.OutputPath); err != nil {
		return v2BootstrapResult{}, errors.Join(fmt.Errorf("sync V2 bootstrap output directory: %w", err), removeReservedOutput())
	}

	caller := corev2.Caller{
		AccountID:    config.Namespace.Account,
		ServiceKeyID: "offline-bootstrap",
		Operations:   []corev2.Operation{corev2.OperationApply},
		Bootstrap:    true,
	}
	applyResult, err := apply.Apply(ctx, corev2.ApplyRequest{
		Caller:         caller,
		Manifest:       corev2.Manifest{Namespace: config.Namespace},
		IdempotencyKey: v2BootstrapIdempotencyKey("namespace", string(config.Namespace.Account), string(config.Namespace.Application), string(config.Namespace.Environment)),
	})
	if err != nil {
		return v2BootstrapResult{}, errors.Join(fmt.Errorf("bootstrap V2 namespace: %w", err), removeReservedOutput())
	}
	if applyResult.NamespaceID == "" || !applyResult.Applied {
		return v2BootstrapResult{}, errors.Join(errors.New("bootstrap V2 namespace returned an invalid result"), removeReservedOutput())
	}

	keyRequest := v2postgres.IssueServiceKeyRequest{
		Scope: v2postgres.Scope{
			AccountID:   string(config.Namespace.Account),
			NamespaceID: string(applyResult.NamespaceID),
		},
		IdempotencyKey: v2BootstrapIdempotencyKey("service-key", string(config.Namespace.Account), string(config.Namespace.Application), string(config.Namespace.Environment), string(config.KeyName)),
		Name:           string(config.KeyName),
		LookupPrefix:   material.LookupPrefix,
		SecretHash:     material.SecretHash[:],
		Operations: []corev2.Operation{
			corev2.OperationConfigApply,
			corev2.OperationSecretsWrite,
			corev2.OperationKeysManage,
			corev2.OperationLimitsSync,
			corev2.OperationUsageRead,
			corev2.OperationAuditRead,
		},
	}
	issued, err := keys.Issue(ctx, keyRequest)
	if err != nil {
		return v2BootstrapResult{}, fmt.Errorf("issue initial V2 admin service key: %w; generated credential was preserved at %q because commit outcome may be ambiguous", err, config.OutputPath)
	}
	if issued.ID == "" || issued.Prefix != corev2.RawServiceKeyPrefix+material.LookupPrefix {
		return v2BootstrapResult{}, fmt.Errorf("initial V2 admin service key returned inconsistent metadata; generated credential was preserved at %q", config.OutputPath)
	}
	return v2BootstrapResult{NamespaceID: applyResult.NamespaceID, ServiceKeyID: issued.ID}, nil
}

func createExclusiveV2BootstrapOutput(path string) (v2BootstrapOutput, error) {
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("inspect V2 bootstrap output directory %q: %w", filepath.Dir(path), err)
	}
	if !parentInfo.IsDir() || parentInfo.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("V2 bootstrap output directory %q must be a directory that is not group- or world-writable", filepath.Dir(path))
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("reserve V2 bootstrap output %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("secure V2 bootstrap output %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		_ = file.Close()
		_ = os.Remove(path)
		if err != nil {
			return nil, fmt.Errorf("inspect V2 bootstrap output %q: %w", path, err)
		}
		return nil, fmt.Errorf("V2 bootstrap output %q is not a regular 0600 file", path)
	}
	if err := syncV2BootstrapParent(path); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync V2 bootstrap output directory %q: %w", filepath.Dir(path), err)
	}
	return file, nil
}

func syncV2BootstrapParent(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func v2BootstrapIdempotencyKey(kind string, parts ...string) corev2.Ref {
	hash := sha256.New()
	_, _ = hash.Write([]byte("kave-v2-offline-bootstrap\x00" + kind))
	for _, part := range parts {
		_, _ = hash.Write([]byte{'\x00'})
		_, _ = hash.Write([]byte(part))
	}
	return corev2.Ref("bootstrap." + kind + "." + hex.EncodeToString(hash.Sum(nil)[:16]))
}
