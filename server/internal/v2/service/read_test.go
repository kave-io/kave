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
	usageRequest corev2.QueryUsageRequest
}

func (*readStoreFake) GetState(_ context.Context, req corev2.GetStateRequest) (corev2.State, error) {
	return corev2.State{
		NamespaceID: req.NamespaceID, Revision: 3,
		Manifest: corev2.Manifest{
			Namespace: corev2.Namespace{Account: req.Caller.AccountID, Application: "simorq", Environment: "prod"},
			Routes: []corev2.RouteSpec{{
				Name: "openai", Provider: "openai", BaseURL: "https://api.openai.com/v1", Secret: "provider-key",
				AllowedModels: []string{"gpt-safe"}, DefaultModel: "gpt-safe", PricingRevision: 2,
				Pricing: []corev2.ModelPrice{{Model: "gpt-safe", InputNanosPerMillionTokens: 1, OutputNanosPerMillionTokens: 4}},
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
		state.Msg.GetManifest().GetRoutes()[0].GetPricing()[0].GetOutputNanosPerMillionTokens() != 4 {
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
