// Package service adapts the transport-neutral V2 kernel to Connect. It does
// not authenticate raw keys or access Postgres directly.
package service

import (
	"context"
	"errors"
	"log/slog"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
	"github.com/kave-io/kave/proto/gen/kave/kernel/v2/kernelv2connect"
	v2authctx "github.com/kave-io/kave/server/internal/v2/authctx"
)

type Server struct {
	kernelv2connect.UnimplementedKernelServiceHandler
	admission *corev2.AdmissionService
	apply     *corev2.ApplyService
	keys      *corev2.ServiceKeyService
	secrets   *corev2.SecretService
	limits    *corev2.LimitSyncService
	reads     *corev2.ReadService
}

type Option func(*Server)

func WithApply(service *corev2.ApplyService) Option {
	return func(server *Server) { server.apply = service }
}

func WithServiceKeys(service *corev2.ServiceKeyService) Option {
	return func(server *Server) { server.keys = service }
}

func WithSecrets(service *corev2.SecretService) Option {
	return func(server *Server) { server.secrets = service }
}

func WithLimitSync(service *corev2.LimitSyncService) Option {
	return func(server *Server) { server.limits = service }
}

func WithReads(service *corev2.ReadService) Option {
	return func(server *Server) { server.reads = service }
}

func New(admission *corev2.AdmissionService, options ...Option) *Server {
	server := &Server{admission: admission}
	for _, option := range options {
		if option != nil {
			option(server)
		}
	}
	return server
}

func (s *Server) Consume(ctx context.Context, req *connect.Request[kernelv2.ConsumeRequest]) (*connect.Response[kernelv2.ConsumeResponse], error) {
	caller, ok := v2authctx.CallerFrom(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("service key required"))
	}
	if req == nil || req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("request required"))
	}

	consume := corev2.ConsumeRequest{
		Caller:         caller,
		Agent:          corev2.Ref(req.Msg.GetAgent()),
		Model:          corev2.Ref(req.Msg.GetModel()),
		Scope:          scopeFromProto(req.Msg.GetScope()),
		Metric:         corev2.Metric(req.Msg.GetMetric()),
		Units:          req.Msg.GetUnits(),
		IdempotencyKey: corev2.Ref(req.Msg.GetIdempotencyKey()),
	}
	if s == nil || s.admission == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("admission unavailable"))
	}
	decision, err := s.admission.Consume(ctx, consume)
	if err != nil {
		return nil, connectError(ctx, err, decision)
	}
	return connect.NewResponse(decisionToProto(decision)), nil
}

func connectError(ctx context.Context, err error, decision corev2.Decision) error {
	switch {
	case errors.Is(err, corev2.ErrInvalidArgument):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, corev2.ErrUnauthorized):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, corev2.ErrIdempotencyConflict):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, corev2.ErrServiceKeyMaterialConflict):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, corev2.ErrRevisionConflict):
		return connect.NewError(connect.CodeAborted, err)
	case errors.Is(err, corev2.ErrSecretNotFound), errors.Is(err, corev2.ErrServiceKeyNotFound),
		errors.Is(err, corev2.ErrNamespaceNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, corev2.ErrSecretEncryptionUnavailable),
		errors.Is(err, corev2.ErrSecretValidationUnavailable),
		errors.Is(err, corev2.ErrLimitOwnershipConflict):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, corev2.ErrLimitExceeded):
		connectErr := connect.NewError(connect.CodeResourceExhausted, corev2.ErrLimitExceeded)
		detail, detailErr := connect.NewErrorDetail(&kernelv2.LimitExceededDetail{
			InvocationId: decision.InvocationID,
			Violations:   violationsToProto(decision.Violations),
		})
		if detailErr == nil {
			connectErr.AddDetail(detail)
		}
		return connectErr
	default:
		slog.ErrorContext(ctx, "v2 kernel operation failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("kernel operation failed"))
	}
}

func scopeFromProto(scope *kernelv2.Scope) corev2.Scope {
	if scope == nil {
		return corev2.Scope{}
	}
	return corev2.Scope{
		Tenant:  corev2.Ref(scope.GetTenant()),
		Actor:   corev2.Ref(scope.GetActor()),
		BillTo:  corev2.Ref(scope.GetBillTo()),
		Session: corev2.Ref(scope.GetSession()),
		Feature: corev2.Ref(scope.GetFeature()),
	}
}

func decisionToProto(decision corev2.Decision) *kernelv2.ConsumeResponse {
	status := kernelv2.DecisionStatus_DECISION_STATUS_UNSPECIFIED
	switch decision.Status {
	case corev2.DecisionAdmitted:
		status = kernelv2.DecisionStatus_DECISION_STATUS_ADMITTED
	case corev2.DecisionRejected:
		status = kernelv2.DecisionStatus_DECISION_STATUS_REJECTED
	}
	warnings := make([]*kernelv2.LimitWarning, 0, len(decision.Warnings))
	for _, warning := range decision.Warnings {
		warnings = append(warnings, &kernelv2.LimitWarning{
			LimitId:   warning.LimitID,
			LimitKey:  string(warning.LimitKey),
			Used:      warning.Used,
			SoftCap:   warning.SoftCap,
			ResetAtMs: warning.ResetAt,
		})
	}
	return &kernelv2.ConsumeResponse{
		InvocationId: decision.InvocationID,
		Status:       status,
		Replayed:     decision.Replayed,
		Warnings:     warnings,
		Violations:   violationsToProto(decision.Violations),
	}
}

func violationsToProto(violations []corev2.Violation) []*kernelv2.LimitViolation {
	out := make([]*kernelv2.LimitViolation, 0, len(violations))
	for _, violation := range violations {
		out = append(out, &kernelv2.LimitViolation{
			LimitId:   violation.LimitID,
			LimitKey:  string(violation.LimitKey),
			Metric:    string(violation.Metric),
			Used:      violation.Used,
			Requested: violation.Requested,
			HardCap:   violation.HardCap,
			ResetAtMs: violation.ResetAt,
		})
	}
	return out
}

var _ kernelv2connect.KernelServiceHandler = (*Server)(nil)
