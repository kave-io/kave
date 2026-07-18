package service

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
)

func (s *Server) GetState(ctx context.Context, req *connect.Request[kernelv2.GetStateRequest]) (*connect.Response[kernelv2.State], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.reads == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("state reads unavailable"))
	}
	state, err := s.reads.GetState(ctx, corev2.GetStateRequest{
		Caller: caller, NamespaceID: corev2.Ref(req.Msg.GetNamespaceId()),
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	return connect.NewResponse(stateToProto(state)), nil
}

func (s *Server) GetLimitStatus(ctx context.Context, req *connect.Request[kernelv2.GetLimitStatusRequest]) (*connect.Response[kernelv2.GetLimitStatusResponse], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.reads == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("limit status unavailable"))
	}
	limits, err := s.reads.GetLimitStatus(ctx, corev2.GetLimitStatusRequest{
		Caller: caller, Scope: scopeFromProto(req.Msg.GetScope()), Agent: corev2.Ref(req.Msg.GetAgent()),
		Model: corev2.Ref(req.Msg.GetModel()), Metric: corev2.Metric(req.Msg.GetMetric()),
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	response := &kernelv2.GetLimitStatusResponse{Limits: make([]*kernelv2.LimitStatus, 0, len(limits))}
	for _, limit := range limits {
		item := &kernelv2.LimitStatus{
			LimitId: string(limit.LimitID), LimitKey: string(limit.LimitKey), Metric: string(limit.Metric),
			Used: limit.Used, Reserved: limit.Reserved, HardCap: limit.HardCap,
			ResetAtMs: limit.ResetAt.UnixMilli(),
		}
		if limit.SoftCap != nil {
			value := *limit.SoftCap
			item.SoftCap = &value
		}
		response.Limits = append(response.Limits, item)
	}
	return connect.NewResponse(response), nil
}

func (s *Server) QueryUsage(ctx context.Context, req *connect.Request[kernelv2.QueryUsageRequest]) (*connect.Response[kernelv2.QueryUsageResponse], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.reads == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("usage reporting unavailable"))
	}
	result, err := s.reads.QueryUsage(ctx, corev2.QueryUsageRequest{
		Caller: caller, Scope: scopeFromProto(req.Msg.GetScope()), Agent: corev2.Ref(req.Msg.GetAgent()),
		Metric: corev2.Metric(req.Msg.GetMetric()), Range: protoTimeRange(req.Msg.GetFromMs(), req.Msg.GetToMs()),
		Page: corev2.Page{Size: int(req.Msg.GetPageSize()), Token: req.Msg.GetPageToken()},
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	response := &kernelv2.QueryUsageResponse{
		Entries: make([]*kernelv2.UsageEntry, 0, len(result.Entries)), NextPageToken: result.NextPageToken,
	}
	for _, entry := range result.Entries {
		response.Entries = append(response.Entries, &kernelv2.UsageEntry{
			Id: string(entry.ID), InvocationId: string(entry.InvocationID), Metric: string(entry.Metric),
			Units: entry.Quantity, RequestCount: entry.RequestCount,
			InputTokens: entry.InputTokens, OutputTokens: entry.OutputTokens,
			CacheReadTokens: entry.CacheReadTokens, CacheWriteTokens: entry.CacheWriteTokens,
			ReasoningTokens: entry.ReasoningTokens, CostNanoUsd: entry.CostNanoUSD,
			Estimated: entry.Estimated,
			Provider:  string(entry.Provider), Model: string(entry.Model), Attempt: entry.Attempt,
			EventKind: string(entry.EventKind), CreatedAtMs: entry.CreatedAt.UnixMilli(),
		})
	}
	return connect.NewResponse(response), nil
}

func (s *Server) QueryInvocations(ctx context.Context, req *connect.Request[kernelv2.QueryInvocationsRequest]) (*connect.Response[kernelv2.QueryInvocationsResponse], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.reads == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("invocation reporting unavailable"))
	}
	status := corev2.DecisionStatus("")
	switch req.Msg.GetStatus() {
	case kernelv2.DecisionStatus_DECISION_STATUS_ADMITTED:
		status = corev2.DecisionAdmitted
	case kernelv2.DecisionStatus_DECISION_STATUS_REJECTED:
		status = corev2.DecisionRejected
	case kernelv2.DecisionStatus_DECISION_STATUS_UNSPECIFIED:
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid invocation decision status"))
	}
	result, err := s.reads.QueryInvocations(ctx, corev2.QueryInvocationsRequest{
		Caller: caller, Scope: scopeFromProto(req.Msg.GetScope()), Agent: corev2.Ref(req.Msg.GetAgent()), Status: status,
		Range: protoTimeRange(req.Msg.GetFromMs(), req.Msg.GetToMs()),
		Page:  corev2.Page{Size: int(req.Msg.GetPageSize()), Token: req.Msg.GetPageToken()},
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	response := &kernelv2.QueryInvocationsResponse{
		Invocations: make([]*kernelv2.Invocation, 0, len(result.Invocations)), NextPageToken: result.NextPageToken,
	}
	for _, invocation := range result.Invocations {
		item := &kernelv2.Invocation{
			Id: string(invocation.ID), Agent: string(invocation.Agent), Model: string(invocation.Model),
			Scope: scopeToProto(invocation.Scope), Decision: decisionStatusToProto(invocation.Decision),
			Status: string(invocation.Status), IdempotencyKey: string(invocation.IdempotencyKey),
			CreatedAtMs: invocation.CreatedAt.UnixMilli(),
		}
		if invocation.SettledAt != nil {
			item.SettledAtMs = invocation.SettledAt.UnixMilli()
		}
		response.Invocations = append(response.Invocations, item)
	}
	return connect.NewResponse(response), nil
}

func (s *Server) QueryAuditEvents(ctx context.Context, req *connect.Request[kernelv2.QueryAuditEventsRequest]) (*connect.Response[kernelv2.QueryAuditEventsResponse], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.reads == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("audit reporting unavailable"))
	}
	result, err := s.reads.QueryAuditEvents(ctx, corev2.QueryAuditEventsRequest{
		Caller: caller, EventKind: corev2.Ref(req.Msg.GetEventKind()),
		Range: protoTimeRange(req.Msg.GetFromMs(), req.Msg.GetToMs()),
		Page:  corev2.Page{Size: int(req.Msg.GetPageSize()), Token: req.Msg.GetPageToken()},
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	response := &kernelv2.QueryAuditEventsResponse{
		Events: make([]*kernelv2.AuditEvent, 0, len(result.Events)), NextPageToken: result.NextPageToken,
	}
	for _, event := range result.Events {
		response.Events = append(response.Events, &kernelv2.AuditEvent{
			Id: string(event.ID), EventKind: string(event.EventKind), ActorKind: string(event.ActorKind),
			ActorId: string(event.ActorID), ResourceKind: string(event.ResourceKind), ResourceId: string(event.ResourceID),
			Outcome: string(event.Outcome), Metadata: event.Metadata, CreatedAtMs: event.CreatedAt.UnixMilli(),
		})
	}
	return connect.NewResponse(response), nil
}

func protoTimeRange(fromMillis, toMillis int64) corev2.TimeRange {
	var from, to time.Time
	if fromMillis != 0 {
		from = time.UnixMilli(fromMillis).UTC()
	}
	if toMillis != 0 {
		to = time.UnixMilli(toMillis).UTC()
	}
	return corev2.TimeRange{From: from, To: to}
}

func stateToProto(state corev2.State) *kernelv2.State {
	return &kernelv2.State{
		NamespaceId: string(state.NamespaceID), Revision: state.Revision, Manifest: manifestToProto(state.Manifest),
	}
}

func manifestToProto(manifest corev2.Manifest) *kernelv2.Manifest {
	result := &kernelv2.Manifest{Namespace: &kernelv2.NamespaceSpec{
		Account: string(manifest.Namespace.Account), Application: string(manifest.Namespace.Application),
		Environment: string(manifest.Namespace.Environment),
	}}
	result.Routes = make([]*kernelv2.RouteSpec, 0, len(manifest.Routes))
	for _, route := range manifest.Routes {
		item := &kernelv2.RouteSpec{
			Name: string(route.Name), Provider: string(route.Provider), BaseUrl: route.BaseURL,
			Secret: string(route.Secret), AllowedModels: append([]string(nil), route.AllowedModels...),
			DefaultModel: route.DefaultModel, PricingRevision: route.PricingRevision,
			Pricing: make([]*kernelv2.ModelPrice, 0, len(route.Pricing)),
		}
		for _, price := range route.Pricing {
			item.Pricing = append(item.Pricing, &kernelv2.ModelPrice{
				Model: string(price.Model), InputNanosPerMillionTokens: price.InputNanosPerMillionTokens,
				OutputNanosPerMillionTokens: price.OutputNanosPerMillionTokens,
			})
		}
		result.Routes = append(result.Routes, item)
	}
	result.Agents = make([]*kernelv2.AgentSpec, 0, len(manifest.Agents))
	for _, agent := range manifest.Agents {
		kind := kernelv2.AgentKind_AGENT_KIND_UNSPECIFIED
		switch agent.Kind {
		case corev2.AgentLLM:
			kind = kernelv2.AgentKind_AGENT_KIND_LLM
		case corev2.AgentEmbedding:
			kind = kernelv2.AgentKind_AGENT_KIND_EMBEDDING
		}
		result.Agents = append(result.Agents, &kernelv2.AgentSpec{
			Name: string(agent.Name), Kind: kind, Route: string(agent.Route), Enabled: agent.Enabled,
		})
	}
	result.Limits = make([]*kernelv2.LimitSpec, 0, len(manifest.Limits))
	for _, limit := range manifest.Limits {
		window := kernelv2.LimitWindow_LIMIT_WINDOW_UNSPECIFIED
		switch limit.Window {
		case corev2.WindowAllTime:
			window = kernelv2.LimitWindow_LIMIT_WINDOW_ALL_TIME
		case corev2.WindowDay:
			window = kernelv2.LimitWindow_LIMIT_WINDOW_DAY
		case corev2.WindowMonth:
			window = kernelv2.LimitWindow_LIMIT_WINDOW_MONTH
		}
		item := &kernelv2.LimitSpec{
			Key: string(limit.Key), Metric: string(limit.Metric), Window: window, HardCap: limit.HardCap,
			Enabled: limit.Enabled, Selector: &kernelv2.LimitSelector{
				Tenant: string(limit.Selector.Tenant), Actor: string(limit.Selector.Actor), BillTo: string(limit.Selector.BillTo),
				Agent: string(limit.Selector.Agent), Model: string(limit.Selector.Model), Feature: string(limit.Selector.Feature),
			},
		}
		if limit.SoftCap != nil {
			value := *limit.SoftCap
			item.SoftCap = &value
		}
		result.Limits = append(result.Limits, item)
	}
	return result
}

func scopeToProto(scope corev2.Scope) *kernelv2.Scope {
	return &kernelv2.Scope{
		Tenant: string(scope.Tenant), Actor: string(scope.Actor), BillTo: string(scope.BillTo),
		Session: string(scope.Session), Feature: string(scope.Feature),
	}
}

func decisionStatusToProto(status corev2.DecisionStatus) kernelv2.DecisionStatus {
	switch status {
	case corev2.DecisionAdmitted:
		return kernelv2.DecisionStatus_DECISION_STATUS_ADMITTED
	case corev2.DecisionRejected:
		return kernelv2.DecisionStatus_DECISION_STATUS_REJECTED
	default:
		return kernelv2.DecisionStatus_DECISION_STATUS_UNSPECIFIED
	}
}
