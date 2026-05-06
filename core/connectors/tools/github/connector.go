package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kave-io/kave/core/connectors"
	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/ids"
	coreruntime "github.com/kave-io/kave/core/runtime"
)

const defaultBase = "https://api.github.com"

type Config struct {
	BaseURL string
}

type Connector struct {
	baseURL string
}

func NewConnector(cfg *Config) *Connector {
	base := defaultBase
	if cfg != nil && strings.TrimSpace(cfg.BaseURL) != "" {
		base = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	}
	return &Connector{baseURL: base}
}

func (c *Connector) Name() string { return "github" }

func (c *Connector) Intercept(ctx context.Context, action *coreruntime.Action, next connectors.Handler) (*pipeline.Result, error) {
	if action.Connector != "github" {
		return nil, fmt.Errorf("github: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		Kind:             connectors.KindTool,
		SupportedActions: []coreruntime.ActionType{coreruntime.TypeTool},
		SupportedMethods: []string{"rest.read", "rest.write"},
		SupportedRoutes:  []string{"/v1/tools/github/*"},
		RequiresAuth:     true,
		CanProxy:         true,
		StreamSupport:    false,
		APIVersion:       "rest",
	}
}

func (c *Connector) ParseToolRequest(req *runtime.Request) (*runtime.ToolCallRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("github: nil request")
	}
	upstreamPath := strings.TrimPrefix(req.Path, "/v1/tools/github")
	if upstreamPath == "" || upstreamPath == "/" {
		return nil, fmt.Errorf("github: missing upstream path")
	}
	if !strings.HasPrefix(upstreamPath, "/") {
		upstreamPath = "/" + upstreamPath
	}
	method := "rest.write"
	if req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions {
		method = "rest.read"
	}

	inputCopy := make([]byte, len(req.Body))
	copy(inputCopy, req.Body)

	return &runtime.ToolCallRequest{
		Connector:    "github",
		Method:       method,
		HTTPMethod:   req.Method,
		UpstreamPath: upstreamPath,
		RawQuery:     req.RawQuery,
		Header:       runtime.CloneHeader(req.Header),
		Body:         req.Body,
		Action: &coreruntime.Action{
			Invocation: coreruntime.Invocation{
				InvocationRef: coreruntime.InvocationRef{ID: ids.New("act")},
				InvocationTarget: coreruntime.InvocationTarget{
					Type:      coreruntime.TypeTool,
					Connector: "github",
					Method:    method,
				},
				InvocationData: coreruntime.InvocationData{Input: &inputCopy},
			},
			Status: coreruntime.StatusPending,
		},
	}, nil
}

func (c *Connector) PrepareToolRequest(call *runtime.ToolCallRequest, credential string) (*runtime.PreparedRequest, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	base.Path = singleSlashJoin(base.Path, call.UpstreamPath)
	base.RawQuery = call.RawQuery

	headers := runtime.CloneHeader(call.Header)
	stripHopHeaders(headers)
	headers.Set("Accept", defaultHeader(headers.Get("Accept"), "application/vnd.github+json"))
	headers.Set("X-GitHub-Api-Version", defaultHeader(headers.Get("X-GitHub-Api-Version"), "2022-11-28"))
	if credential != "" {
		headers.Set("Authorization", "Bearer "+strings.TrimPrefix(credential, "Bearer "))
	}

	return &runtime.PreparedRequest{
		Method: call.HTTPMethod,
		URL:    base.String(),
		Header: headers,
		Body:   call.Body,
	}, nil
}

func (c *Connector) ParseToolResponse(body []byte, _ string) (*pipeline.Result, error) {
	return &pipeline.Result{
		Body: body,
		Usage: &coreruntime.Usage{
			RequestCount: 1,
		},
	}, nil
}

func (c *Connector) RequiresAuth() bool { return true }

func singleSlashJoin(basePath, path string) string {
	basePath = strings.TrimRight(basePath, "/")
	path = "/" + strings.TrimLeft(path, "/")
	if basePath == "" {
		return path
	}
	return basePath + path
}

func stripHopHeaders(headers http.Header) {
	for _, key := range []string{
		"Authorization",
		"Connection",
		"Transfer-Encoding",
		"Accept-Encoding",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailer",
		"Upgrade",
	} {
		headers.Del(key)
	}
}

func defaultHeader(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
