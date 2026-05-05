package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/kave-io/kave/core/bus"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
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

// ── Aggregate Read Models ─────────────────────────────────────────────────

func (s *Server) GetDashboardOverview(ctx context.Context, req *runtimev1.GetDashboardOverviewRequest) (*runtimev1.DashboardOverview, error) {
	recentLimit := int(req.RecentLimit)
	if recentLimit <= 0 {
		recentLimit = 12
	}
	if recentLimit > 100 {
		recentLimit = 100
	}

	runFilter := &runtimemodel.RunFilter{
		ProjectID: req.ProjectId,
		EnvID:     req.EnvId,
		FromMs:    req.FromMs,
		ToMs:      req.ToMs,
	}
	runsPage, err := s.appStore.ListRuns(ctx, runFilter, store.Page{Limit: 1000})
	if err != nil {
		return nil, err
	}

	spend, err := s.appStore.GetSpendReport(ctx, &runtimemodel.SpendFilter{
		ProjectID: req.ProjectId,
		EnvID:     req.EnvId,
		FromMs:    req.FromMs,
		ToMs:      req.ToMs,
	})
	if err != nil {
		return nil, err
	}

	spansPage, err := s.spanStore.QuerySpans(ctx, &runtimemodel.SpanFilter{
		ProjectID: req.ProjectId,
		EnvID:     req.EnvId,
		FromMs:    req.FromMs,
		ToMs:      req.ToMs,
	}, store.Page{Limit: 10000})
	if err != nil {
		return nil, err
	}

	overview := &runtimev1.DashboardOverview{
		Spend: spendReportToProto(spend),
	}
	agentRuns := map[string]int32{}
	agentSpend := map[string]money.Amount{}

	var latencyCount int64
	for _, run := range runsPage.Items {
		overview.TotalRuns++
		agentRuns[run.AgentID]++
		if run.Spent != 0 {
			agentSpend[run.AgentID] += run.Spent
		}
		switch run.Status {
		case "active":
			overview.ActiveRuns++
		case "blocked":
			overview.BlockedRuns++
		case "failed":
			overview.FailedRuns++
		}
		if run.EndedAt != nil && run.StartedAt > 0 && *run.EndedAt >= run.StartedAt {
			overview.AvgLatencyMs += *run.EndedAt - run.StartedAt
			latencyCount++
		}
		if len(overview.RecentRuns) < recentLimit {
			overview.RecentRuns = append(overview.RecentRuns, runToProto(run))
		}
		if (run.Status == "blocked" || run.Status == "failed") && len(overview.RecentAttentionRuns) < recentLimit {
			overview.RecentAttentionRuns = append(overview.RecentAttentionRuns, runToProto(run))
		}
	}
	if latencyCount > 0 {
		overview.AvgLatencyMs /= latencyCount
	}

	for _, span := range spansPage.Items {
		if span.InputTokens != nil {
			overview.TotalInputTokens += int64(*span.InputTokens)
		}
		if span.OutputTokens != nil {
			overview.TotalOutputTokens += int64(*span.OutputTokens)
		}
		if span.Cost != nil {
			agentSpend[span.AgentID] += *span.Cost
		}
	}

	overview.TopAgents = s.dashboardTopAgents(ctx, req.EnvId, agentRuns, agentSpend, recentLimit)
	return overview, nil
}

func (s *Server) GetTraceGraph(ctx context.Context, req *runtimev1.GetTraceGraphRequest) (*runtimev1.TraceGraph, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}

	filter := &runtimemodel.SpanFilter{RunID: req.RunId, TraceID: req.TraceId}
	if filter.RunID == "" && filter.TraceID == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id or trace_id is required")
	}
	spansPage, err := s.spanStore.QuerySpans(ctx, filter, store.Page{Limit: limit})
	if err != nil {
		return nil, err
	}

	runID := req.RunId
	if runID == "" && len(spansPage.Items) > 0 {
		runID = spansPage.Items[0].RunID
	}
	run, err := s.appStore.GetRunByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, status.Errorf(codes.NotFound, "run %q not found", runID)
	}

	graph := &runtimev1.TraceGraph{
		Run: runToProto(run),
	}
	if run.EndedAt != nil && run.StartedAt > 0 && *run.EndedAt >= run.StartedAt {
		graph.TotalDurationMs = *run.EndedAt - run.StartedAt
	}
	graph.Spans = make([]*runtimev1.SpanRow, 0, len(spansPage.Items))
	graph.Nodes = make([]*runtimev1.TraceGraphNode, 0, len(spansPage.Items))

	base := run.StartedAt
	if base == 0 && len(spansPage.Items) > 0 {
		base = spansPage.Items[0].StartedAt
	}
	depthByID := spanDepths(spansPage.Items)

	var totalCost money.Amount
	for _, span := range spansPage.Items {
		graph.Spans = append(graph.Spans, spanToProto(span))
		node := &runtimev1.TraceGraphNode{
			SpanId:     span.ID,
			Name:       span.Name,
			Connector:  span.Connector,
			HasError:   span.Error != nil && *span.Error != "",
			Depth:      int32(depthByID[span.ID]),
			OffsetMs:   span.StartedAt - base,
			DurationMs: span.DurationMs,
		}
		if span.ParentID != nil {
			node.ParentSpanId = *span.ParentID
			graph.Edges = append(graph.Edges, &runtimev1.TraceGraphEdge{ParentSpanId: *span.ParentID, ChildSpanId: span.ID})
		}
		if span.Model != nil {
			node.Model = *span.Model
		}
		if span.Cost != nil {
			node.Cost = amountToProto(*span.Cost)
			totalCost += *span.Cost
		}
		if span.InputTokens != nil {
			node.InputTokens = int64(*span.InputTokens)
			graph.TotalInputTokens += int64(*span.InputTokens)
		}
		if span.OutputTokens != nil {
			node.OutputTokens = int64(*span.OutputTokens)
			graph.TotalOutputTokens += int64(*span.OutputTokens)
		}
		if end := span.StartedAt + span.DurationMs - base; end > graph.TotalDurationMs {
			graph.TotalDurationMs = end
		}
		graph.Nodes = append(graph.Nodes, node)
	}
	graph.TotalCost = amountToProto(totalCost)
	return graph, nil
}

func (s *Server) dashboardTopAgents(ctx context.Context, envID string, runCounts map[string]int32, spend map[string]money.Amount, limit int) []*runtimev1.DashboardAgentSpend {
	agents, _ := s.appStore.ListAgents(ctx, envID, store.Page{Limit: 10000})
	names := make(map[string]string, len(agents.Items))
	for _, agent := range agents.Items {
		names[agent.ID] = agent.Name
		if _, ok := spend[agent.ID]; !ok {
			spend[agent.ID] = 0
		}
	}

	rows := make([]*runtimev1.DashboardAgentSpend, 0, len(spend))
	for agentID, amount := range spend {
		rows = append(rows, &runtimev1.DashboardAgentSpend{
			AgentId:   agentID,
			AgentName: names[agentID],
			Spend:     amountToProto(amount),
			RunCount:  runCounts[agentID],
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return spend[rows[i].AgentId] > spend[rows[j].AgentId]
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func spanDepths(spans []*runtimemodel.SpanRow) map[string]int {
	byID := make(map[string]*runtimemodel.SpanRow, len(spans))
	for _, span := range spans {
		byID[span.ID] = span
	}
	depths := make(map[string]int, len(spans))
	var depthOf func(*runtimemodel.SpanRow) int
	depthOf = func(span *runtimemodel.SpanRow) int {
		if span == nil || span.ParentID == nil || *span.ParentID == "" {
			return 0
		}
		if d, ok := depths[span.ID]; ok {
			return d
		}
		d := depthOf(byID[*span.ParentID]) + 1
		depths[span.ID] = d
		return d
	}
	for _, span := range spans {
		depths[span.ID] = depthOf(span)
	}
	return depths
}

// ── Streaming Operations ───────────────────────────────────────────────────

func (s *Server) WatchEvents(req *runtimev1.WatchEventsRequest, stream grpc.ServerStreamingServer[runtimev1.RuntimeEvent]) error {
	if s.bus == nil {
		return status.Error(codes.Unavailable, "event bus not configured")
	}
	filter := bus.Filter{ProjectID: req.ProjectId, EnvID: req.EnvId}
	if req.Kind != "" {
		filter.Kinds = []string{req.Kind}
	}
	ch, cancel := s.bus.Subscribe(filter)
	defer cancel()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&runtimev1.RuntimeEvent{Kind: ev.Kind, At: ev.At, Data: ev.Payload}); err != nil {
				return err
			}
		}
	}
}

func (s *Server) WatchLogs(req *runtimev1.WatchLogsRequest, stream grpc.ServerStreamingServer[runtimev1.LogLine]) error {
	if s.bus == nil {
		return status.Error(codes.Unavailable, "event bus not configured")
	}
	ch, cancel := s.bus.Subscribe(bus.Filter{Kinds: []string{"daemon.log"}})
	defer cancel()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			var payload struct {
				Level   string            `json:"level"`
				Message string            `json:"message"`
				Context map[string]string `json:"context"`
			}
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				continue
			}
			if req.Level != "" && strings.ToLower(payload.Level) != strings.ToLower(req.Level) {
				continue
			}
			if err := stream.Send(&runtimev1.LogLine{At: ev.At, Level: payload.Level, Message: payload.Message, Context: payload.Context}); err != nil {
				return err
			}
		}
	}
}

func (s *Server) TailTraces(req *runtimev1.TailTracesRequest, stream grpc.ServerStreamingServer[runtimev1.TraceEvent]) error {
	if s.bus == nil {
		return status.Error(codes.Unavailable, "event bus not configured")
	}
	ch, cancel := s.bus.Subscribe(bus.Filter{Kinds: []string{"run.", "span."}, ProjectID: req.ProjectId, EnvID: req.EnvId, RunID: req.RunId})
	defer cancel()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if strings.HasPrefix(ev.Kind, "run.") {
				run, err := s.appStore.GetRunByID(ctx, ev.RunID)
				if err != nil || run == nil {
					continue
				}
				raw, err := json.Marshal(runToProto(run))
				if err != nil {
					continue
				}
				if err := stream.Send(&runtimev1.TraceEvent{Kind: "Run", At: ev.At, Data: raw}); err != nil {
					return err
				}
			} else if ev.Kind == "span.completed" {
				span, err := s.spanStore.GetSpan(ctx, ev.SpanID)
				if err != nil || span == nil {
					continue
				}
				raw, err := json.Marshal(spanToProto(span))
				if err != nil {
					continue
				}
				if err := stream.Send(&runtimev1.TraceEvent{Kind: "Span", At: ev.At, Data: raw}); err != nil {
					return err
				}
			}
		}
	}
}

func (s *Server) StreamSpans(req *runtimev1.StreamSpansRequest, stream grpc.ServerStreamingServer[runtimev1.SpanEvent]) error {
	if s.bus == nil {
		return status.Error(codes.Unavailable, "event bus not configured")
	}
	ch, cancel := s.bus.Subscribe(bus.Filter{Kinds: []string{"span.completed"}, ProjectID: req.ProjectId, EnvID: req.EnvId, RunID: req.RunId})
	defer cancel()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			span, err := s.spanStore.GetSpan(ctx, ev.SpanID)
			if err != nil || span == nil {
				continue
			}
			if err := stream.Send(&runtimev1.SpanEvent{At: ev.At, Span: spanToProto(span)}); err != nil {
				return err
			}
		}
	}
}

// ── Trace Operations ───────────────────────────────────────────────────────

func (s *Server) ExportTrace(ctx context.Context, req *runtimev1.ExportTraceRequest) (*runtimev1.ExportTraceResponse, error) {
	result, err := s.spanStore.QuerySpans(ctx, &runtimemodel.SpanFilter{RunID: req.TraceId}, store.Page{Limit: 10000})
	if err != nil {
		return nil, err
	}
	spans := make([]*runtimev1.SpanRow, len(result.Items))
	for i, span := range result.Items {
		spans[i] = spanToProto(span)
	}
	data, err := json.Marshal(spans)
	if err != nil {
		return nil, err
	}
	return &runtimev1.ExportTraceResponse{Data: data, ContentType: "application/json"}, nil
}

func (s *Server) IngestTraces(ctx context.Context, req *runtimev1.IngestTracesRequest) (*runtimev1.IngestTracesResponse, error) {
	return &runtimev1.IngestTracesResponse{Accepted: 0}, nil
}

func stringPtr(s string) *string { return &s }
