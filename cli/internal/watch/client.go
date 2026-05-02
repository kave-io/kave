package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type Client interface {
	Stream(ctx context.Context, filter Filter) (<-chan Event, <-chan error)
	Endpoint() string
}

type grpcClient struct {
	svc     runtimev1.RuntimeServiceClient
	project string
	env     string
	server  string
}

func NewGRPCClientFromContext(ctx context.Context) (Client, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.RuntimeSvc()
	if err != nil {
		return nil, err
	}
	return &grpcClient{
		svc:     svc,
		project: runtime.ActiveProject(ctx),
		env:     runtime.ActiveEnv(ctx),
		server:  runtime.ActiveServer(ctx),
	}, nil
}

func (c *grpcClient) Endpoint() string { return c.server }

func (c *grpcClient) Stream(ctx context.Context, filter Filter) (<-chan Event, <-chan error) {
	events := make(chan Event, 64)
	errs := make(chan error, 4)

	eventsStream, err := c.svc.WatchEvents(ctx, &runtimev1.WatchEventsRequest{
		ProjectId: c.project,
		EnvId:     c.env,
		Kind:      strings.TrimSpace(filter.Type),
	})
	if err != nil {
		errs <- err
		close(events)
		close(errs)
		return events, errs
	}

	runsStream, runErr := c.svc.WatchRuns(ctx, watchRunsReq(c.env, filter))

	go func() {
		defer close(events)
		defer close(errs)

		runChan := make(chan Event, 32)
		eventChan := make(chan Event, 32)

		if runErr == nil {
			go recvRuns(ctx, runsStream, runChan, errs)
		} else {
			errs <- runErr
			close(runChan)
		}
		go recvEvents(ctx, eventsStream, eventChan, errs)

		for runChan != nil || eventChan != nil {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-eventChan:
				if !ok {
					eventChan = nil
					continue
				}
				if matchFilter(ev, filter) {
					events <- ev
				}
			case ev, ok := <-runChan:
				if !ok {
					runChan = nil
					continue
				}
				if matchFilter(ev, filter) {
					events <- ev
				}
			}
		}
	}()

	return events, errs
}

func watchRunsReq(env string, filter Filter) *runtimev1.WatchRunsRequest {
	req := &runtimev1.WatchRunsRequest{EnvId: env}
	if v := strings.TrimSpace(filter.Agent); v != "" {
		req.AgentId = &v
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		for _, s := range strings.Split(status, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			switch s {
			case "active", "running":
				req.Statuses = append(req.Statuses, runtimev1.RunStatus_RUN_STATUS_ACTIVE)
			case "completed":
				req.Statuses = append(req.Statuses, runtimev1.RunStatus_RUN_STATUS_COMPLETED)
			case "failed":
				req.Statuses = append(req.Statuses, runtimev1.RunStatus_RUN_STATUS_FAILED)
			case "cancelled", "canceled":
				req.Statuses = append(req.Statuses, runtimev1.RunStatus_RUN_STATUS_CANCELLED)
			case "timed_out", "timeout":
				req.Statuses = append(req.Statuses, runtimev1.RunStatus_RUN_STATUS_TIMED_OUT)
			case "blocked":
				req.Statuses = append(req.Statuses, runtimev1.RunStatus_RUN_STATUS_BLOCKED)
			}
		}
	}
	return req
}

type ioRecv[T any] interface{ Recv() (T, error) }

func recvEvents(ctx context.Context, stream ioRecv[*runtimev1.RuntimeEvent], out chan<- Event, errs chan<- error) {
	defer close(out)
	for {
		rec, err := stream.Recv()
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				errs <- err
			}
			return
		}
		out <- mapRuntimeEvent(rec)
	}
}

func recvRuns(ctx context.Context, stream ioRecv[*runtimev1.RunRecord], out chan<- Event, errs chan<- error) {
	defer close(out)
	for {
		rec, err := stream.Recv()
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				errs <- err
			}
			return
		}
		out <- mapRunRecord(rec)
	}
}

func mapRunRecord(rec *runtimev1.RunRecord) Event {
	ev := Event{
		At:        time.UnixMilli(rec.GetUpdatedAtMs()),
		Kind:      "run.update",
		Status:    strings.TrimPrefix(strings.ToLower(rec.GetStatus().String()), "run_status_"),
		RunID:     rec.GetId(),
		AgentID:   rec.GetAgentId(),
		AgentName: rec.GetName(),
		Metadata:  map[string]any{},
	}
	if rec.GetCreatedAtMs() > 0 && ev.At.IsZero() {
		ev.At = time.UnixMilli(rec.GetCreatedAtMs())
	}
	if spent := rec.GetSpent(); spent != nil {
		ev.Cost = parseAmount(spent.GetDecimal())
		ev.Currency = spent.GetCurrency()
	}
	if msg := rec.GetErrorMessage(); msg != "" {
		ev.Error = msg
		ev.Message = msg
	}
	return ev
}

func mapRuntimeEvent(rec *runtimev1.RuntimeEvent) Event {
	ev := Event{
		At:       time.UnixMilli(rec.GetAt()),
		Kind:     rec.GetKind(),
		Metadata: map[string]any{},
		Raw:      map[string]any{},
	}
	if len(rec.GetData()) == 0 {
		return ev
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.GetData(), &raw); err != nil {
		ev.Message = string(rec.GetData())
		return ev
	}
	ev.Raw = raw
	ev.RunID = firstString(raw, "run_id", "runId")
	ev.AgentID = firstString(raw, "agent_id", "agentId")
	ev.AgentName = firstString(raw, "agent_name", "agentName", "name")
	ev.TraceID = firstString(raw, "trace_id", "traceId")
	ev.SpanID = firstString(raw, "span_id", "spanId", "id")
	ev.Status = firstString(raw, "status")
	ev.Connector = firstString(raw, "connector")
	ev.Tool = firstString(raw, "method", "tool")
	ev.Model = firstString(raw, "model")
	ev.Message = firstString(raw, "message", "error_message", "error")
	ev.Error = firstString(raw, "error", "error_message")
	ev.PolicyDecision = firstString(raw, "decision", "policy_decision", "policyDecision")
	ev.Cost = firstFloat(raw, "cost", "amount")
	ev.Currency = firstString(raw, "currency", "currency_code")
	ev.InputTokens = int(firstFloat(raw, "input_tokens", "inputTokens"))
	ev.OutputTokens = int(firstFloat(raw, "output_tokens", "outputTokens"))
	ev.Metadata = raw
	if ev.Message == "" {
		ev.Message = rec.GetKind()
	}
	return ev
}

func matchFilter(ev Event, f Filter) bool {
	if f.Since > 0 && !ev.At.IsZero() && ev.At.Before(time.Now().Add(-f.Since)) {
		return false
	}
	if v := strings.TrimSpace(f.Agent); v != "" {
		if !strings.EqualFold(ev.AgentID, v) && !strings.EqualFold(ev.AgentName, v) {
			return false
		}
	}
	if v := strings.TrimSpace(f.RunID); v != "" && !strings.EqualFold(ev.RunID, v) {
		return false
	}
	if v := strings.TrimSpace(f.TraceID); v != "" && !strings.EqualFold(ev.TraceID, v) {
		return false
	}
	if v := strings.TrimSpace(f.Status); v != "" && !strings.Contains(strings.ToLower(ev.Status), strings.ToLower(v)) {
		return false
	}
	if v := strings.TrimSpace(f.Type); v != "" && !strings.Contains(strings.ToLower(ev.Kind), strings.ToLower(v)) {
		return false
	}
	return true
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func firstFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s == "" {
				continue
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func parseAmount(v string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}
