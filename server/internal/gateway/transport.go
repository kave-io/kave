package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/kave-io/kave/connectors/runtime"
)

type HTTPTransport struct {
	client *http.Client
}

func NewHTTPTransport() *HTTPTransport {
	return &HTTPTransport{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (t *HTTPTransport) Do(ctx context.Context, req *runtime.PreparedRequest) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	httpReq.Header = runtime.CloneHeader(req.Header)
	return t.client.Do(httpReq)
}
