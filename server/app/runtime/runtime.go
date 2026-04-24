package runtime

import (
	"context"
	"encoding/json"

	"github.com/kave-io/kave/core/bus"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/store"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements runtimev1.RuntimeServiceServer.
type Server struct {
	runtimev1.UnimplementedRuntimeServiceServer

	appStore  store.AppStore
	spanStore store.SpanStore
	bus       *bus.Bus
}

// New creates a new RuntimeAPI server.
func New(appStore store.AppStore, spanStore store.SpanStore, b *bus.Bus) *Server {
	return &Server{appStore: appStore, spanStore: spanStore, bus: b}
}

// Register registers the RuntimeService server with gRPC.
func (s *Server) Register(srv *grpc.Server) {
	runtimev1.RegisterRuntimeServiceServer(srv, s)
}

// ── Run Operations ─────────────────────────────────────────────────────────

func (s *Server) CreateRun(ctx context.Context, req *runtimev1.CreateRunRequest) (*runtimev1.RunRecord, error) {
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		existing, err := s.appStore.GetRunByIdempotencyKey(ctx, req.EnvId, *req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return runToProto(existing), nil
		}
	}
	now := nowMS()
	triggerType := triggerTypeFromProto(req.TriggerType)
	if triggerType == "" {
		triggerType = "api"
	}
	run := &runtimemodel.RunRecord{
		ID:             newID("run"),
		ProjectID:      req.ProjectId,
		EnvID:          req.EnvId,
		AgentID:        req.AgentId,
		PolicyID:       req.PolicyId,
		Name:           req.Name,
		Status:         "active",
		Metadata:       map[string]any{},
		TriggerType:    triggerType,
		TriggerID:      req.TriggerId,
		CorrelationID:  req.CorrelationId,
		SessionID:      req.SessionId,
		IdempotencyKey: req.IdempotencyKey,
		StartedAt:      now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.appStore.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	s.publishRun("run.started", run)
	return runToProto(run), nil
}

func (s *Server) GetRun(ctx context.Context, req *runtimev1.GetRunRequest) (*runtimev1.RunRecord, error) {
	run, err := s.appStore.GetRunByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, status.Errorf(codes.NotFound, "run %q not found", req.Id)
	}
	return runToProto(run), nil
}

func (s *Server) ListRuns(ctx context.Context, req *runtimev1.ListRunsRequest) (*runtimev1.ListRunsResponse, error) {
	filter := runFilterFromProto(req.Filter)
	page := pageFromProto(req.Limit, req.Cursor)
	result, err := s.appStore.ListRuns(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	resp := &runtimev1.ListRunsResponse{}
	for _, r := range result.Items {
		resp.Runs = append(resp.Runs, runToProto(r))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

func (s *Server) UpdateRun(ctx context.Context, req *runtimev1.UpdateRunRequest) (*runtimev1.RunRecord, error) {
	if req.Update == nil {
		return s.GetRun(ctx, &runtimev1.GetRunRequest{Id: req.Id})
	}
	u := req.Update
	update := &runtimemodel.RunUpdate{}
	if u.Status != nil {
		sv := runStatusFromProto(*u.Status)
		update.Status = &sv
	}
	if u.Spent != nil {
		a := amountFromProto(u.Spent)
		update.Spent = &a
	}
	if u.ErrorMessage != nil {
		update.ErrorMessage = u.ErrorMessage
	}
	if u.EndedAtMs != nil {
		update.EndedAt = u.EndedAtMs
	}
	if u.Metadata != nil {
		update.Metadata = structToMap(u.Metadata)
	}
	if err := s.appStore.UpdateRun(ctx, req.Id, update); err != nil {
		return nil, err
	}
	updatedRun, err := s.GetRun(ctx, &runtimev1.GetRunRequest{Id: req.Id})
	if err == nil {
		if kind, ok := runEventKindFromString(runStatusFromProto(updatedRun.GetStatus())); ok {
			s.publishRun(kind, runToModel(updatedRun))
		}
	}
	return updatedRun, err
}

func (s *Server) CancelRun(ctx context.Context, req *runtimev1.CancelRunRequest) (*runtimev1.RunRecord, error) {
	run, err := s.appStore.GetRunByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, status.Errorf(codes.NotFound, "run %q not found", req.Id)
	}
	now := nowMS()
	update := &runtimemodel.RunUpdate{
		Status:  stringPtr("cancelled"),
		EndedAt: &now,
	}
	if reason := req.GetReason(); reason != "" {
		update.ErrorMessage = &reason
	}
	if err := s.appStore.UpdateRun(ctx, req.Id, update); err != nil {
		return nil, err
	}
	updatedRun, err := s.GetRun(ctx, &runtimev1.GetRunRequest{Id: req.Id})
	if err == nil {
		s.publishRun("run.canceled", runToModel(updatedRun))
	}
	return updatedRun, err
}

func (s *Server) WatchRuns(req *runtimev1.WatchRunsRequest, stream grpc.ServerStreamingServer[runtimev1.RunRecord]) error {
	if s.bus == nil {
		return status.Error(codes.Unavailable, "event bus not configured")
	}
	ch, cancel := s.bus.Subscribe(bus.Filter{Kinds: []string{"run."}, EnvID: req.EnvId})
	defer cancel()

	// Build status filter set (empty = all).
	wantStatus := make(map[string]bool, len(req.Statuses))
	for _, s := range req.Statuses {
		wantStatus[runStatusFromProto(s)] = true
	}

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			// Filter by envID and optional agentID.
			if req.EnvId != "" && ev.EnvID != req.EnvId {
				continue
			}
			if req.AgentId != nil && ev.AgentID != *req.AgentId {
				continue
			}
			// Fetch current run state and stream it.
			run, err := s.appStore.GetRunByID(ctx, ev.RunID)
			if err != nil || run == nil {
				continue
			}
			if len(wantStatus) > 0 && !wantStatus[run.Status] {
				continue
			}
			if err := stream.Send(runToProto(run)); err != nil {
				return err
			}
		}
	}
}

func (s *Server) publishRun(kind string, run *runtimemodel.RunRecord) {
	if s == nil || s.bus == nil || run == nil {
		return
	}
	raw, err := json.Marshal(runToProto(run))
	if err != nil {
		return
	}
	s.bus.Publish(bus.Event{
		Kind:      kind,
		ProjectID: run.ProjectID,
		EnvID:     run.EnvID,
		RunID:     run.ID,
		AgentID:   run.AgentID,
		At:        nowMS(),
		Payload:   raw,
	})
}

func runEventKindFromString(status string) (string, bool) {
	switch status {
	case "completed":
		return "run.completed", true
	case "failed":
		return "run.failed", true
	case "cancelled":
		return "run.canceled", true
	default:
		return "", false
	}
}

func runToModel(r *runtimev1.RunRecord) *runtimemodel.RunRecord {
	if r == nil {
		return nil
	}
	model := &runtimemodel.RunRecord{
		ID:             r.Id,
		ProjectID:      r.ProjectId,
		EnvID:          r.EnvId,
		AgentID:        r.AgentId,
		PolicyID:       r.PolicyId,
		Name:           r.Name,
		Status:         runStatusFromProto(r.Status),
		Metadata:       structToMap(r.Metadata),
		ErrorMessage:   r.ErrorMessage,
		TriggerType:    triggerTypeFromProto(r.TriggerType),
		TriggerID:      r.TriggerId,
		CorrelationID:  r.CorrelationId,
		SessionID:      r.SessionId,
		IdempotencyKey: r.IdempotencyKey,
		StartedAt:      r.StartedAtMs,
		EndedAt:        r.EndedAtMs,
		CreatedAt:      r.CreatedAtMs,
		UpdatedAt:      r.UpdatedAtMs,
	}
	if r.BudgetCap != nil {
		amount := amountFromProto(r.BudgetCap)
		model.BudgetCap = amount
	}
	if r.Spent != nil {
		amount := amountFromProto(r.Spent)
		model.Spent = amount
	}
	return model
}

// ── Action Operations ──────────────────────────────────────────────────────

func (s *Server) CreateAction(ctx context.Context, req *runtimev1.CreateActionRequest) (*runtimev1.ActionRecord, error) {
	now := nowMS()
	action := &runtimemodel.ActionRecord{
		ID:         newID("act"),
		RunID:      req.RunId,
		AgentID:    req.AgentId,
		ProjectID:  req.ProjectId,
		EnvID:      req.EnvId,
		ActionType: actionTypeFromProto(req.ActionType),
		Connector:  req.Connector,
		Method:     req.Method,
		Status:     "pending",
		Source:     "intercepted",
		Metadata:   map[string]any{},
		StartedAt:  &now,
	}
	if err := s.appStore.CreateAction(ctx, action); err != nil {
		return nil, err
	}
	return actionToProto(action), nil
}

func (s *Server) GetAction(ctx context.Context, req *runtimev1.GetActionRequest) (*runtimev1.ActionRecord, error) {
	action, err := s.appStore.GetAction(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if action == nil {
		return nil, status.Errorf(codes.NotFound, "action %q not found", req.Id)
	}
	return actionToProto(action), nil
}

func (s *Server) ListActions(ctx context.Context, req *runtimev1.ListActionsRequest) (*runtimev1.ListActionsResponse, error) {
	var runID string
	if req.Filter != nil {
		runID = req.Filter.RunId
	}
	page := pageFromProto(req.Limit, req.Cursor)
	result, err := s.appStore.ListActionsByRun(ctx, runID, page)
	if err != nil {
		return nil, err
	}
	resp := &runtimev1.ListActionsResponse{}
	for _, a := range result.Items {
		resp.Actions = append(resp.Actions, actionToProto(a))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

// ── Span Operations ────────────────────────────────────────────────────────

func (s *Server) OpenSpan(ctx context.Context, req *runtimev1.OpenSpanRequest) (*runtimev1.SpanRow, error) {
	row := spanInputToModel(req.Span)
	if err := s.spanStore.OpenSpan(ctx, row); err != nil {
		return nil, err
	}
	return spanToProto(row), nil
}

func (s *Server) CloseSpan(ctx context.Context, req *runtimev1.CloseSpanRequest) (*runtimev1.SpanRow, error) {
	end := spanEndToModel(req.End)
	if err := s.spanStore.CloseSpan(ctx, req.SpanId, end); err != nil {
		return nil, err
	}
	row, err := s.spanStore.GetSpan(ctx, req.SpanId)
	if err != nil {
		return nil, err
	}
	return spanToProto(row), nil
}

func (s *Server) GetSpan(ctx context.Context, req *runtimev1.GetSpanRequest) (*runtimev1.SpanRow, error) {
	row, err := s.spanStore.GetSpan(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, status.Errorf(codes.NotFound, "span %q not found", req.Id)
	}
	return spanToProto(row), nil
}

func (s *Server) QuerySpans(ctx context.Context, req *runtimev1.QuerySpansRequest) (*runtimev1.QuerySpansResponse, error) {
	filter := spanFilterFromProto(req.Filter)
	page := pageFromProto(req.Limit, req.Cursor)
	result, err := s.spanStore.QuerySpans(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	resp := &runtimev1.QuerySpansResponse{}
	for _, span := range result.Items {
		resp.Spans = append(resp.Spans, spanToProto(span))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

// ── Cost Operations ────────────────────────────────────────────────────────

func (s *Server) GetPriceBook(ctx context.Context, req *runtimev1.GetPriceBookRequest) (*runtimev1.PriceBook, error) {
	book, err := s.appStore.GetPriceBook(ctx)
	if err != nil {
		return nil, err
	}
	if book == nil {
		return nil, status.Error(codes.NotFound, "price book not found")
	}
	return priceBookToProto(book), nil
}

func (s *Server) GetSpendReport(ctx context.Context, req *runtimev1.GetSpendReportRequest) (*runtimev1.SpendReport, error) {
	report, err := s.appStore.GetSpendReport(ctx, spendFilterFromProto(req.Filter))
	if err != nil {
		return nil, err
	}
	return spendReportToProto(report), nil
}

func stringPtr(s string) *string { return &s }
