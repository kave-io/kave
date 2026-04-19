package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kave-io/kave/connectors/runtime"
	"github.com/kave-io/kave/core/pkg/ids"
	coreruntime "github.com/kave-io/kave/core/runtime"
)

const prefix = "/frameworks/claude-code/"

type Connector struct{}

func NewConnector() *Connector {
	return &Connector{}
}

func (c *Connector) Name() string {
	return "claude-code"
}

func (c *Connector) ParseLLMRequest(req *runtime.Request) (*runtime.LLMCall, error) {
	trimmed := strings.TrimPrefix(req.Path, prefix)
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		return nil, fmt.Errorf("invalid claude-code path: missing provider upstream path")
	}

	provider := trimmed[:slash]
	upstreamPath := trimmed[slash:]
	method := actionMethod(upstreamPath, req.Body)

	inputCopy := make([]byte, len(req.Body))
	copy(inputCopy, req.Body)

	return &runtime.LLMCall{
		Provider:     provider,
		Method:       req.Method,
		UpstreamPath: upstreamPath,
		RawQuery:     req.RawQuery,
		Header:       runtime.CloneHeader(req.Header),
		Body:         req.Body,
		Action: &coreruntime.Action{
			Invocation: coreruntime.Invocation{
				InvocationRef: coreruntime.InvocationRef{
					ID: ids.New("act"),
				},
				InvocationTarget: coreruntime.InvocationTarget{
					Type:      coreruntime.TypeLLM,
					Connector: provider,
					Method:    method,
				},
				InvocationData: coreruntime.InvocationData{
					Input: &inputCopy,
				},
			},
			Status: coreruntime.StatusPending,
		},
	}, nil
}

func actionMethod(upstreamPath string, body []byte) string {
	segments := strings.Split(strings.Trim(upstreamPath, "/"), "/")
	method := segments[len(segments)-1]

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return method
	}

	stream, ok := payload["stream"].(bool)
	if ok && stream {
		return method + ".streaming"
	}

	return method
}
