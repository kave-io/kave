package mappers

import (
	"encoding/json"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/ops/trace"
	"github.com/kave-io/kave/core/pkg/money"
)

// SpanRowOptions carries optional storage-only fields that do not live on trace.Span.
type SpanRowOptions struct {
	Input               *[]byte
	Output              *[]byte
	Attrs               *map[string]trace.AttrVal
	CacheReadTokens     *int
	CacheWriteTokens    *int
	ReasoningTokens     *int
	AudioInputTokens    *int
	AudioOutputTokens   *int
	ImageUnits          *int
	RequestCount        *int
	ComputeMs           *int64
	StorageBytes        *int64
	BandwidthBytes      *int64
	PriceVersion        *string
	PriceSnapshot       *runtimemodel.PriceSnapshot
	TraceID             string
	RootSpanID          string
	ValidationMeta      *trace.ValidationMeta
	CreatedAt           *int64
}

// SpanEndOptions carries optional finalization fields that do not live on trace.Span.
type SpanEndOptions struct {
	Output              *[]byte
	Attrs               *map[string]trace.AttrVal
	CacheReadTokens     *int
	CacheWriteTokens    *int
	ReasoningTokens     *int
	AudioInputTokens    *int
	AudioOutputTokens   *int
	ImageUnits          *int
	RequestCount        *int
	ComputeMs           *int64
	StorageBytes        *int64
	BandwidthBytes      *int64
	PriceVersion        *string
	PriceSnapshot       *runtimemodel.PriceSnapshot
	TraceID             string
	RootSpanID          string
	ValidationMeta      *trace.ValidationMeta
}

// SpanToRow converts a trace span to its persisted row representation.
func SpanToRow(s *trace.Span, opts *SpanRowOptions) *runtimemodel.SpanRow {
	if s == nil {
		return nil
	}

	var createdAt int64
	if opts != nil && opts.CreatedAt != nil {
		createdAt = *opts.CreatedAt
	} else {
		createdAt = msSinceEpoch()
	}

	attrs := s.Attrs
	var input, output *[]byte
	var cacheReadTokens, cacheWriteTokens *int
	var reasoningTokens, audioInputTokens, audioOutputTokens, imageUnits *int
	var requestCount *int
	var computeMs, storageBytes, bandwidthBytes *int64
	var priceVersion *string
	var priceSnapshot *runtimemodel.PriceSnapshot
	var traceID, rootSpanID string
	var validationMeta *trace.ValidationMeta
	if opts != nil {
		if opts.Attrs != nil {
			attrs = *opts.Attrs
		}
		input = opts.Input
		output = opts.Output
		cacheReadTokens = opts.CacheReadTokens
		cacheWriteTokens = opts.CacheWriteTokens
		reasoningTokens = opts.ReasoningTokens
		audioInputTokens = opts.AudioInputTokens
		audioOutputTokens = opts.AudioOutputTokens
		imageUnits = opts.ImageUnits
		requestCount = opts.RequestCount
		computeMs = opts.ComputeMs
		storageBytes = opts.StorageBytes
		bandwidthBytes = opts.BandwidthBytes
		priceVersion = opts.PriceVersion
		priceSnapshot = opts.PriceSnapshot
		traceID = opts.TraceID
		rootSpanID = opts.RootSpanID
		validationMeta = opts.ValidationMeta
	}

	// Use TraceID/RootSpanID from span if not overridden by opts
	if traceID == "" {
		traceID = s.TraceID
	}
	if rootSpanID == "" {
		rootSpanID = s.RootSpanID
	}
	if validationMeta == nil {
		validationMeta = s.ValidationMeta
	}

	duration := s.DurationMS
	if duration == 0 {
		duration = s.ComputeDuration()
	}

	// Convert money.Amount from trace span (nil-safe pointer)
	var cost *money.Amount
	if s.Cost != nil {
		cost = s.Cost
	}

	return &runtimemodel.SpanRow{
		ID:                  s.ID,
		ProjectID:           s.ProjectID,
		EnvID:               s.EnvID,
		AgentID:             s.AgentID,
		RunID:               s.RunID,
		ActionID:            s.ActionID,
		ParentID:            s.ParentID,
		Name:                s.Name,
		Kind:                string(s.Kind),
		Source:              string(s.Source),
		Connector:           s.Connector,
		StartedAt:           timingToMS(s.StartedAt),
		EndedAt:             ptrMSFromTiming(s.EndedAt),
		DurationMs:          duration,
		Input:               input,
		Output:              output,
		Attrs:               encodeAttrs(attrs),
		Error:               s.Error,
		InputTokens:         s.InputTokens,
		OutputTokens:        s.OutputTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheWriteTokens:    cacheWriteTokens,
		ReasoningTokens:     reasoningTokens,
		AudioInputTokens:    audioInputTokens,
		AudioOutputTokens:   audioOutputTokens,
		ImageUnits:          imageUnits,
		RequestCount:        requestCount,
		ComputeMs:           computeMs,
		StorageBytes:        storageBytes,
		BandwidthBytes:      bandwidthBytes,
		Model:               s.Model,
		Cost:                cost,
		PriceVersion:        priceVersion,
		PriceSnapshot:       priceSnapshot,
		TraceID:             traceID,
		RootSpanID:          rootSpanID,
		ValidationMeta:      encodeValidationMeta(validationMeta),
		CreatedAt:           createdAt,
	}
}

// RowToSpan converts a persisted span row to a trace span.
func RowToSpan(r *runtimemodel.SpanRow) *trace.Span {
	if r == nil {
		return nil
	}

	return &trace.Span{
		ID:             r.ID,
		ProjectID:      r.ProjectID,
		EnvID:          r.EnvID,
		AgentID:        r.AgentID,
		RunID:          r.RunID,
		ActionID:       r.ActionID,
		ParentID:       r.ParentID,
		Name:           r.Name,
		Kind:           trace.SpanKind(r.Kind),
		Source:         trace.SpanSource(r.Source),
		Connector:      r.Connector,
		Model:          r.Model,
		StartedAt:      msToTimingValue(r.StartedAt),
		EndedAt:        ptrMSToTimingValue(r.EndedAt),
		DurationMS:     r.DurationMs,
		InputTokens:    r.InputTokens,
		OutputTokens:   r.OutputTokens,
		Cost:           r.Cost,
		Error:          r.Error,
		Attrs:          decodeAttrs(r.Attrs),
		TraceID:        r.TraceID,
		RootSpanID:     r.RootSpanID,
		ValidationMeta: decodeValidationMeta(r.ValidationMeta),
	}
}

// SpanToEnd converts a completed span to an end/update payload for stores.
func SpanToEnd(s *trace.Span, opts *SpanEndOptions) *runtimemodel.SpanEnd {
	if s == nil {
		return nil
	}

	attrs := s.Attrs
	var output *[]byte
	var cacheReadTokens, cacheWriteTokens *int
	var reasoningTokens, audioInputTokens, audioOutputTokens, imageUnits *int
	var requestCount *int
	var computeMs, storageBytes, bandwidthBytes *int64
	var priceVersion *string
	var priceSnapshot *runtimemodel.PriceSnapshot
	var traceID, rootSpanID string
	var validationMeta *trace.ValidationMeta
	if opts != nil {
		if opts.Attrs != nil {
			attrs = *opts.Attrs
		}
		output = opts.Output
		cacheReadTokens = opts.CacheReadTokens
		cacheWriteTokens = opts.CacheWriteTokens
		reasoningTokens = opts.ReasoningTokens
		audioInputTokens = opts.AudioInputTokens
		audioOutputTokens = opts.AudioOutputTokens
		imageUnits = opts.ImageUnits
		requestCount = opts.RequestCount
		computeMs = opts.ComputeMs
		storageBytes = opts.StorageBytes
		bandwidthBytes = opts.BandwidthBytes
		priceVersion = opts.PriceVersion
		priceSnapshot = opts.PriceSnapshot
		traceID = opts.TraceID
		rootSpanID = opts.RootSpanID
		validationMeta = opts.ValidationMeta
	}

	// Use TraceID/RootSpanID from span if not overridden by opts
	if traceID == "" {
		traceID = s.TraceID
	}
	if rootSpanID == "" {
		rootSpanID = s.RootSpanID
	}
	if validationMeta == nil {
		validationMeta = s.ValidationMeta
	}

	duration := s.DurationMS
	if duration == 0 {
		duration = s.ComputeDuration()
	}

	return &runtimemodel.SpanEnd{
		EndedAt:             ptrMSFromTiming(s.EndedAt),
		DurationMs:          duration,
		Output:              output,
		Attrs:               encodeAttrs(attrs),
		Error:               s.Error,
		InputTokens:         s.InputTokens,
		OutputTokens:        s.OutputTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheWriteTokens:    cacheWriteTokens,
		ReasoningTokens:     reasoningTokens,
		AudioInputTokens:    audioInputTokens,
		AudioOutputTokens:   audioOutputTokens,
		ImageUnits:          imageUnits,
		RequestCount:        requestCount,
		ComputeMs:           computeMs,
		StorageBytes:        storageBytes,
		BandwidthBytes:      bandwidthBytes,
		Model:               s.Model,
		Cost:                s.Cost,
		PriceVersion:        priceVersion,
		PriceSnapshot:       priceSnapshot,
		TraceID:             traceID,
		RootSpanID:          rootSpanID,
		ValidationMeta:      encodeValidationMeta(validationMeta),
	}
}

func encodeAttrs(attrs map[string]trace.AttrVal) *[]byte {
	if len(attrs) == 0 {
		return nil
	}
	b, err := json.Marshal(attrs)
	if err != nil {
		return nil
	}
	return &b
}

func decodeAttrs(attrs *[]byte) map[string]trace.AttrVal {
	if attrs == nil || len(*attrs) == 0 {
		return nil
	}
	out := make(map[string]trace.AttrVal)
	if err := json.Unmarshal(*attrs, &out); err != nil {
		return nil
	}
	return out
}

func encodeValidationMeta(meta *trace.ValidationMeta) []byte {
	if meta == nil {
		return nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	return b
}

func decodeValidationMeta(data []byte) *trace.ValidationMeta {
	if len(data) == 0 {
		return nil
	}
	var meta trace.ValidationMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}
