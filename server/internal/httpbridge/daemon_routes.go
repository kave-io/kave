package httpbridge

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/kave-io/kave/server/internal/daemon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BuildDaemonRoutes returns the daemon lifecycle routes served by the bridge.
func BuildDaemonRoutes(state *daemon.State) []Route {
	return []Route{
		{Path: "GET /api/v1/status", Invoke: statusRoute(state)},
		{Path: "GET /api/v1/doctor", Invoke: doctorRoute(state)},
		{Path: "GET /api/v1/config/view", Invoke: configViewRoute(state)},
		{Path: "GET /api/v1/config/diff", Invoke: configDiffRoute(state)},
		{Path: "GET /api/v1/config/paths", Invoke: configPathsRoute(state)},
		{Path: "GET /api/v1/diff", Invoke: diffRoute(state)},
		{Path: "POST /api/v1/apply", Invoke: applyRoute(state)},
		{Path: "POST /api/v1/config/reload", Invoke: configReloadRoute(state)},
		{Path: "GET /api/v1/admin/store", Invoke: adminStoreRoute(state)},
	}
}

func statusRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		return Outcome{Kind: "Status", Data: state.Snapshot(ctx)}, nil
	}
}

func doctorRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		return Outcome{Kind: "Doctor", Data: state.Doctor(ctx)}, nil
	}
}

func configViewRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		view, err := state.ConfigView()
		if err != nil {
			return Outcome{Kind: "Config"}, err
		}
		return Outcome{Kind: "Config", Data: view}, nil
	}
}

func configDiffRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		diff, err := state.ConfigDiff()
		if err != nil {
			return Outcome{Kind: "ConfigDiff"}, err
		}
		return Outcome{Kind: "ConfigDiff", Data: diff}, nil
	}
}

func configPathsRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		return Outcome{Kind: "ConfigPaths", Data: state.ConfigPaths()}, nil
	}
}

func diffRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		plan, err := state.BuildPlan(ctx)
		if err != nil {
			return Outcome{Kind: "ApplyPlan"}, err
		}
		return Outcome{Kind: "ApplyPlan", Data: plan}, nil
	}
}

func applyRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req struct {
			Prune bool `json:"prune"`
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				return Outcome{Kind: "ApplyReport"}, status.Error(codes.InvalidArgument, "invalid request body")
			}
		}
		plan, err := state.BuildPlan(ctx)
		if err != nil {
			return Outcome{Kind: "ApplyReport"}, err
		}
		report, err := state.Apply(ctx, plan, req.Prune)
		if err != nil {
			return Outcome{Kind: "ApplyReport"}, err
		}
		return Outcome{Kind: "ApplyReport", Data: report}, nil
	}
}

func configReloadRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		report, err := state.Reload(ctx)
		if err != nil {
			if daemon.IsInvalidConfig(err) {
				return Outcome{Kind: "ConfigReload"}, status.Error(codes.FailedPrecondition, err.Error())
			}
			return Outcome{Kind: "ConfigReload"}, err
		}
		return Outcome{Kind: "ConfigReload", Data: report}, nil
	}
}

func adminStoreRoute(state *daemon.State) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		report, err := state.AdminStore(ctx)
		if err != nil {
			return Outcome{Kind: "Store"}, err
		}
		return Outcome{Kind: "Store", Data: report}, nil
	}
}
