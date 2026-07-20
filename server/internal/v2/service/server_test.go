package service_test

import (
	"context"
	"errors"
	"testing"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/core/v2/memory"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
	"github.com/kave-io/kave/server/internal/v2/authctx"
	"github.com/kave-io/kave/server/internal/v2/service"
)

func TestConsumeRequiresAuthenticatedCaller(t *testing.T) {
	t.Parallel()

	server := service.New(corev2.NewAdmissionService(memory.New()))
	_, err := server.Consume(context.Background(), connect.NewRequest(validRequest()))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated (err=%v)", connect.CodeOf(err), err)
	}
}

func TestConsumeMapsLimitExceededToStructuredDetail(t *testing.T) {
	t.Parallel()

	store := memory.New(corev2.BoundLimit{
		ID: "lim_zero", AccountID: "account/acme", NamespaceID: "namespace/prod",
		Spec: corev2.LimitSpec{Key: "zero", Metric: "ai_actions", Window: corev2.WindowMonth, HardCap: 0, Enabled: true},
	})
	server := service.New(corev2.NewAdmissionService(store))
	ctx := authctx.WithCaller(context.Background(), caller())
	_, err := server.Consume(ctx, connect.NewRequest(validRequest()))
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("code = %v, want resource exhausted (err=%v)", connect.CodeOf(err), err)
	}
	connectErr := new(connect.Error)
	if !errors.As(err, &connectErr) {
		t.Fatalf("error is not a Connect error: %T", err)
	}
	if len(connectErr.Details()) != 1 {
		t.Fatalf("details = %d, want 1", len(connectErr.Details()))
	}
	detail, valueErr := connectErr.Details()[0].Value()
	if valueErr != nil {
		t.Fatal(valueErr)
	}
	exceeded, ok := detail.(*kernelv2.LimitExceededDetail)
	if !ok || exceeded.GetInvocationId() == "" || len(exceeded.GetViolations()) != 1 {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestConsumeReturnsReplay(t *testing.T) {
	t.Parallel()

	server := service.New(corev2.NewAdmissionService(memory.New()))
	ctx := authctx.WithCaller(context.Background(), caller())
	first, err := server.Consume(ctx, connect.NewRequest(validRequest()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.Consume(ctx, connect.NewRequest(validRequest()))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Msg.GetReplayed() || second.Msg.GetInvocationId() != first.Msg.GetInvocationId() {
		t.Fatalf("unexpected replay: first=%+v second=%+v", first.Msg, second.Msg)
	}
}

func caller() corev2.Caller {
	return corev2.Caller{
		AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/worker",
		Operations: []corev2.Operation{corev2.OperationConsume}, AllowedAgents: []corev2.Ref{"clinic-assistant"}, CanAssertScope: true,
	}
}

func validRequest() *kernelv2.ConsumeRequest {
	return &kernelv2.ConsumeRequest{
		Agent:  "clinic-assistant",
		Scope:  &kernelv2.Scope{Tenant: "clinic/a", BillTo: "clinic/a", Session: "run/1", Feature: "ai_actions"},
		Metric: "ai_actions", Units: 1, IdempotencyKey: "run/1",
	}
}
