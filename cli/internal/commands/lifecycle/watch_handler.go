package lifecycle

import (
	"bufio"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type WatchInput struct {
}

type WatchOutput struct {
	Data any `json:"data"`
}

func RunWatch(ctx context.Context, in WatchInput) (*WatchOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	type sample struct {
		Stream string `json:"stream"`
		Frame  string `json:"frame"`
	}
	paths := []string{"/api/v1/spans/stream", "/api/v1/events/tail", "/api/v1/logs/tail"}
	out := make([]sample, 0, len(paths))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			streamCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			rc, err := rt.Client().Stream(streamCtx, path, nil)
			if err != nil {
				return
			}
			defer rc.Close()
			scanner := bufio.NewScanner(rc)
			if !scanner.Scan() {
				return
			}
			mu.Lock()
			out = append(out, sample{Stream: path, Frame: scanner.Text()})
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return &WatchOutput{Data: map[string]any{"frames": out}}, nil
}
