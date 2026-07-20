package service_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
	"github.com/kave-io/kave/server/internal/v2/authctx"
	"github.com/kave-io/kave/server/internal/v2/service"
)

type readStoreFake struct {
	usageRequest  corev2.QueryUsageRequest
	tenantRequest corev2.ListTenantsRequest
}

func (*readStoreFake) GetState(_ context.Context, req corev2.GetStateRequest) (corev2.State, error) {
	return corev2.State{
		NamespaceID: req.NamespaceID, Revision: 3,
		Manifest: corev2.Manifest{
			Namespace: corev2.Namespace{Account: req.Caller.AccountID, Application: "simorq", Environment: "prod"},
			Routes: []corev2.RouteSpec{{
				Name: "openai", Provider: "openai", BaseURL: "https://api.openai.com/v1", Secret: "provider-key",
				AllowedModels: []string{"gpt-safe"}, DefaultModel: "gpt-safe", PricingRevision: 2,
				Pricing: []corev2.ModelPrice{{
					Model: "gpt-safe", InputNanosPerMillionTokens: 1, OutputNanosPerMillionTokens: 4,
					CacheReadNanosPerMillionTokens: 2, CacheWriteNanosPerMillionTokens: 3,
					ReasoningNanosPerMillionTokens: 5,
				}},
			}},
		},
	}, nil
}

func (*readStoreFake) GetLimitStatus(context.Context, corev2.GetLimitStatusRequest) ([]corev2.LimitStatus, error) {
	soft := int64(8)
	return []corev2.LimitStatus{{LimitID: "lim_1", LimitKey: "monthly", Metric: "ai_actions", Used: 2, Reserved: 1, HardCap: 10, SoftCap: &soft, ResetAt: time.UnixMilli(1234)}}, nil
}

func (f *readStoreFake) QueryUsage(_ context.Context, req corev2.QueryUsageRequest) (corev2.QueryUsageResult, error) {
	f.usageRequest = req
	return corev2.QueryUsageResult{
		Entries: []corev2.UsageEntry{{
			ID: "use_1", InvocationID: "ivk_1", Metric: "input_tokens", Quantity: 7,
			RequestCount: 1, InputTokens: 7, OutputTokens: 2, CacheReadTokens: 3,
			CacheWriteTokens: 1, ReasoningTokens: 1, CostNanoUSD: 3, Estimated: true,
			Provider: "openai", Model: "gpt-safe", Attempt: 2,
			EventKind: "settlement", CreatedAt: time.UnixMilli(1500),
		}},
		NextPageToken: "opaque-next",
	}, nil
}

func (*readStoreFake) QueryInvocations(context.Context, corev2.QueryInvocationsRequest) (corev2.QueryInvocationsResult, error) {
	return corev2.QueryInvocationsResult{}, nil
}

func (f *readStoreFake) ListTenants(_ context.Context, req corev2.ListTenantsRequest) (corev2.ListTenantsResult, error) {
	f.tenantRequest = req
	lastSeen := time.UnixMilli(1700).UTC()
	return corev2.ListTenantsResult{
		Tenants: []corev2.TenantSummary{{
			Tenant: "clinic/opaque", BillTo: "clinic/opaque", Status: corev2.TenantStatusActive,
			LastSeenAt: &lastSeen, InvocationCount: 4, RequestCount: 3, CostNanoUSD: 42, ActiveLimits: 2,
		}},
		NextPageToken: "tenant-next",
	}, nil
}

func (*readStoreFake) QueryAuditEvents(context.Context, corev2.QueryAuditEventsRequest) (corev2.QueryAuditEventsResult, error) {
	return corev2.QueryAuditEventsResult{}, nil
}

func TestReadRPCsPreserveStatePricingAndUsageDimensions(t *testing.T) {
	t.Parallel()
	store := &readStoreFake{}
	server := service.New(nil, service.WithReads(corev2.NewReadService(store)))
	caller := corev2.Caller{
		AccountID: "account/acme", NamespaceID: "nsp_prod", ServiceKeyID: "key_admin",
		Operations: []corev2.Operation{corev2.OperationConfigApply, corev2.OperationUsageRead}, CanAssertScope: true,
	}
	ctx := authctx.WithCaller(context.Background(), caller)

	state, err := server.GetState(ctx, connect.NewRequest(&kernelv2.GetStateRequest{NamespaceId: "nsp_prod"}))
	if err != nil {
		t.Fatal(err)
	}
	if state.Msg.GetRevision() != 3 || len(state.Msg.GetManifest().GetRoutes()) != 1 ||
		state.Msg.GetManifest().GetRoutes()[0].GetPricing()[0].GetOutputNanosPerMillionTokens() != 4 ||
		state.Msg.GetManifest().GetRoutes()[0].GetPricing()[0].GetCacheReadNanosPerMillionTokens() != 2 ||
		state.Msg.GetManifest().GetRoutes()[0].GetPricing()[0].GetCacheWriteNanosPerMillionTokens() != 3 ||
		state.Msg.GetManifest().GetRoutes()[0].GetPricing()[0].GetReasoningNanosPerMillionTokens() != 5 {
		t.Fatalf("state = %+v", state.Msg)
	}

	usage, err := server.QueryUsage(ctx, connect.NewRequest(&kernelv2.QueryUsageRequest{
		Scope:  &kernelv2.Scope{Tenant: "clinic/opaque", BillTo: "clinic/opaque"},
		FromMs: 1000, ToMs: 2000, PageSize: 10,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(usage.Msg.GetEntries()) != 1 || usage.Msg.GetEntries()[0].GetCostNanoUsd() != 3 ||
		usage.Msg.GetEntries()[0].GetAttempt() != 2 || !usage.Msg.GetEntries()[0].GetEstimated() || usage.Msg.GetEntries()[0].GetInputTokens() != 7 ||
		usage.Msg.GetEntries()[0].GetCacheReadTokens() != 3 || usage.Msg.GetNextPageToken() != "opaque-next" {
		t.Fatalf("usage = %+v", usage.Msg)
	}
	if store.usageRequest.Scope.Tenant != "clinic/opaque" || store.usageRequest.Page.Size != 10 {
		t.Fatalf("captured usage request = %+v", store.usageRequest)
	}

	tenants, err := server.ListTenants(ctx, connect.NewRequest(&kernelv2.ListTenantsRequest{
		FromMs: 1000, ToMs: 2000, PageSize: 10, PageToken: "tenant-cursor",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(tenants.Msg.GetTenants()) != 1 || tenants.Msg.GetTenants()[0].GetTenant() != "clinic/opaque" ||
		tenants.Msg.GetTenants()[0].GetStatus() != "active" || tenants.Msg.GetTenants()[0].GetLastSeenAtMs() != 1700 ||
		tenants.Msg.GetTenants()[0].GetInvocationCount() != 4 || tenants.Msg.GetTenants()[0].GetRequestCount() != 3 ||
		tenants.Msg.GetTenants()[0].GetCostNanoUsd() != 42 || tenants.Msg.GetTenants()[0].GetActiveLimits() != 2 ||
		tenants.Msg.GetNextPageToken() != "tenant-next" {
		t.Fatalf("tenants = %+v", tenants.Msg)
	}
	if !store.tenantRequest.Range.From.Equal(time.UnixMilli(1000)) || !store.tenantRequest.Range.To.Equal(time.UnixMilli(2000)) ||
		store.tenantRequest.Page.Size != 10 || store.tenantRequest.Page.Token != "tenant-cursor" ||
		store.tenantRequest.Caller.NamespaceID != "nsp_prod" {
		t.Fatalf("captured tenant request = %+v", store.tenantRequest)
	}
}

func TestUsageRPCRejectsMissingTenantBoundary(t *testing.T) {
	t.Parallel()
	server := service.New(nil, service.WithReads(corev2.NewReadService(&readStoreFake{})))
	caller := corev2.Caller{
		AccountID: "account/acme", NamespaceID: "nsp_prod", ServiceKeyID: "key_admin",
		Operations: []corev2.Operation{corev2.OperationUsageRead}, CanAssertScope: true,
	}
	ctx := authctx.WithCaller(context.Background(), caller)
	_, err := server.QueryUsage(ctx, connect.NewRequest(&kernelv2.QueryUsageRequest{
		Scope: &kernelv2.Scope{BillTo: "clinic/opaque"}, FromMs: 1000, ToMs: 2000,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, err=%v", connect.CodeOf(err), err)
	}
}

func TestListTenantsRPCRequiresUsageRead(t *testing.T) {
	t.Parallel()
	store := &readStoreFake{}
	server := service.New(nil, service.WithReads(corev2.NewReadService(store)))
	caller := corev2.Caller{
		AccountID: "account/acme", NamespaceID: "nsp_prod", ServiceKeyID: "key_auditor",
		Operations: []corev2.Operation{corev2.OperationAuditRead},
	}
	ctx := authctx.WithCaller(context.Background(), caller)
	_, err := server.ListTenants(ctx, connect.NewRequest(&kernelv2.ListTenantsRequest{FromMs: 1000, ToMs: 2000}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("code = %v, err=%v", connect.CodeOf(err), err)
	}
	if store.tenantRequest.Caller.AccountID != "" {
		t.Fatalf("unauthorized request reached store: %+v", store.tenantRequest)
	}
}
