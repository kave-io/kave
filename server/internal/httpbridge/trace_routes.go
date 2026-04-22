package httpbridge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/contract"
	tracetree "github.com/kave-io/kave/server/ops/trace"
	tracexport "github.com/kave-io/kave/server/ops/trace/export"
	coltracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type traceSummary struct {
	TraceID     string          `json:"trace_id"`
	RootSpanID  string          `json:"root_span_id"`
	SpanCount   int             `json:"span_count"`
	DurationMs  int64           `json:"duration_ms"`
	StartedAt   string          `json:"started_at"`
	StartedAtMS int64           `json:"started_at_ms"`
	EndedAt     string          `json:"ended_at"`
	EndedAtMS   int64           `json:"ended_at_ms"`
	TotalCost   *contract.Money `json:"total_cost,omitempty"`
}

type traceView struct {
	TraceID    string          `json:"trace_id"`
	RootSpanID string          `json:"root_span_id"`
	Root       *tracetree.Node `json:"root"`
}

// RegisterTraceRoutes installs raw trace export and OTLP ingest endpoints.
func RegisterTraceRoutes(mux *http.ServeMux, spans store.SpanStore) {
	mux.HandleFunc("GET /api/v1/traces/{id}/export", exportTraceHandler(spans))
	mux.HandleFunc("POST /v1/traces", ingestTraceHandler(spans))
}

func listTraces(spans store.SpanStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		filter := spanFilterFromQuery(query)
		limit, cursor := pageFromQuery(query)
		rows, err := querySpansForTraces(ctx, spans, filter, limit)
		if err != nil {
			return Outcome{Kind: "TraceList"}, err
		}
		summaries := summarizeTraces(rows)
		sort.SliceStable(summaries, func(i, j int) bool {
			if summaries[i].StartedAtMS != summaries[j].StartedAtMS {
				return summaries[i].StartedAtMS > summaries[j].StartedAtMS
			}
			return summaries[i].TraceID < summaries[j].TraceID
		})
		result := store.Paginate(summaries, store.Page{Limit: limit, Cursor: cursor})
		page := pageContract(limit, result.NextCursor)
		return Outcome{Kind: "TraceList", Data: result.Items, Page: page}, nil
	}
}

func getTrace(spans store.SpanStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		traceID := path["id"]
		rows, err := spans.QuerySpans(ctx, &runtime.SpanFilter{TraceID: traceID}, store.Page{Limit: 1000})
		if err != nil {
			return Outcome{Kind: "Trace"}, err
		}
		root, err := tracetree.BuildTree(rows.Items)
		if err != nil {
			return Outcome{Kind: "Trace"}, err
		}
		rootSpanID := ""
		if root != nil && root.Span != nil {
			rootSpanID = root.Span.RootSpanID
			if rootSpanID == "" {
				rootSpanID = root.Span.ID
			}
		}
		return Outcome{Kind: "Trace", Data: traceView{TraceID: traceID, RootSpanID: rootSpanID, Root: root}}, nil
	}
}

func exportTraceHandler(spans store.SpanStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceID := r.PathValue("id")
		rows, err := spans.QuerySpans(r.Context(), &runtime.SpanFilter{TraceID: traceID}, store.Page{Limit: 1000})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server.internal", err.Error(), nil)
			return
		}
		root, err := tracetree.BuildTree(rows.Items)
		if err != nil {
			writeError(w, http.StatusBadRequest, "trace.invalid", err.Error(), nil)
			return
		}

		format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = "jsonl"
		}

		var body []byte
		var contentType string
		switch format {
		case "jsonl":
			body, err = tracexport.JSONL(rows.Items)
			contentType = "application/x-ndjson"
		case "mermaid":
			body, err = tracexport.Mermaid(root)
			contentType = "text/vnd.mermaid; charset=utf-8"
		case "dot":
			body, err = tracexport.DOT(root)
			contentType = "text/vnd.graphviz; charset=utf-8"
		case "otlp":
			req, convErr := tracexport.OTLP(root)
			if convErr != nil {
				err = convErr
				break
			}
			if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "json") {
				body, err = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(req)
				contentType = "application/json"
			} else {
				body, err = proto.Marshal(req)
				contentType = "application/x-protobuf"
			}
		case "parquet":
			body, err = tracexport.Parquet(rows.Items)
			contentType = "application/vnd.apache.parquet"
		default:
			writeError(w, http.StatusBadRequest, "request.invalid", "unsupported export format", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "server.internal", err.Error(), nil)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func ingestTraceHandler(spans store.SpanStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "request.invalid", "invalid request body", nil)
			return
		}
		req := &coltracev1.ExportTraceServiceRequest{}
		switch {
		case strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json"):
			err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(body, req)
		default:
			err = proto.Unmarshal(body, req)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "request.invalid", err.Error(), nil)
			return
		}

		count := 0
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					row := otlpSpanToRow(span)
					if row == nil {
						continue
					}
					if err := spans.OpenSpan(r.Context(), row); err != nil {
						writeError(w, http.StatusInternalServerError, "server.internal", err.Error(), nil)
						return
					}
					count++
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"accepted":%d}`, count)))
	}
}

func summarizeTraces(rows []*runtime.SpanRow) []traceSummary {
	type aggregate struct {
		summary  traceSummary
		started  int64
		ended    int64
		cost     money.Amount
		haveCost bool
	}

	aggs := map[string]*aggregate{}
	for _, row := range rows {
		if row == nil || row.TraceID == "" {
			continue
		}
		agg := aggs[row.TraceID]
		if agg == nil {
			agg = &aggregate{
				summary: traceSummary{
					TraceID:    row.TraceID,
					RootSpanID: row.RootSpanID,
				},
				started: row.StartedAt,
				ended:   row.StartedAt,
			}
			aggs[row.TraceID] = agg
		}
		agg.summary.SpanCount++
		if row.StartedAt < agg.started {
			agg.started = row.StartedAt
		}
		if end := endedAtOrDefault(row); end > agg.ended {
			agg.ended = end
		}
		if row.RootSpanID != "" {
			agg.summary.RootSpanID = row.RootSpanID
		}
		if row.Cost != nil {
			if !agg.haveCost {
				agg.cost = *row.Cost
				agg.haveCost = true
			} else {
				next, _ := agg.cost.Add(*row.Cost)
				agg.cost = next
			}
		}
	}

	out := make([]traceSummary, 0, len(aggs))
	for _, agg := range aggs {
		agg.summary.StartedAtMS = agg.started
		agg.summary.StartedAt = isoFromMS(agg.started)
		agg.summary.EndedAtMS = agg.ended
		agg.summary.EndedAt = isoFromMS(agg.ended)
		agg.summary.DurationMs = agg.ended - agg.started
		if agg.haveCost {
			agg.summary.TotalCost = &contract.Money{Amount: agg.cost.String(), Currency: defaultCurrency}
		}
		out = append(out, agg.summary)
	}
	return out
}

func querySpansForTraces(ctx context.Context, spans store.SpanStore, filter *runtime.SpanFilter, limit int) ([]*runtime.SpanRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	fetchLimit := limit * 25
	if fetchLimit < 1000 {
		fetchLimit = 1000
	}
	if fetchLimit > 5000 {
		fetchLimit = 5000
	}
	result, err := spans.QuerySpans(ctx, filter, store.Page{Limit: fetchLimit})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func spanFilterFromQuery(query url.Values) *runtime.SpanFilter {
	return &runtime.SpanFilter{
		ProjectID: first(query, "project_id"),
		EnvID:     first(query, "env_id"),
		AgentID:   first(query, "agent_id"),
		Connector: first(query, "connector"),
		Model:     first(query, "model"),
		Kind:      first(query, "kind"),
	}
}

func first(query url.Values, key string) string {
	values := query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func endedAtOrDefault(row *runtime.SpanRow) int64 {
	if row.EndedAt != nil {
		return *row.EndedAt
	}
	return row.StartedAt + row.DurationMs
}

func otlpSpanToRow(span *tracev1.Span) *runtime.SpanRow {
	if span == nil {
		return nil
	}
	row := &runtime.SpanRow{
		ID:         hexToString(span.SpanId),
		RunID:      hexToString(span.TraceId),
		ActionID:   hexToString(span.SpanId),
		Name:       span.Name,
		StartedAt:  int64(span.StartTimeUnixNano / 1_000_000),
		TraceID:    hexToString(span.TraceId),
		RootSpanID: hexToString(span.SpanId),
		CreatedAt:  int64(span.EndTimeUnixNano / 1_000_000),
	}
	if span.EndTimeUnixNano > span.StartTimeUnixNano {
		row.DurationMs = int64((span.EndTimeUnixNano - span.StartTimeUnixNano) / 1_000_000)
	}
	if span.ParentSpanId != nil && len(span.ParentSpanId) > 0 {
		parent := hexToString(span.ParentSpanId)
		row.ParentID = &parent
	}
	row.Source = "otel_import"
	row.Kind = "action"
	if span.Kind == tracev1.Span_SPAN_KIND_CLIENT {
		row.Kind = "tool"
	}
	for _, attr := range span.Attributes {
		if attr == nil {
			continue
		}
		switch attr.Key {
		case "project_id":
			row.ProjectID = attrValueString(attr.Value)
		case "env_id":
			row.EnvID = attrValueString(attr.Value)
		case "agent_id":
			row.AgentID = attrValueString(attr.Value)
		case "connector":
			row.Connector = attrValueString(attr.Value)
		default:
			if strings.HasPrefix(attr.Key, "gen_ai.") {
				row.Kind = "llm"
			}
		}
	}
	if row.Connector == "" {
		row.Connector = "otel"
	}
	return row
}

func hexToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return strings.ToLower(fmt.Sprintf("%x", b))
}

func attrValueString(v *commonv1.AnyValue) string {
	if v == nil {
		return ""
	}
	switch x := v.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	default:
		return ""
	}
}
