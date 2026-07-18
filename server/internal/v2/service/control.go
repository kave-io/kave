package service

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
	v2authctx "github.com/kave-io/kave/server/internal/v2/authctx"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) Apply(ctx context.Context, req *connect.Request[kernelv2.ApplyRequest]) (*connect.Response[kernelv2.ApplyResponse], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.apply == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("apply unavailable"))
	}
	manifest, err := manifestFromProto(req.Msg.GetManifest())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := s.apply.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: manifest, DryRun: req.Msg.GetDryRun(), Prune: req.Msg.GetPrune(),
		ExpectedRevision: req.Msg.GetExpectedRevision(), IdempotencyKey: corev2.Ref(req.Msg.GetIdempotencyKey()),
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	return connect.NewResponse(applyResultToProto(result)), nil
}

func (s *Server) IssueServiceKey(ctx context.Context, req *connect.Request[kernelv2.IssueServiceKeyRequest]) (*connect.Response[kernelv2.IssuedServiceKey], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.keys == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("service-key administration unavailable"))
	}
	operations := make([]corev2.Operation, len(req.Msg.GetOperations()))
	for i, operation := range req.Msg.GetOperations() {
		operations[i] = corev2.Operation(operation)
	}
	agents := make([]corev2.Ref, len(req.Msg.GetAllowedAgents()))
	for i, agent := range req.Msg.GetAllowedAgents() {
		agents[i] = corev2.Ref(agent)
	}
	var expiresAt *time.Time
	if req.Msg.GetExpiresAtMs() != 0 {
		value := time.UnixMilli(req.Msg.GetExpiresAtMs()).UTC()
		expiresAt = &value
	}
	issued, err := s.keys.Issue(ctx, corev2.IssueServiceKeyRequest{
		Caller: caller, NamespaceID: corev2.Ref(req.Msg.GetNamespaceId()), Name: corev2.Ref(req.Msg.GetName()),
		LookupPrefix: req.Msg.GetLookupPrefix(), SecretHash: append([]byte(nil), req.Msg.GetSecretHash()...),
		Operations: operations, AllowedAgents: agents, CanAssertScope: req.Msg.GetCanAssertScope(),
		ExpiresAt: expiresAt, IdempotencyKey: corev2.Ref(req.Msg.GetIdempotencyKey()),
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	response := &kernelv2.IssuedServiceKey{
		Id: issued.ID, Name: string(issued.Name), Prefix: issued.Prefix,
		Created: issued.Created,
	}
	if !issued.CreatedAt.IsZero() {
		response.CreatedAtMs = issued.CreatedAt.UnixMilli()
	}
	if issued.ExpiresAt != nil {
		response.ExpiresAtMs = issued.ExpiresAt.UnixMilli()
	}
	return connect.NewResponse(response), nil
}

func (s *Server) RevokeServiceKey(ctx context.Context, req *connect.Request[kernelv2.RevokeServiceKeyRequest]) (*connect.Response[emptypb.Empty], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.keys == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("service-key administration unavailable"))
	}
	if err := s.keys.Revoke(ctx, corev2.RevokeServiceKeyRequest{
		Caller: caller, ID: corev2.Ref(req.Msg.GetId()), Reason: req.Msg.GetReason(),
	}); err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *Server) PutSecret(ctx context.Context, req *connect.Request[kernelv2.PutSecretRequest]) (*connect.Response[kernelv2.SecretMetadata], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.secrets == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("secret storage unavailable"))
	}
	plaintext := append([]byte(nil), req.Msg.GetPlaintext()...)
	defer clear(plaintext)
	defer clear(req.Msg.GetPlaintext())
	metadata, err := s.secrets.PutSecret(ctx, corev2.PutSecretRequest{
		Caller: caller, NamespaceID: corev2.Ref(req.Msg.GetNamespaceId()), Name: corev2.Ref(req.Msg.GetName()),
		Plaintext: plaintext, ExternalURI: req.Msg.GetExternalUri(), Validate: req.Msg.GetValidate(),
		IdempotencyKey: corev2.Ref(req.Msg.GetIdempotencyKey()),
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	return connect.NewResponse(secretMetadataToProto(metadata)), nil
}

func (s *Server) RevokeSecret(ctx context.Context, req *connect.Request[kernelv2.RevokeSecretRequest]) (*connect.Response[emptypb.Empty], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.secrets == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("secret storage unavailable"))
	}
	if err := s.secrets.RevokeSecret(ctx, corev2.RevokeSecretRequest{
		Caller: caller, ID: corev2.Ref(req.Msg.GetId()), Reason: req.Msg.GetReason(),
	}); err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *Server) SyncLimits(ctx context.Context, req *connect.Request[kernelv2.SyncLimitsRequest]) (*connect.Response[kernelv2.SyncLimitsResponse], error) {
	caller, err := authenticatedCaller(ctx, req != nil && req.Msg != nil)
	if err != nil {
		return nil, err
	}
	if s == nil || s.limits == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("limit synchronization unavailable"))
	}
	limits, err := limitSpecsFromProto(req.Msg.GetLimits())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := s.limits.Sync(ctx, corev2.SyncLimitsRequest{
		Caller: caller, NamespaceID: corev2.Ref(req.Msg.GetNamespaceId()), Owner: corev2.Ref(req.Msg.GetOwner()),
		Revision: req.Msg.GetRevision(), Limits: limits, IdempotencyKey: corev2.Ref(req.Msg.GetIdempotencyKey()),
	})
	if err != nil {
		return nil, connectError(ctx, err, corev2.Decision{})
	}
	return connect.NewResponse(&kernelv2.SyncLimitsResponse{
		Revision: result.Revision, Created: result.Created, Updated: result.Updated, Disabled: result.Disabled,
	}), nil
}

func authenticatedCaller(ctx context.Context, requestPresent bool) (corev2.Caller, error) {
	caller, ok := v2authctx.CallerFrom(ctx)
	if !ok {
		return corev2.Caller{}, connect.NewError(connect.CodeUnauthenticated, errors.New("service key required"))
	}
	if !requestPresent {
		return corev2.Caller{}, connect.NewError(connect.CodeInvalidArgument, errors.New("request required"))
	}
	return caller, nil
}

func manifestFromProto(input *kernelv2.Manifest) (corev2.Manifest, error) {
	if input == nil || input.GetNamespace() == nil {
		return corev2.Manifest{}, errors.New("manifest and namespace are required")
	}
	namespace := input.GetNamespace()
	manifest := corev2.Manifest{Namespace: corev2.Namespace{
		Account: corev2.Ref(namespace.GetAccount()), Application: corev2.Ref(namespace.GetApplication()),
		Environment: corev2.Ref(namespace.GetEnvironment()),
	}}
	manifest.Routes = make([]corev2.RouteSpec, 0, len(input.GetRoutes()))
	for _, route := range input.GetRoutes() {
		if route == nil {
			return corev2.Manifest{}, errors.New("manifest contains a nil route")
		}
		pricing := make([]corev2.ModelPrice, 0, len(route.GetPricing()))
		for _, price := range route.GetPricing() {
			if price == nil {
				return corev2.Manifest{}, errors.New("manifest contains a nil model price")
			}
			pricing = append(pricing, corev2.ModelPrice{
				Model:                       corev2.Ref(price.GetModel()),
				InputNanosPerMillionTokens:  price.GetInputNanosPerMillionTokens(),
				OutputNanosPerMillionTokens: price.GetOutputNanosPerMillionTokens(),
			})
		}
		manifest.Routes = append(manifest.Routes, corev2.RouteSpec{
			Name: corev2.Ref(route.GetName()), Provider: corev2.Ref(route.GetProvider()), BaseURL: route.GetBaseUrl(),
			Secret: corev2.Ref(route.GetSecret()), AllowedModels: append([]string(nil), route.GetAllowedModels()...),
			DefaultModel: route.GetDefaultModel(), PricingRevision: route.GetPricingRevision(), Pricing: pricing,
		})
	}
	manifest.Agents = make([]corev2.AgentSpec, 0, len(input.GetAgents()))
	for _, agent := range input.GetAgents() {
		if agent == nil {
			return corev2.Manifest{}, errors.New("manifest contains a nil agent")
		}
		kind := corev2.AgentKind("")
		switch agent.GetKind() {
		case kernelv2.AgentKind_AGENT_KIND_LLM:
			kind = corev2.AgentLLM
		case kernelv2.AgentKind_AGENT_KIND_EMBEDDING:
			kind = corev2.AgentEmbedding
		}
		manifest.Agents = append(manifest.Agents, corev2.AgentSpec{
			Name: corev2.Ref(agent.GetName()), Kind: kind, Route: corev2.Ref(agent.GetRoute()), Enabled: agent.GetEnabled(),
		})
	}
	limits, err := limitSpecsFromProto(input.GetLimits())
	if err != nil {
		return corev2.Manifest{}, err
	}
	manifest.Limits = limits
	return manifest, nil
}

func limitSpecsFromProto(input []*kernelv2.LimitSpec) ([]corev2.LimitSpec, error) {
	limits := make([]corev2.LimitSpec, 0, len(input))
	for _, limit := range input {
		if limit == nil {
			return nil, errors.New("request contains a nil limit")
		}
		selector := limit.GetSelector()
		var coreSelector corev2.LimitSelector
		if selector != nil {
			coreSelector = corev2.LimitSelector{
				Tenant: corev2.Ref(selector.GetTenant()), Actor: corev2.Ref(selector.GetActor()),
				BillTo: corev2.Ref(selector.GetBillTo()), Agent: corev2.Ref(selector.GetAgent()),
				Model: corev2.Ref(selector.GetModel()), Feature: corev2.Ref(selector.GetFeature()),
			}
		}
		window := corev2.Window("")
		switch limit.GetWindow() {
		case kernelv2.LimitWindow_LIMIT_WINDOW_ALL_TIME:
			window = corev2.WindowAllTime
		case kernelv2.LimitWindow_LIMIT_WINDOW_DAY:
			window = corev2.WindowDay
		case kernelv2.LimitWindow_LIMIT_WINDOW_MONTH:
			window = corev2.WindowMonth
		}
		var softCap *int64
		if limit.SoftCap != nil {
			value := limit.GetSoftCap()
			softCap = &value
		}
		limits = append(limits, corev2.LimitSpec{
			Key: corev2.Ref(limit.GetKey()), Metric: corev2.Metric(limit.GetMetric()), Selector: coreSelector,
			Window: window, HardCap: limit.GetHardCap(), SoftCap: softCap, Enabled: limit.GetEnabled(),
		})
	}
	return limits, nil
}

func applyResultToProto(result corev2.ApplyResult) *kernelv2.ApplyResponse {
	changes := make([]*kernelv2.Change, 0, len(result.Changes))
	for _, change := range result.Changes {
		kind := kernelv2.ChangeKind_CHANGE_KIND_UNSPECIFIED
		switch change.Kind {
		case corev2.ChangeCreate:
			kind = kernelv2.ChangeKind_CHANGE_KIND_CREATE
		case corev2.ChangeUpdate:
			kind = kernelv2.ChangeKind_CHANGE_KIND_UPDATE
		case corev2.ChangeDelete:
			kind = kernelv2.ChangeKind_CHANGE_KIND_DELETE
		case corev2.ChangeUnchanged:
			kind = kernelv2.ChangeKind_CHANGE_KIND_UNCHANGED
		}
		changes = append(changes, &kernelv2.Change{
			Kind: kind, ResourceKind: change.ResourceKind, Name: string(change.Name), Fields: append([]string(nil), change.Fields...),
		})
	}
	return &kernelv2.ApplyResponse{
		NamespaceId: string(result.NamespaceID), Revision: result.Revision, Applied: result.Applied, Changes: changes,
	}
}

func secretMetadataToProto(metadata corev2.SecretMetadata) *kernelv2.SecretMetadata {
	source := kernelv2.SecretSource_SECRET_SOURCE_UNSPECIFIED
	switch metadata.Source {
	case corev2.SecretEncrypted:
		source = kernelv2.SecretSource_SECRET_SOURCE_ENCRYPTED
	case corev2.SecretExternal:
		source = kernelv2.SecretSource_SECRET_SOURCE_EXTERNAL
	}
	return &kernelv2.SecretMetadata{
		Id: metadata.ID, Name: string(metadata.Name), Source: source, Version: metadata.Version,
		Status: metadata.Status, UpdatedAtMs: metadata.UpdatedAt,
	}
}
