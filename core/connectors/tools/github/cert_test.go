package github

import (
	"net/http"
	"testing"

	"github.com/kave-io/kave/core/connectors/cert"
	connruntime "github.com/kave-io/kave/core/connectors/runtime"
)

func TestCertSuite(t *testing.T) {
	conn := NewConnector(nil)

	okBody := []byte(`{"id":1,"name":"kave","full_name":"kave-io/kave"}`)
	errorBody := []byte(`{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`)

	header := func() http.Header {
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		return h
	}

	cert.RunTool(t, cert.ToolSpec{
		Name:       "github",
		Connector:  conn,
		Descriptor: conn,
		GoldenDir:  "testdata/cert",
		Cases: []cert.ToolCase{
			{
				Name: "get_repo",
				Call: &connruntime.ToolCallRequest{
					Connector:    "github",
					Method:       "rest.read",
					HTTPMethod:   http.MethodGet,
					UpstreamPath: "/repos/kave-io/kave",
					Header:       header(),
				},
				Credential:        "ghp-test-token",
				ExpectURLContains: "api.github.com/repos/kave-io/kave",
				ResponseBody:      okBody,
				ResponseType:      "application/json",
			},
			{
				Name: "post_issue",
				Call: &connruntime.ToolCallRequest{
					Connector:    "github",
					Method:       "rest.write",
					HTTPMethod:   http.MethodPost,
					UpstreamPath: "/repos/kave-io/kave/issues",
					Header:       header(),
					Body:         []byte(`{"title":"bug","body":"repro steps"}`),
				},
				Credential:        "ghp-test-token",
				ExpectURLContains: "api.github.com/repos/kave-io/kave/issues",
				ResponseBody:      okBody,
				ResponseType:      "application/json",
			},
			{
				Name: "upstream_error",
				Call: &connruntime.ToolCallRequest{
					Connector:    "github",
					Method:       "rest.read",
					HTTPMethod:   http.MethodGet,
					UpstreamPath: "/repos/nonexistent/repo",
					Header:       header(),
				},
				Credential:   "ghp-test-token",
				ResponseBody: errorBody,
				ResponseType: "application/json",
				// ParseToolResponse must not error on an upstream error body.
			},
		},
	})
}
