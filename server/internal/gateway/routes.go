package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/kave-io/kave/core/connectors/runtime"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/timex"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
)

func (g *FrameworkGateway) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/openai/", g.handleRaw("openai"))
	mux.HandleFunc("/backend-api/codex/responses", g.handleCodexChatGPTCompatibility)
	mux.HandleFunc("/backend-api/codex/responses/", g.handleCodexChatGPTCompatibility)
	mux.HandleFunc("/backend-api/wham/apps", g.handleChatGPTAppsPassthrough)
	mux.HandleFunc("/backend-api/wham/apps/", g.handleChatGPTAppsPassthrough)
	mux.HandleFunc("/backend-api/codex/wham/apps", g.handleCodexChatGPTAppsPassthrough)
	mux.HandleFunc("/backend-api/codex/wham/apps/", g.handleCodexChatGPTAppsPassthrough)
	mux.HandleFunc("/connectors/directory/list", g.handleChatGPTDirectoryPassthrough)
	mux.HandleFunc("/connectors/directory/list/", g.handleChatGPTDirectoryPassthrough)
	mux.HandleFunc("/connectors/directory/list_workspace", g.handleChatGPTDirectoryPassthrough)
	mux.HandleFunc("/connectors/directory/list_workspace/", g.handleChatGPTDirectoryPassthrough)
	mux.HandleFunc("/v1/tools/github/", g.handleTool("github"))
	mux.HandleFunc("/frameworks/", g.handleFramework)
}

func (g *FrameworkGateway) handleRaw(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/"+provider+"/") {
			writeError(w, http.StatusBadRequest, "gateway.bad_request", "invalid provider route", nil)
			return
		}
		g.handleProxy(w, r, "raw")
	}
}

func (g *FrameworkGateway) handleCodexChatGPTCompatibility(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/backend-api/codex/responses" {
		http.NotFound(w, r)
		return
	}
	g.handleProxy(w, r, "raw")
}

func (g *FrameworkGateway) handleChatGPTAppsPassthrough(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/backend-api/wham/apps" {
		http.NotFound(w, r)
		return
	}
	g.handleChatGPTPassthrough(w, r, "/backend-api/wham/apps")
}

func (g *FrameworkGateway) handleCodexChatGPTAppsPassthrough(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/backend-api/codex/wham/apps" {
		http.NotFound(w, r)
		return
	}
	g.handleChatGPTPassthrough(w, r, "/backend-api/wham/apps")
}

func (g *FrameworkGateway) handleChatGPTDirectoryPassthrough(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/connectors/directory/list", "/connectors/directory/list_workspace":
		g.handleChatGPTPassthrough(w, r, r.URL.Path)
	default:
		http.NotFound(w, r)
	}
}

func (g *FrameworkGateway) handleChatGPTPassthrough(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	upstreamURL, err := chatGPTUpstreamURL(upstreamPath, r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "gateway.bad_upstream", "invalid ChatGPT upstream", nil)
		return
	}

	headers := runtime.CloneHeader(r.Header)
	stripProxyHopHeaders(headers)
	resp, err := g.transport.Do(r.Context(), &runtime.PreparedRequest{
		Method: r.Method,
		URL:    upstreamURL,
		Header: headers,
		Body:   body,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "gateway.upstream_error", fmt.Sprintf("gateway upstream error: transport: %v", err), nil)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if strings.Contains(resp.Header.Get("Content-Type"), "event-stream") {
		flusher, ok := w.(http.Flusher)
		if !ok {
			_, _ = io.Copy(w, resp.Body)
			return
		}
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				flusher.Flush()
			}
			if readErr == io.EOF {
				return
			}
			if readErr != nil {
				return
			}
		}
	}

	_, _ = io.Copy(w, resp.Body)
}

func (g *FrameworkGateway) handleFramework(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/frameworks/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "gateway.bad_request", "invalid framework route", nil)
		return
	}
	g.handleProxy(w, r, parts[0])
}

func (g *FrameworkGateway) handleTool(toolName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/tools/"+toolName+"/") {
			writeError(w, http.StatusBadRequest, "gateway.bad_request", "invalid tool route", nil)
			return
		}
		g.handleToolProxy(w, r, toolName)
	}
}

func (g *FrameworkGateway) handleToolProxy(w http.ResponseWriter, r *http.Request, toolName string) {
	ctx := r.Context()
	tw := &trackingResponseWriter{ResponseWriter: w}

	identity, ok := authctx.From(ctx)
	if !ok {
		identity, _ = serverauth.ParseIdentity(ctx, r.Header.Get("Authorization"), g.app, nil, true)
	}
	if identity.IsInvalid() || identity.IsAnonymous() || identity.IsGuest() {
		writeError(tw, http.StatusUnauthorized, "gateway.unauthorized", "invalid authorization", nil)
		return
	}
	ctx = authctx.With(ctx, identity)
	if identity.AgentID == "" {
		writeError(tw, http.StatusUnauthorized, "gateway.unauthorized", "authenticated identity is not bound to an agent", nil)
		return
	}

	agent, err := g.app.GetAgentByID(ctx, identity.AgentID)
	if err != nil || agent == nil {
		writeError(tw, http.StatusUnauthorized, "gateway.agent_not_found", "agent not found", nil)
		return
	}

	tool, err := g.registry.ResolveTool(toolName)
	if err != nil {
		writeError(tw, http.StatusBadRequest, "gateway.provider_not_supported", err.Error(), nil)
		return
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	call, err := tool.ParseToolRequest(&runtime.Request{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   runtime.CloneHeader(r.Header),
		Body:     body,
	})
	if err != nil {
		writeError(tw, http.StatusBadRequest, "gateway.bad_request", err.Error(), nil)
		return
	}

	now := int64(timex.Now())
	run := &runtimemodel.RunRecord{
		ID:        ids.New("run"),
		ProjectID: agent.ProjectID,
		EnvID:     agent.EnvID,
		AgentID:   agent.ID,
		Name:      toolName + "." + call.Action.InvocationTarget.Method,
		Status:    string(coreruntime.RunActive),
		Metadata:  map[string]any{},
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := g.app.CreateRun(ctx, run); err != nil {
		writeError(tw, http.StatusInternalServerError, "gateway.create_run_failed", "create run failed", nil)
		return
	}

	call.Action.InvocationRef.RunID = run.ID
	call.Action.InvocationRef.AgentID = agent.ID
	call.Action.InvocationRef.ProjectID = agent.ProjectID
	call.Action.InvocationRef.EnvID = agent.EnvID

	if err := g.app.CreateAction(ctx, &runtimemodel.ActionRecord{
		ID:         call.Action.InvocationRef.ID,
		RunID:      run.ID,
		AgentID:    agent.ID,
		ProjectID:  agent.ProjectID,
		EnvID:      agent.EnvID,
		ActionType: string(call.Action.InvocationTarget.Type),
		Connector:  call.Action.InvocationTarget.Connector,
		Method:     call.Action.InvocationTarget.Method,
		Input:      call.Action.InvocationData.Input,
		Status:     string(coreruntime.StatusRunning),
		Source:     string(coreruntime.ActionSourceIntercepted),
		Metadata:   map[string]any{},
		Attempt:    1,
		CreatedAt:  now,
	}); err != nil {
		writeError(tw, http.StatusInternalServerError, "gateway.create_action_failed", "create action failed", nil)
		return
	}

	handler := func(ctx context.Context, action *coreruntime.Action) (*pipeline.Result, error) {
		credential, credErr := g.resolveCredential(ctx, agent.EnvID, toolName)
		if errors.Is(credErr, credresolve.ErrPassthrough) {
			credential = identity.RawAuthorization
		} else if credErr != nil && tool.RequiresAuth() {
			return nil, fmt.Errorf("%w: missing connector credential", serverauth.ErrUnauthenticated)
		}
		prepared, err := tool.PrepareToolRequest(call, credential)
		if err != nil {
			return nil, err
		}
		resp, err := g.transport.Do(ctx, prepared)
		if err != nil {
			return nil, fmt.Errorf("%w: transport: %v", ErrUpstream, err)
		}
		defer resp.Body.Close()
		return g.handleToolBuffered(tw, resp, tool)
	}

	result, err := g.pipeline.Execute(ctx, call.Action, handler)
	g.finishAction(ctx, call.Action, result, err)
	g.finishRun(ctx, run.ID, err)
	if err != nil && result == nil && !tw.WroteHeader() {
		status, code, message, details := mapError(err)
		writeError(tw, status, code, message, details)
	}
}

func (g *FrameworkGateway) handleProxy(w http.ResponseWriter, r *http.Request, framework string) {
	ctx := r.Context()
	tw := &trackingResponseWriter{ResponseWriter: w}
	callerAuthorization := strings.TrimSpace(r.Header.Get("Authorization"))

	identity, ok := authctx.From(ctx)
	if !ok {
		parsed, err := serverauth.ParseIdentity(ctx, callerAuthorization, g.app, nil, true)
		if err != nil && strings.HasPrefix(callerAuthorization, "Bearer ") {
			// Unknown bearer is treated as upstream passthrough auth, not Kave identity auth.
			identity = authctx.Identity{Kind: authctx.KindAnonymous}
		} else {
			identity = parsed
		}
	}
	if identity.IsInvalid() {
		writeError(tw, http.StatusUnauthorized, "gateway.unauthorized", "invalid authorization", nil)
		return
	}
	ctx = authctx.With(ctx, identity)

	// Three-axis auth boundary decision
	var agentID string
	var isGuest bool

	if identity.IsAgentToken() || identity.IsUser() {
		// Authenticated: use the identity's agent
		if identity.AgentID != "" {
			agentID = identity.AgentID
		} else {
			writeError(tw, http.StatusUnauthorized, "gateway.unauthorized", "authenticated identity is not bound to an agent", nil)
			return
		}
	} else if identity.IsAnonymous() {
		// Anonymous: try the three-axis decision
		// Need to parse provider from the path to resolve connector
		provider := g.parseProviderFromPath(r.URL.Path, framework)
		if provider == "" {
			writeError(tw, http.StatusBadRequest, "gateway.bad_request", "cannot determine provider from path", nil)
			return
		}

		connector, err := g.registry.ResolveConnector(provider)
		if err != nil {
			writeError(tw, http.StatusBadRequest, "gateway.provider_not_supported", err.Error(), nil)
			return
		}

		// Parse environment from the context or from request (for now, use default)
		// In a real scenario, this might come from a routing parameter
		env, err := g.app.GetEnvironmentBySlug(ctx, "default", "default")
		if err != nil || env == nil {
			// Fallback: treat as strict
			writeError(tw, http.StatusUnauthorized, "gateway.env_requires_authentication", "environment not found", nil)
			return
		}

		// Axis 1: Environment trust mode
		if env.TrustMode != controlmodel.TrustPermissive {
			writeError(tw, http.StatusUnauthorized, "gateway.env_requires_authentication", "environment requires authentication", nil)
			return
		}

		// Axis 2: Provider auth requirement
		if connector.RequiresAuth() && callerAuthorization == "" {
			writeError(tw, http.StatusUnauthorized, "gateway.provider_requires_authentication", "provider requires authentication", nil)
			return
		}

		// Axis 3: Rate limit guest traffic (10 RPS per connector × remote addr)
		remoteAddr := getRemoteAddr(r)
		limiter := g.getGuestLimiter(provider, remoteAddr)
		if !limiter.Allow() {
			writeError(tw, http.StatusTooManyRequests, "gateway.rate_limited", "too many requests", nil)
			return
		}

		// All three axes permit: create synthetic guest identity
		isGuest = true
		identity = authctx.NewGuest(env.ID, env.ID, provider, getBindScope(r))
		ctx = authctx.With(ctx, identity)
	} else if identity.IsGuest() {
		// Guest identity already set (shouldn't happen at this level)
		isGuest = true
	} else {
		writeError(tw, http.StatusUnauthorized, "gateway.unauthorized", "missing agent token", nil)
		return
	}

	var agent *controlmodel.Agent
	var err error

	// For guests, don't look up an agent (they have zero budget and no agent binding)
	if !isGuest {
		agent, err = g.app.GetAgentByID(ctx, agentID)
		if err != nil || agent == nil {
			writeError(tw, http.StatusUnauthorized, "gateway.agent_not_found", "agent not found", nil)
			return
		}
	} else {
		env, envErr := g.app.GetEnvironmentBySlug(ctx, "default", "default")
		if envErr != nil || env == nil {
			writeError(tw, http.StatusUnauthorized, "gateway.env_requires_authentication", "environment not found", nil)
			return
		}
		agent, err = g.app.GetAgentByName(ctx, env.ID, "default")
		if err != nil || agent == nil {
			writeError(tw, http.StatusUnauthorized, "gateway.agent_not_found", "default guest agent not found", nil)
			return
		}
	}

	family, err := g.registry.Resolve(framework)
	if err != nil {
		writeError(tw, http.StatusBadRequest, "gateway.framework_not_supported", err.Error(), nil)
		return
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	call, err := family.Framework.ParseLLMRequest(&runtime.Request{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   runtime.CloneHeader(r.Header),
		Body:     body,
	})
	if err != nil {
		writeError(tw, http.StatusBadRequest, "gateway.bad_request", err.Error(), nil)
		return
	}

	now := int64(timex.Now())
	run := &runtimemodel.RunRecord{
		ID:        ids.New("run"),
		ProjectID: agent.ProjectID,
		EnvID:     agent.EnvID,
		AgentID:   agent.ID,
		Name:      call.Provider + "." + call.Action.InvocationTarget.Method,
		Status:    string(coreruntime.RunActive),
		Metadata:  map[string]any{},
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := g.app.CreateRun(ctx, run); err != nil {
		writeError(tw, http.StatusInternalServerError, "gateway.create_run_failed", "create run failed", nil)
		return
	}

	call.Action.Invocation.InvocationRef.RunID = run.ID
	call.Action.Invocation.InvocationRef.AgentID = agent.ID
	call.Action.Invocation.InvocationRef.ProjectID = agent.ProjectID
	call.Action.Invocation.InvocationRef.EnvID = agent.EnvID

	if err := g.app.CreateAction(ctx, &runtimemodel.ActionRecord{
		ID:         call.Action.InvocationRef.ID,
		RunID:      run.ID,
		AgentID:    agent.ID,
		ProjectID:  agent.ProjectID,
		EnvID:      agent.EnvID,
		ActionType: string(call.Action.InvocationTarget.Type),
		Connector:  call.Action.InvocationTarget.Connector,
		Method:     call.Action.InvocationTarget.Method,
		Input:      call.Action.InvocationData.Input,
		Status:     string(coreruntime.StatusRunning),
		Source:     string(coreruntime.ActionSourceIntercepted),
		Metadata:   map[string]any{},
		Attempt:    1,
		CreatedAt:  now,
	}); err != nil {
		writeError(tw, http.StatusInternalServerError, "gateway.create_action_failed", "create action failed", nil)
		return
	}

	provider, err := runtime.RequireProvider(family.Providers, call.Provider)
	if err != nil {
		writeError(tw, http.StatusBadRequest, "gateway.provider_not_supported", err.Error(), nil)
		return
	}

	handler := func(ctx context.Context, action *coreruntime.Action) (*pipeline.Result, error) {
		credential := ""
		if (identity.IsAnonymous() || identity.IsGuest()) && callerAuthorization != "" {
			credential = callerAuthorization
		}
		if credential == "" {
			var credErr error
			credential, credErr = g.resolveCredential(ctx, agent.EnvID, call.Provider)
			if errors.Is(credErr, credresolve.ErrPassthrough) {
				credential = identity.RawAuthorization
			}
			// Connector gets a chance to self-acquire (e.g. codex ChatGPT-login).
			// If it can't, it returns ErrCredentialRequired below.
		}
		prepared, err := provider.PrepareRequest(call, credential)
		if err != nil {
			if errors.Is(err, runtime.ErrCredentialRequired) {
				return nil, fmt.Errorf("%w: missing connector credential", serverauth.ErrUnauthenticated)
			}
			return nil, err
		}

		resp, err := g.transport.Do(ctx, prepared)
		if err != nil {
			return nil, fmt.Errorf("%w: transport: %v", ErrUpstream, err)
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		transport := responseTransport(r, resp)
		setIngressHeaders(tw.Header(), run.ID, action.TraceID, routeName(framework, call), transport)
		mergeActionAttrs(action, ingressAttrs(r, framework, call, transport, credentialMode(identity, callerAuthorization, credential), resp.StatusCode))
		if transport == "websocket" {
			return g.handleWebSocket(tw, resp, provider)
		}
		if strings.Contains(contentType, "event-stream") {
			return g.handleStream(tw, resp, provider)
		}
		return g.handleBuffered(tw, resp, provider)
	}

	result, err := g.pipeline.Execute(ctx, call.Action, handler)
	g.finishAction(ctx, call.Action, result, err)
	g.finishRun(ctx, run.ID, err)
	if err != nil && result == nil && !tw.WroteHeader() {
		status, code, message, details := mapError(err)
		setIngressHeaders(tw.Header(), run.ID, call.Action.TraceID, routeName(framework, call), "blocked")
		mergeActionAttrs(call.Action, ingressAttrs(r, framework, call, "blocked", "none", status))
		writeError(tw, status, code, message, details)
	}
}

func routeName(framework string, call *runtime.LLMCall) string {
	if isCodexChatGPTPath(call.UpstreamPath) {
		return "openai.codex.chatgpt"
	}
	if framework == "" || framework == "raw" {
		return "openai"
	}
	return framework + "." + call.Provider
}

func responseTransport(r *http.Request, resp *http.Response) string {
	if isWebSocketRequest(r) || isWebSocketResponse(resp) {
		return "websocket"
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "event-stream") {
		return "sse"
	}
	return "buffered"
}

func setIngressHeaders(header http.Header, runID, traceID, route, transport string) {
	if runID != "" {
		header.Set("X-Kave-Run-ID", runID)
	}
	if traceID != "" {
		header.Set("X-Kave-Trace-ID", traceID)
	}
	if route != "" {
		header.Set("X-Kave-Ingress-Route", route)
	}
	if transport != "" {
		header.Set("X-Kave-Transport", transport)
	}
}

func ingressAttrs(r *http.Request, framework string, call *runtime.LLMCall, transport, credMode string, status int) map[string]any {
	authMode := "api_key"
	if isCodexChatGPTPath(call.UpstreamPath) {
		authMode = "chatgpt_bearer"
	}
	return map[string]any{
		"ingress.route":           routeName(framework, call),
		"ingress.transport":       transport,
		"ingress.inbound_path":    r.URL.Path,
		"ingress.upstream_path":   call.UpstreamPath,
		"ingress.framework":       framework,
		"ingress.auth_mode":       authMode,
		"ingress.credential_mode": credMode,
		"ingress.http_status":     status,
		"connector.path":          "/v1/openai",
	}
}

func credentialMode(identity authctx.Identity, callerAuthorization, credential string) string {
	if callerAuthorization != "" && credential == callerAuthorization && (identity.IsAnonymous() || identity.IsGuest()) {
		return "passthrough"
	}
	if credential != "" {
		return "stored"
	}
	return "none"
}

func mergeActionAttrs(action *coreruntime.Action, attrs map[string]any) {
	if action == nil || len(attrs) == 0 {
		return
	}
	if action.Attrs == nil {
		action.Attrs = map[string]any{}
	}
	for k, v := range attrs {
		action.Attrs[k] = v
	}
}

func isCodexChatGPTPath(path string) bool {
	return strings.HasPrefix(path, "/backend-api/codex/")
}

func chatGPTUpstreamURL(path, rawQuery string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("KAVE_CODEX_CHATGPT_UPSTREAM")), "/")
	if baseURL == "" {
		baseURL = "https://chatgpt.com"
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	base.Path = joinURLPath(base.Path, path)
	base.RawQuery = rawQuery
	return base.String(), nil
}

func joinURLPath(basePath, path string) string {
	basePath = strings.TrimRight(basePath, "/")
	path = "/" + strings.TrimLeft(path, "/")
	if basePath == "" {
		return path
	}
	return basePath + path
}

func stripProxyHopHeaders(headers http.Header) {
	for _, key := range []string{
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

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") && headerHasToken(r.Header, "Connection", "upgrade")
}

func isWebSocketResponse(resp *http.Response) bool {
	return resp.StatusCode == http.StatusSwitchingProtocols && strings.EqualFold(resp.Header.Get("Upgrade"), "websocket")
}

func headerHasToken(h http.Header, key, token string) bool {
	for _, value := range h.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func (g *FrameworkGateway) handleBuffered(w http.ResponseWriter, resp *http.Response, provider runtime.LLMConnector) (*pipeline.Result, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read upstream response: %v", ErrUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: upstream status %d", ErrUpstream, resp.StatusCode)
	}

	result, err := provider.ParseResponse(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	result.ProviderRequestID = providerRequestID(resp.Header)

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
	return result, nil
}

func (g *FrameworkGateway) handleToolBuffered(w http.ResponseWriter, resp *http.Response, tool runtime.ToolConnector) (*pipeline.Result, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read upstream response: %v", ErrUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: upstream status %d", ErrUpstream, resp.StatusCode)
	}

	result, err := tool.ParseToolResponse(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	result.ProviderRequestID = providerRequestID(resp.Header)

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
	return result, nil
}

func (g *FrameworkGateway) handleStream(w http.ResponseWriter, resp *http.Response, provider runtime.LLMConnector) (*pipeline.Result, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("%w: upstream status %d", ErrUpstream, resp.StatusCode)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported by response writer")
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)
	tmp := make([]byte, 4096)
	for {
		n, readErr := tee.Read(tmp)
		if n > 0 {
			_, _ = w.Write(tmp[:n])
			flusher.Flush()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("stream upstream: %w", readErr)
		}
	}

	result, err := provider.ParseResponse(buf.Bytes(), resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	result.ProviderRequestID = providerRequestID(resp.Header)
	return result, nil
}

func (g *FrameworkGateway) handleWebSocket(w http.ResponseWriter, resp *http.Response, _ runtime.LLMConnector) (*pipeline.Result, error) {
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("%w: upstream status %d", ErrUpstream, resp.StatusCode)
	}

	upstream, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		return nil, fmt.Errorf("%w: upstream websocket is not writable", ErrUpstream)
	}
	defer upstream.Close()

	downstream, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket hijack: %w", err)
	}
	defer downstream.Close()

	copyHeaders(resp.Header, w.Header())
	if _, err := fmt.Fprintf(rw, "HTTP/1.1 %d %s\r\n", resp.StatusCode, http.StatusText(resp.StatusCode)); err != nil {
		return nil, err
	}
	if err := resp.Header.Write(rw); err != nil {
		return nil, err
	}
	if _, err := rw.WriteString("\r\n"); err != nil {
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, downstream)
		_ = upstream.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(downstream, upstream)
		_ = downstream.Close()
	}()
	wg.Wait()

	return &pipeline.Result{ProviderRequestID: providerRequestID(resp.Header)}, nil
}
