package github

import (
	"net/http"
	"testing"

	"github.com/kave-io/kave/core/connectors/runtime"
)

func TestParseToolRequestClassifiesReadAndWrite(t *testing.T) {
	conn := NewConnector(nil)

	read, err := conn.ParseToolRequest(&runtime.Request{
		Method: http.MethodGet,
		Path:   "/v1/tools/github/repos/kave-io/kave",
		Header: http.Header{},
	})
	if err != nil {
		t.Fatalf("ParseToolRequest read: %v", err)
	}
	if read.Method != "rest.read" || read.HTTPMethod != http.MethodGet {
		t.Fatalf("read method = %s/%s", read.Method, read.HTTPMethod)
	}
	if read.Action == nil || read.Action.Connector != "github" || string(read.Action.Type) != "tool" {
		t.Fatalf("read action = %#v", read.Action)
	}

	write, err := conn.ParseToolRequest(&runtime.Request{
		Method: http.MethodPost,
		Path:   "/v1/tools/github/repos/kave-io/kave/issues",
		Header: http.Header{},
		Body:   []byte(`{"title":"bug"}`),
	})
	if err != nil {
		t.Fatalf("ParseToolRequest write: %v", err)
	}
	if write.Method != "rest.write" || write.HTTPMethod != http.MethodPost {
		t.Fatalf("write method = %s/%s", write.Method, write.HTTPMethod)
	}
	if string(*write.Action.Input) != `{"title":"bug"}` {
		t.Fatalf("input = %q", string(*write.Action.Input))
	}
}

func TestPrepareToolRequestStripsCallerAuth(t *testing.T) {
	conn := NewConnector(&Config{BaseURL: "https://github.example/api"})
	call := &runtime.ToolCallRequest{
		HTTPMethod:   http.MethodGet,
		UpstreamPath: "/repos/kave-io/kave",
		RawQuery:     "per_page=1",
		Header:       http.Header{"Authorization": {"Bearer caller"}, "Accept": {"application/json"}},
	}

	req, err := conn.PrepareToolRequest(call, "github-token")
	if err != nil {
		t.Fatalf("PrepareToolRequest: %v", err)
	}
	if req.URL != "https://github.example/api/repos/kave-io/kave?per_page=1" {
		t.Fatalf("url = %q", req.URL)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer github-token" {
		t.Fatalf("authorization = %q", got)
	}
	if got := req.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Fatalf("api version = %q", got)
	}
}
