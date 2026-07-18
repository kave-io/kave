// Package v2 assembles the compact V2 kernel on Kave's existing HTTP listener.
package v2

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/proto/gen/kave/kernel/v2/kernelv2connect"
	v2gateway "github.com/kave-io/kave/server/internal/v2/gateway"
	"github.com/kave-io/kave/server/internal/v2/httpapi"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	v2service "github.com/kave-io/kave/server/internal/v2/service"
)

type Config struct {
	MasterKey                       string
	MasterDecryptionKeys            []string
	SecretIdempotencyKey            string
	RuntimeRole                     string
	ProviderEgressAllowedPrivateIPs []string
}

// Prepare applies migrations through a privileged, transient connection and
// grants the configured runtime role only the kernel DML it needs.
func Prepare(ctx context.Context, migrationPool *pgxpool.Pool, runtimeRole string) error {
	migrator, err := v2postgres.NewMigrator(migrationPool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create migrator: %w", err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("v2 kernel: migrate: %w", err)
	}
	if err := migrator.GrantRuntime(ctx, runtimeRole); err != nil {
		return fmt.Errorf("v2 kernel: grant runtime role: %w", err)
	}
	return nil
}

// Register mounts the authenticated Connect kernel on an already migrated,
// verified non-privileged runtime pool. Every unimplemented RPC fails closed.
func Register(ctx context.Context, mux *http.ServeMux, pool *pgxpool.Pool, cfg Config) error {
	if mux == nil || pool == nil {
		return fmt.Errorf("v2 kernel: HTTP mux and Postgres pool are required")
	}
	if err := v2postgres.VerifyRuntimeRole(ctx, pool, cfg.RuntimeRole); err != nil {
		return fmt.Errorf("v2 kernel: unsafe runtime database role: %w", err)
	}
	admissionStore, err := v2postgres.NewAdmissionStore(pool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create admission store: %w", err)
	}
	serviceKeyAuth, err := v2postgres.NewServiceKeyAuthenticator(pool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create service-key authenticator: %w", err)
	}
	auth := httpapi.NewDatabaseAuthenticator(serviceKeyAuth)
	applyStore, err := v2postgres.NewApplyStore(pool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create apply store: %w", err)
	}
	readStore, err := v2postgres.NewReadStore(pool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create read store: %w", err)
	}
	limitSyncStore, err := v2postgres.NewLimitSyncStore(pool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create limit synchronization store: %w", err)
	}
	serviceKeyAdmin, err := v2postgres.NewServiceKeyAdmin(pool)
	if err != nil {
		return fmt.Errorf("v2 kernel: create service-key admin: %w", err)
	}
	var secretCipher v2postgres.SecretCipher
	if cfg.MasterKey != "" {
		secretCipher, err = v2postgres.NewLocalEnvelopeCipherKeyring(v2postgres.LocalEnvelopeKeyring{
			CurrentKey: cfg.MasterKey, DecryptionKeys: cfg.MasterDecryptionKeys,
			IdempotencyKey: cfg.SecretIdempotencyKey,
		})
		if err != nil {
			return fmt.Errorf("v2 kernel: configure secret encryption: %w", err)
		}
	}
	secretStore, err := v2postgres.NewSecretStore(pool, secretCipher)
	if err != nil {
		return fmt.Errorf("v2 kernel: create secret store: %w", err)
	}
	providerStore, err := v2postgres.NewProviderStore(pool, secretCipher)
	if err != nil {
		return fmt.Errorf("v2 kernel: create provider store: %w", err)
	}
	providerGateway, err := v2gateway.New(auth, providerStore, v2gateway.ProviderEgressPolicy{
		AllowedPrivateIPs: cfg.ProviderEgressAllowedPrivateIPs,
	}, nil)
	if err != nil {
		return fmt.Errorf("v2 kernel: create provider gateway: %w", err)
	}

	handler := v2service.New(
		corev2.NewAdmissionService(admissionStore),
		v2service.WithApply(corev2.NewApplyService(applyStore)),
		v2service.WithReads(corev2.NewReadService(readStore)),
		v2service.WithLimitSync(corev2.NewLimitSyncService(limitSyncStore)),
		v2service.WithServiceKeys(corev2.NewServiceKeyService(serviceKeyStoreAdapter{admin: serviceKeyAdmin})),
		v2service.WithSecrets(corev2.NewSecretService(secretStore)),
	)
	path, connectHandler := kernelv2connect.NewKernelServiceHandler(handler)
	mux.Handle(path, httpapi.NewAuthMiddleware(auth).WrapConnect(connectHandler))
	if err := v2gateway.Register(mux, providerGateway); err != nil {
		return fmt.Errorf("v2 kernel: register provider gateway: %w", err)
	}
	return nil
}
