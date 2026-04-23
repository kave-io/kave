package runtime

import (
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	commonv1 "github.com/kave-io/kave/proto/gen/kave/common/v1"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newID(prefix string) string { return ids.New(prefix) }

func nowMS() int64 { return time.Now().UnixMilli() }

// ── Amount ────────────────────────────────────────────────────────────────────

func amountToProto(a money.Amount) *commonv1.Amount {
	return &commonv1.Amount{Decimal: a.String()}
}

func amountFromProto(p *commonv1.Amount) money.Amount {
	if p == nil {
		return 0
	}
	a, _ := money.ParseAmount(p.Decimal)
	return a
}

func ptrAmountToProto(a money.Amount) *commonv1.Amount {
	if a == 0 {
		return nil
	}
	return amountToProto(a)
}

// ── Run ───────────────────────────────────────────────────────────────────────

func runToProto(r *runtimemodel.RunRecord) *runtimev1.RunRecord {
	if r == nil {
		return nil
	}
	p := &runtimev1.RunRecord{
		Id:           r.ID,
		ProjectId:    r.ProjectID,
		EnvId:        r.EnvID,
		AgentId:      r.AgentID,
		PolicyId:     r.PolicyID,
		Name:         r.Name,
		Status:       runStatusToProto(r.Status),
		BudgetCap:    ptrAmountToProto(r.BudgetCap),
		Spent:        ptrAmountToProto(r.Spent),
		Metadata:     mapToStruct(r.Metadata),
		ErrorMessage: r.ErrorMessage,
		TriggerType:  triggerTypeToProto(r.TriggerType),
		TriggerId:    r.TriggerID,
		StartedAtMs:  r.StartedAt,
		EndedAtMs:    r.EndedAt,
		CreatedAtMs:  r.CreatedAt,
		UpdatedAtMs:  r.UpdatedAt,
	}
	return p
}

func runStatusToProto(s string) runtimev1.RunStatus {
	switch s {
	case "active":
		return runtimev1.RunStatus_RUN_STATUS_ACTIVE
	case "completed":
		return runtimev1.RunStatus_RUN_STATUS_COMPLETED
	case "failed":
		return runtimev1.RunStatus_RUN_STATUS_FAILED
	case "cancelled":
		return runtimev1.RunStatus_RUN_STATUS_CANCELLED
	case "timed_out":
		return runtimev1.RunStatus_RUN_STATUS_TIMED_OUT
	case "blocked":
		return runtimev1.RunStatus_RUN_STATUS_BLOCKED
	default:
		return runtimev1.RunStatus_RUN_STATUS_UNSPECIFIED
	}
}

func runStatusFromProto(s runtimev1.RunStatus) string {
	switch s {
	case runtimev1.RunStatus_RUN_STATUS_ACTIVE:
		return "active"
	case runtimev1.RunStatus_RUN_STATUS_COMPLETED:
		return "completed"
	case runtimev1.RunStatus_RUN_STATUS_FAILED:
		return "failed"
	case runtimev1.RunStatus_RUN_STATUS_CANCELLED:
		return "cancelled"
	case runtimev1.RunStatus_RUN_STATUS_TIMED_OUT:
		return "timed_out"
	case runtimev1.RunStatus_RUN_STATUS_BLOCKED:
		return "blocked"
	default:
		return ""
	}
}

func triggerTypeToProto(t string) runtimev1.TriggerType {
	switch t {
	case "api":
		return runtimev1.TriggerType_TRIGGER_TYPE_API
	case "schedule":
		return runtimev1.TriggerType_TRIGGER_TYPE_SCHEDULE
	case "webhook":
		return runtimev1.TriggerType_TRIGGER_TYPE_WEBHOOK
	case "manual":
		return runtimev1.TriggerType_TRIGGER_TYPE_MANUAL
	default:
		return runtimev1.TriggerType_TRIGGER_TYPE_UNSPECIFIED
	}
}

func runFilterFromProto(f *runtimev1.RunFilter) *runtimemodel.RunFilter {
	if f == nil {
		return &runtimemodel.RunFilter{}
	}
	return &runtimemodel.RunFilter{
		ProjectID: f.ProjectId,
		EnvID:     f.EnvId,
		AgentID:   f.AgentId,
		Status:    runStatusFromProto(f.Status),
		FromMs:    f.FromMs,
		ToMs:      f.ToMs,
	}
}

// ── Action ────────────────────────────────────────────────────────────────────

func actionToProto(a *runtimemodel.ActionRecord) *runtimev1.ActionRecord {
	if a == nil {
		return nil
	}
	p := &runtimev1.ActionRecord{
		Id:          a.ID,
		RunId:       a.RunID,
		AgentId:     a.AgentID,
		ProjectId:   a.ProjectID,
		EnvId:       a.EnvID,
		ParentId:    a.ParentID,
		ActionType:  actionTypeToProto(a.ActionType),
		Connector:   a.Connector,
		Method:      a.Method,
		Error:       a.Error,
		StartedAtMs: a.StartedAt,
		EndedAtMs:   a.EndedAt,
		Depth:       int32(a.Depth),
		Seq:         int32(a.Seq),
		Status:      actionStatusToProto(a.Status),
		Source:      actionSourceToProto(a.Source),
		Metadata:    mapToStruct(a.Metadata),
	}
	if a.Input != nil {
		p.Input = *a.Input
	}
	if a.Output != nil {
		p.Output = *a.Output
	}
	return p
}

func actionTypeToProto(t string) runtimev1.ActionType {
	switch t {
	case "llm":
		return runtimev1.ActionType_ACTION_TYPE_LLM
	case "tool":
		return runtimev1.ActionType_ACTION_TYPE_TOOL
	case "retrieval":
		return runtimev1.ActionType_ACTION_TYPE_RETRIEVAL
	case "mutation":
		return runtimev1.ActionType_ACTION_TYPE_MUTATION
	case "api":
		return runtimev1.ActionType_ACTION_TYPE_API
	default:
		return runtimev1.ActionType_ACTION_TYPE_UNSPECIFIED
	}
}

func actionTypeFromProto(t runtimev1.ActionType) string {
	switch t {
	case runtimev1.ActionType_ACTION_TYPE_LLM:
		return "llm"
	case runtimev1.ActionType_ACTION_TYPE_TOOL:
		return "tool"
	case runtimev1.ActionType_ACTION_TYPE_RETRIEVAL:
		return "retrieval"
	case runtimev1.ActionType_ACTION_TYPE_MUTATION:
		return "mutation"
	case runtimev1.ActionType_ACTION_TYPE_API:
		return "api"
	default:
		return ""
	}
}

func actionStatusToProto(s string) runtimev1.ActionStatus {
	switch s {
	case "pending":
		return runtimev1.ActionStatus_ACTION_STATUS_PENDING
	case "running":
		return runtimev1.ActionStatus_ACTION_STATUS_RUNNING
	case "completed":
		return runtimev1.ActionStatus_ACTION_STATUS_COMPLETED
	case "failed":
		return runtimev1.ActionStatus_ACTION_STATUS_FAILED
	case "blocked":
		return runtimev1.ActionStatus_ACTION_STATUS_BLOCKED
	case "retrying":
		return runtimev1.ActionStatus_ACTION_STATUS_RETRYING
	default:
		return runtimev1.ActionStatus_ACTION_STATUS_UNSPECIFIED
	}
}

func actionSourceToProto(s string) runtimev1.ActionSource {
	switch s {
	case "intercepted":
		return runtimev1.ActionSource_ACTION_SOURCE_INTERCEPTED
	case "observed":
		return runtimev1.ActionSource_ACTION_SOURCE_OBSERVED
	default:
		return runtimev1.ActionSource_ACTION_SOURCE_UNSPECIFIED
	}
}

// ── Span ──────────────────────────────────────────────────────────────────────

func spanToProto(s *runtimemodel.SpanRow) *runtimev1.SpanRow {
	if s == nil {
		return nil
	}
	p := &runtimev1.SpanRow{
		Id:          s.ID,
		ProjectId:   s.ProjectID,
		EnvId:       s.EnvID,
		AgentId:     s.AgentID,
		RunId:       s.RunID,
		ActionId:    s.ActionID,
		ParentId:    s.ParentID,
		Name:        s.Name,
		Kind:        spanKindToProto(s.Kind),
		Source:      spanSourceToProto(s.Source),
		Connector:   s.Connector,
		StartedAtMs: s.StartedAt,
		EndedAtMs:   s.EndedAt,
		DurationMs:  s.DurationMs,
		Error:       s.Error,
		Model:       s.Model,
		TraceId:     s.TraceID,
		RootSpanId:  s.RootSpanID,
		CreatedAtMs: s.CreatedAt,
	}
	if s.Input != nil {
		p.Input = *s.Input
	}
	if s.Output != nil {
		p.Output = *s.Output
	}
	if s.Attrs != nil {
		p.Attrs = *s.Attrs
	}
	if s.Cost != nil {
		p.Cost = amountToProto(*s.Cost)
	}
	ptrInt(s.InputTokens, &p.InputTokens)
	ptrInt(s.OutputTokens, &p.OutputTokens)
	ptrInt(s.CacheReadTokens, &p.CacheReadTokens)
	ptrInt(s.CacheWriteTokens, &p.CacheWriteTokens)
	ptrInt(s.ReasoningTokens, &p.ReasoningTokens)
	ptrInt(s.AudioInputTokens, &p.AudioInputTokens)
	ptrInt(s.AudioOutputTokens, &p.AudioOutputTokens)
	ptrInt(s.ImageUnits, &p.ImageUnits)
	ptrInt(s.RequestCount, &p.RequestCount)
	p.ComputeMs = s.ComputeMs
	p.StorageBytes = s.StorageBytes
	p.BandwidthBytes = s.BandwidthBytes
	return p
}

func ptrInt(src *int, dst **int32) {
	if src != nil {
		v := int32(*src)
		*dst = &v
	}
}

func spanKindToProto(k string) runtimev1.SpanKind {
	switch k {
	case "action":
		return runtimev1.SpanKind_SPAN_KIND_ACTION
	case "observed_action":
		return runtimev1.SpanKind_SPAN_KIND_OBSERVED_ACTION
	case "import":
		return runtimev1.SpanKind_SPAN_KIND_IMPORT
	default:
		return runtimev1.SpanKind_SPAN_KIND_UNSPECIFIED
	}
}

func spanSourceToProto(src string) runtimev1.SpanSource {
	switch src {
	case "intercept":
		return runtimev1.SpanSource_SPAN_SOURCE_INTERCEPT
	case "report":
		return runtimev1.SpanSource_SPAN_SOURCE_REPORT
	case "otel_import":
		return runtimev1.SpanSource_SPAN_SOURCE_OTEL_IMPORT
	default:
		return runtimev1.SpanSource_SPAN_SOURCE_UNSPECIFIED
	}
}

func spanInputToModel(in *runtimev1.SpanInput) *runtimemodel.SpanRow {
	if in == nil {
		return &runtimemodel.SpanRow{}
	}
	row := &runtimemodel.SpanRow{
		ID:         newID("spn"),
		ProjectID:  in.ProjectId,
		EnvID:      in.EnvId,
		AgentID:    in.AgentId,
		RunID:      in.RunId,
		ActionID:   in.ActionId,
		ParentID:   in.ParentId,
		Name:       in.Name,
		Kind:       spanKindFromProto(in.Kind),
		Source:     spanSourceFromProto(in.Source),
		Connector:  in.Connector,
		StartedAt:  in.StartedAtMs,
		TraceID:    in.TraceId,
		RootSpanID: in.RootSpanId,
		CreatedAt:  nowMS(),
	}
	if len(in.Input) > 0 {
		b := in.Input
		row.Input = &b
	}
	return row
}

func spanKindFromProto(k runtimev1.SpanKind) string {
	switch k {
	case runtimev1.SpanKind_SPAN_KIND_ACTION:
		return "action"
	case runtimev1.SpanKind_SPAN_KIND_OBSERVED_ACTION:
		return "observed_action"
	case runtimev1.SpanKind_SPAN_KIND_IMPORT:
		return "import"
	default:
		return ""
	}
}

func spanSourceFromProto(s runtimev1.SpanSource) string {
	switch s {
	case runtimev1.SpanSource_SPAN_SOURCE_INTERCEPT:
		return "intercept"
	case runtimev1.SpanSource_SPAN_SOURCE_REPORT:
		return "report"
	case runtimev1.SpanSource_SPAN_SOURCE_OTEL_IMPORT:
		return "otel_import"
	default:
		return ""
	}
}

func spanEndToModel(end *runtimev1.SpanEnd) *runtimemodel.SpanEnd {
	if end == nil {
		return &runtimemodel.SpanEnd{}
	}
	se := &runtimemodel.SpanEnd{
		EndedAt:           end.EndedAtMs,
		DurationMs:        end.DurationMs,
		Error:             end.Error,
		InputTokens:       ptrInt32ToInt(end.InputTokens),
		OutputTokens:      ptrInt32ToInt(end.OutputTokens),
		CacheReadTokens:   ptrInt32ToInt(end.CacheReadTokens),
		CacheWriteTokens:  ptrInt32ToInt(end.CacheWriteTokens),
		ReasoningTokens:   ptrInt32ToInt(end.ReasoningTokens),
		AudioInputTokens:  ptrInt32ToInt(end.AudioInputTokens),
		AudioOutputTokens: ptrInt32ToInt(end.AudioOutputTokens),
		ImageUnits:        ptrInt32ToInt(end.ImageUnits),
		RequestCount:      ptrInt32ToInt(end.RequestCount),
		ComputeMs:         end.ComputeMs,
		StorageBytes:      end.StorageBytes,
		BandwidthBytes:    end.BandwidthBytes,
		Model:             end.Model,
	}
	if len(end.Output) > 0 {
		b := end.Output
		se.Output = &b
	}
	if len(end.Attrs) > 0 {
		b := end.Attrs
		se.Attrs = &b
	}
	if end.Cost != nil {
		a := amountFromProto(end.Cost)
		se.Cost = &a
	}
	return se
}

func ptrInt32ToInt(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func spanFilterFromProto(f *runtimev1.SpanFilter) *runtimemodel.SpanFilter {
	if f == nil {
		return &runtimemodel.SpanFilter{}
	}
	return &runtimemodel.SpanFilter{
		RunID:    f.RunId,
		ActionID: f.ActionId,
		FromMs:   f.FromMs,
		ToMs:     f.ToMs,
		HasError: f.HasError,
	}
}

func spendFilterFromProto(f *runtimev1.SpendFilter) *runtimemodel.SpendFilter {
	if f == nil {
		return &runtimemodel.SpendFilter{}
	}
	return &runtimemodel.SpendFilter{
		ProjectID: f.ProjectId,
		EnvID:     f.EnvId,
		PolicyID:  f.PolicyId,
		AgentID:   f.AgentId,
		Connector: f.Connector,
		Model:     f.Model,
		FromMs:    f.FromMs,
		ToMs:      f.ToMs,
	}
}

// ── Cost ─────────────────────────────────────────────────────────────────────

func priceBookToProto(book *runtimemodel.PriceBook) *runtimev1.PriceBook {
	if book == nil {
		return nil
	}
	out := &runtimev1.PriceBook{
		Version: book.Version,
		Entries: make([]*runtimev1.PriceModel, 0, len(book.Entries)),
	}
	for _, entry := range book.Entries {
		e := entry
		proto := &runtimev1.PriceModel{
			Provider:              e.Provider,
			Match:                 e.Match,
			Source:                e.Source,
			Currency:              string(e.Currency),
			InputPerMillion:       amountToProto(e.InputPerMillion),
			OutputPerMillion:      amountToProto(e.OutputPerMillion),
			CacheReadPerMillion:   amountToProto(e.CacheReadPerMillion),
			CacheWritePerMillion:  amountToProto(e.CacheWritePerMillion),
			ReasoningPerMillion:   amountToProto(e.ReasoningPerMillion),
			AudioInputPerMillion:  amountToProto(e.AudioInputPerMillion),
			AudioOutputPerMillion: amountToProto(e.AudioOutputPerMillion),
			ImageUnitPrice:        amountToProto(e.ImageUnitPrice),
			PerRequest:            amountToProto(e.PerRequest),
			PerComputeMs:          amountToProto(e.PerComputeMs),
			PerGbStored:           amountToProto(e.PerGBStored),
			PerGbTransferred:      amountToProto(e.PerGBTransferred),
			EffectiveFromMs:       e.EffectiveFrom,
			RevisionNote:          e.RevisionNote,
		}
		if e.EffectiveTo != nil {
			proto.EffectiveToMs = e.EffectiveTo
		}
		out.Entries = append(out.Entries, proto)
	}
	return out
}

func spendReportToProto(report *runtimemodel.SpendReport) *runtimev1.SpendReport {
	if report == nil {
		return nil
	}
	out := &runtimev1.SpendReport{
		Total:         amountToProto(report.Total),
		ByProject:     map[string]*commonv1.Amount{},
		ByEnv:         map[string]*commonv1.Amount{},
		ByPolicy:      map[string]*commonv1.Amount{},
		ByAgent:       map[string]*commonv1.Amount{},
		ByConnector:   map[string]*commonv1.Amount{},
		ByModel:       map[string]*commonv1.Amount{},
		PeriodStartMs: report.PeriodStart,
		PeriodEndMs:   report.PeriodEnd,
	}
	for k, v := range report.ByProject {
		amount := v
		out.ByProject[k] = amountToProto(amount)
	}
	for k, v := range report.ByEnv {
		amount := v
		out.ByEnv[k] = amountToProto(amount)
	}
	for k, v := range report.ByPolicy {
		amount := v
		out.ByPolicy[k] = amountToProto(amount)
	}
	for k, v := range report.ByAgent {
		amount := v
		out.ByAgent[k] = amountToProto(amount)
	}
	for k, v := range report.ByConnector {
		amount := v
		out.ByConnector[k] = amountToProto(amount)
	}
	for k, v := range report.ByModel {
		amount := v
		out.ByModel[k] = amountToProto(amount)
	}
	return out
}

// ── Page ──────────────────────────────────────────────────────────────────────

func pageFromProto(limit int32, cursor string) store.Page {
	return store.Page{Limit: int(limit), Cursor: cursor}
}

// ── Struct helpers ────────────────────────────────────────────────────────────

func mapToStruct(m map[string]any) *structpb.Struct {
	if len(m) == 0 {
		return nil
	}
	s, _ := structpb.NewStruct(m)
	return s
}

func structToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return nil
	}
	return s.AsMap()
}
