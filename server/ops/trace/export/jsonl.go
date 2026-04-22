package export

import (
	"bytes"
	"encoding/json"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
)

// JSONL encodes spans as newline-delimited JSON.
func JSONL(spans []*runtimemodel.SpanRow) ([]byte, error) {
	var buf bytes.Buffer
	for i, span := range spans {
		if span == nil {
			continue
		}
		raw, err := json.Marshal(span)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.Write(raw)
	}
	return buf.Bytes(), nil
}
