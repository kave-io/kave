package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kave-io/kave/cli/internal/output"
	"github.com/kave-io/kave/cli/internal/runtime"
	"github.com/kave-io/kave/cli/internal/tty"
	"github.com/kave-io/kave/cli/internal/watch"
	"github.com/kave-io/kave/cli/internal/watchui"
)

type WatchInput struct {
	Agent   string
	Run     string
	Trace   string
	Status  string
	Type    string
	Since   time.Duration
	Limit   int
	Compact bool
}

func RunWatch(ctx context.Context, in WatchInput) error {
	if !tty.IsTerminal(os.Stdout) {
		return fmt.Errorf("kave watch requires an interactive terminal. Use kave events tail, kave span tail, or kave trace export for scriptable output.")
	}

	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return fmt.Errorf("runtime missing")
	}
	if rt.Output == output.FormatJSON || rt.Output == output.FormatYAML {
		return &output.CommandError{
			Code:    "watch.output.unsupported",
			Message: "kave watch is TUI-only and does not support --output json/yaml. Use kave events tail, kave span tail, or kave trace export for scriptable output.",
			Exit:    1,
		}
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := watch.NewGRPCClientFromContext(ctx)
	if err != nil {
		return err
	}

	filter := watch.Filter{
		Agent:   strings.TrimSpace(in.Agent),
		RunID:   strings.TrimSpace(in.Run),
		TraceID: strings.TrimSpace(in.Trace),
		Status:  strings.TrimSpace(in.Status),
		Type:    strings.TrimSpace(in.Type),
		Since:   in.Since,
		Limit:   in.Limit,
		Compact: in.Compact,
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}

	return watchui.Run(ctx, client, filter, watchui.Options{NoColor: rt.Resolution != nil && rt.Resolution.Options.NoColor})
}
