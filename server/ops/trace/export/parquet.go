package export

import (
	"bytes"
	"encoding/json"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
)

// Parquet encodes spans as a parquet file.
func Parquet(spans []*runtimemodel.SpanRow) ([]byte, error) {
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String},
		{Name: "project_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "env_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "agent_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "run_id", Type: arrow.BinaryTypes.String},
		{Name: "action_id", Type: arrow.BinaryTypes.String},
		{Name: "parent_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "name", Type: arrow.BinaryTypes.String},
		{Name: "kind", Type: arrow.BinaryTypes.String},
		{Name: "source", Type: arrow.BinaryTypes.String},
		{Name: "connector", Type: arrow.BinaryTypes.String},
		{Name: "started_at", Type: arrow.PrimitiveTypes.Int64},
		{Name: "ended_at", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "duration_ms", Type: arrow.PrimitiveTypes.Int64},
		{Name: "input", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "output", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "attrs", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "error", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "input_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "output_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "cache_read_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "cache_write_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "reasoning_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "audio_input_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "audio_output_tokens", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "image_units", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "request_count", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "compute_ms", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "storage_bytes", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "bandwidth_bytes", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "model", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "cost_nanos", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "price_version", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "price_snapshot", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "trace_id", Type: arrow.BinaryTypes.String},
		{Name: "root_span_id", Type: arrow.BinaryTypes.String},
		{Name: "validation_meta", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "created_at", Type: arrow.PrimitiveTypes.Int64},
	}, nil)

	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()

	for _, span := range spans {
		appendTraceRow(builder, span)
	}

	rec := builder.NewRecord()
	defer rec.Release()

	table := array.NewTableFromRecords(schema, []arrow.Record{rec})
	defer table.Release()

	var buf bytes.Buffer
	if err := pqarrow.WriteTable(table, &buf, table.NumRows(), nil, pqarrow.NewArrowWriterProperties(pqarrow.WithAllocator(memory.DefaultAllocator))); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func appendTraceRow(builder *array.RecordBuilder, span *runtimemodel.SpanRow) {
	setString := func(idx int, v string) {
		if v == "" {
			builder.Field(idx).(*array.StringBuilder).AppendNull()
			return
		}
		builder.Field(idx).(*array.StringBuilder).Append(v)
	}
	setStringPtr := func(idx int, v *string) {
		if v == nil {
			builder.Field(idx).(*array.StringBuilder).AppendNull()
			return
		}
		builder.Field(idx).(*array.StringBuilder).Append(*v)
	}
	setBytes := func(idx int, v *[]byte) {
		if v == nil {
			builder.Field(idx).(*array.BinaryBuilder).AppendNull()
			return
		}
		builder.Field(idx).(*array.BinaryBuilder).Append(*v)
	}
	setInt32 := func(idx int, v *int) {
		if v == nil {
			builder.Field(idx).(*array.Int32Builder).AppendNull()
			return
		}
		builder.Field(idx).(*array.Int32Builder).Append(int32(*v))
	}
	setInt64 := func(idx int, v *int64) {
		if v == nil {
			builder.Field(idx).(*array.Int64Builder).AppendNull()
			return
		}
		builder.Field(idx).(*array.Int64Builder).Append(*v)
	}

	setString(0, span.ID)
	setString(1, span.ProjectID)
	setString(2, span.EnvID)
	setString(3, span.AgentID)
	setString(4, span.RunID)
	setString(5, span.ActionID)
	setStringPtr(6, span.ParentID)
	setString(7, span.Name)
	setString(8, span.Kind)
	setString(9, span.Source)
	setString(10, span.Connector)
	builder.Field(11).(*array.Int64Builder).Append(span.StartedAt)
	if span.EndedAt == nil {
		builder.Field(12).(*array.Int64Builder).AppendNull()
	} else {
		builder.Field(12).(*array.Int64Builder).Append(*span.EndedAt)
	}
	builder.Field(13).(*array.Int64Builder).Append(span.DurationMs)
	setBytes(14, span.Input)
	setBytes(15, span.Output)
	setBytes(16, span.Attrs)
	setStringPtr(17, span.Error)
	setInt32(18, span.InputTokens)
	setInt32(19, span.OutputTokens)
	setInt32(20, span.CacheReadTokens)
	setInt32(21, span.CacheWriteTokens)
	setInt32(22, span.ReasoningTokens)
	setInt32(23, span.AudioInputTokens)
	setInt32(24, span.AudioOutputTokens)
	setInt32(25, span.ImageUnits)
	setInt32(26, span.RequestCount)
	setInt64(27, span.ComputeMs)
	setInt64(28, span.StorageBytes)
	setInt64(29, span.BandwidthBytes)
	setStringPtr(30, span.Model)
	if span.Cost == nil {
		builder.Field(31).(*array.Int64Builder).AppendNull()
	} else {
		builder.Field(31).(*array.Int64Builder).Append(span.Cost.Nano())
	}
	setStringPtr(32, span.PriceVersion)
	if span.PriceSnapshot == nil {
		builder.Field(33).(*array.BinaryBuilder).AppendNull()
	} else {
		raw, _ := json.Marshal(span.PriceSnapshot)
		builder.Field(33).(*array.BinaryBuilder).Append(raw)
	}
	setString(34, span.TraceID)
	setString(35, span.RootSpanID)
	if len(span.ValidationMeta) == 0 {
		builder.Field(36).(*array.BinaryBuilder).AppendNull()
	} else {
		setBytes(36, &span.ValidationMeta)
	}
	builder.Field(37).(*array.Int64Builder).Append(span.CreatedAt)
}
