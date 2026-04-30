package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/connectors/runtime"
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
	mux.HandleFunc("/v1/anthropic/", g.handleRaw("anthropic"))
	mux.HandleFunc("/v1/google/", g.handleRaw("google"))
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

func (g *FrameworkGateway) handleFramework(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/frameworks/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "gateway.bad_request", "invalid framework route", nil)
		return
	}
	g.handleProxy(w, r, parts[0])
}

func (g *FrameworkGateway) handleProxy(w http.ResponseWriter, r *http.Request, framework string) {
	ctx := r.Context()
	tw := &trackingResponseWriter{ResponseWriter: w}

	identity, ok := authctx.From(ctx)
	if !ok {
		identity, _ = serverauth.ParseIdentity(ctx, r.Header.Get("Authorization"), g.app, nil, true)
	}
	if identity.IsInvalid() {
		writeError(tw, http.StatusUnauthorized, "gateway.unauthorized", "invalid authorization", nil)
		return
	}

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
		if connector.RequiresAuth() {
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

	credential, credErr := g.resolveCredential(ctx, agent.EnvID, call.Provider)
	if errors.Is(credErr, credresolve.ErrPassthrough) {
		credential = identity.RawAuthorization
	}

	handler := func(ctx context.Context, action *coreruntime.Action) (*pipeline.Result, error) {
		prepared, err := provider.PrepareRequest(call, credential)
		if err != nil {
			return nil, err
		}

		resp, err := g.transport.Do(ctx, prepared)
		if err != nil {
			return nil, fmt.Errorf("%w: transport: %v", ErrUpstream, err)
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "event-stream") {
			return g.handleStream(tw, resp, provider)
		}
		return g.handleBuffered(tw, resp, provider)
	}

	result, err := g.pipeline.Execute(ctx, call.Action, handler)
	g.finishRun(ctx, run.ID, err)
	if err != nil && result == nil && !tw.WroteHeader() {
		status, code, message, details := mapError(err)
		writeError(tw, status, code, message, details)
	}
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

	return provider.ParseResponse(buf.Bytes(), resp.Header.Get("Content-Type"))
}
