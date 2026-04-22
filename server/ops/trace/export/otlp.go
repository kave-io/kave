package export

import (
	"encoding/hex"
	"fmt"
	"strings"

	coltracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/server/ops/trace"
	"google.golang.org/protobuf/proto"
)

// OTLP converts a trace tree to an OTLP ExportTraceServiceRequest.
func OTLP(root *trace.Node) (*coltracev1.ExportTraceServiceRequest, error) {
	if root == nil || root.Span == nil {
		return nil, fmt.Errorf("trace: empty trace tree")
	}

	spans := make([]*tracev1.Span, 0)
	var walk func(parent *runtimemodel.SpanRow, n *trace.Node)
	walk = func(parent *runtimemodel.SpanRow, n *trace.Node) {
		if n == nil || n.Span == nil {
			return
		}
		spans = append(spans, spanToOTLP(parent, n.Span))
		for _, child := range n.Children {
			walk(n.Span, child)
		}
	}
	walk(nil, root)

	return &coltracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{
							Key:   "service.name",
							Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "kave"}},
						},
					},
				},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Scope: &commonv1.InstrumentationScope{Name: "kave.trace.export"},
						Spans: spans,
					},
				},
			},
		},
	}, nil
}

func spanToOTLP(_ *runtimemodel.SpanRow, row *runtimemodel.SpanRow) *tracev1.Span {
	attrs := make([]*commonv1.KeyValue, 0, 8)
	for _, kv := range []*commonv1.KeyValue{
		stringAttr("project_id", row.ProjectID),
		stringAttr("env_id", row.EnvID),
		stringAttr("agent_id", row.AgentID),
		stringAttr("run_id", row.RunID),
		stringAttr("action_id", row.ActionID),
		stringAttr("kind", row.Kind),
		stringAttr("source", row.Source),
		stringAttr("connector", row.Connector),
	} {
		if kv != nil {
			attrs = append(attrs, kv)
		}
	}

	span := &tracev1.Span{
		TraceId:           hexBytes(row.TraceID, 16),
		SpanId:            hexBytes(row.ID, 8),
		Name:              row.Name,
		StartTimeUnixNano: uint64(row.StartedAt) * 1_000_000,
		EndTimeUnixNano:   uint64(valueOrZero(row.EndedAt, row.StartedAt)) * 1_000_000,
		Attributes:        attrs,
	}
	if row.ParentID != nil && *row.ParentID != "" {
		span.ParentSpanId = hexBytes(*row.ParentID, 8)
	}
	if row.Error != nil {
		span.Status = &tracev1.Status{
			Code:    tracev1.Status_STATUS_CODE_ERROR,
			Message: *row.Error,
		}
	}
	switch strings.ToLower(row.Kind) {
	case "llm":
		span.Kind = tracev1.Span_SPAN_KIND_CLIENT
	case "tool":
		span.Kind = tracev1.Span_SPAN_KIND_CLIENT
	default:
		span.Kind = tracev1.Span_SPAN_KIND_INTERNAL
	}
	return span
}

func hexBytes(v string, expected int) []byte {
	if v == "" {
		return make([]byte, expected)
	}
	out, err := hex.DecodeString(v)
	if err != nil {
		return make([]byte, expected)
	}
	if len(out) < expected {
		padded := make([]byte, expected)
		copy(padded[expected-len(out):], out)
		return padded
	}
	if len(out) > expected {
		return out[len(out)-expected:]
	}
	return out
}

func stringAttr(key, value string) *commonv1.KeyValue {
	if value == "" {
		return nil
	}
	return &commonv1.KeyValue{
		Key:   key,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}},
	}
}

func valueOrZero(v *int64, fallback int64) int64 {
	if v == nil {
		return fallback
	}
	return *v
}

var _ proto.Message = (*coltracev1.ExportTraceServiceRequest)(nil)
