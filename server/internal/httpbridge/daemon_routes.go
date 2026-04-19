package httpbridge

import (
	"context"
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
