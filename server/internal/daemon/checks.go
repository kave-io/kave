package daemon

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/core/store"
	"golang.org/x/sync/errgroup"
)

// Doctor runs the daemon check list and returns the aggregate result.
func (s *State) Doctor(ctx context.Context) DoctorReport {
	checks := []struct {
		name string
		run  func(context.Context) CheckResult
	}{
		{name: "daemon", run: s.checkDaemon},
		{name: "app_store", run: s.checkAppStore},
		{name: "span_store_default", run: s.checkSpanStore},
		{name: "fx_fresh", run: s.checkFXFresh},
		{name: "pricing_loaded", run: s.checkPricingLoaded},
		{name: "credentials_resolve", run: s.checkCredentialsResolve},
		{name: "config_merge", run: s.checkConfigMerge},
	}

	results := make([]CheckResult, len(checks))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i, chk := range checks {
		i, chk := i, chk
		g.Go(func() error {
			results[i] = chk.run(gctx)
			return nil
		})
	}
	_ = g.Wait()

	overall := "ok"
	for _, result := range results {
		switch result.Status {
		case "fail":
			overall = "fail"
		case "warn":
			if overall == "ok" {
				overall = "warn"
			}
		}
	}

	return DoctorReport{Checks: results, Overall: overall}
}

func (s *State) checkDaemon(context.Context) CheckResult {
	return CheckResult{Name: "daemon", Status: "ok"}
}

func (s *State) checkAppStore(ctx context.Context) CheckResult {
	if s.app == nil {
		return CheckResult{Name: "app_store", Status: "fail", Detail: "app store unavailable"}
	}
	if err := ping(ctx, s.app); err != nil {
		return CheckResult{Name: "app_store", Status: "fail", Detail: err.Error()}
	}
	return CheckResult{Name: "app_store", Status: "ok"}
}

func (s *State) checkSpanStore(ctx context.Context) CheckResult {
	if s.span == nil {
		return CheckResult{Name: "span_store_default", Status: "fail", Detail: "span store unavailable"}
	}
	if err := ping(ctx, s.span); err != nil {
		return CheckResult{Name: "span_store_default", Status: "fail", Detail: err.Error()}
	}
	return CheckResult{Name: "span_store_default", Status: "ok"}
}

func (s *State) checkFXFresh(context.Context) CheckResult {
	if s.fx == nil {
		return CheckResult{Name: "fx_fresh", Status: "warn", Detail: "fx service unavailable"}
	}
	snapshot := s.fx.Snapshot()
	if snapshot == nil {
		return CheckResult{Name: "fx_fresh", Status: "warn", Detail: "fx snapshot unavailable"}
	}
	loaded, _ := snapshot["loaded"].(bool)
	if !loaded {
		return CheckResult{Name: "fx_fresh", Status: "warn", Detail: "fx rates are not loaded"}
	}
	ageMs, _ := snapshot["age_ms"].(int64)
	if ageAny, ok := snapshot["age_ms"].(int); ok {
		ageMs = int64(ageAny)
	}
	if ageMs > 24*60*60*1000 {
		hours := ageMs / 3600000
		return CheckResult{Name: "fx_fresh", Status: "warn", Detail: fmt.Sprintf("rates are %dh old", hours)}
	}
	return CheckResult{Name: "fx_fresh", Status: "ok"}
}

func (s *State) checkPricingLoaded(context.Context) CheckResult {
	if s.cost == nil {
		return CheckResult{Name: "pricing_loaded", Status: "fail", Detail: "pricing service unavailable"}
	}
	book := s.cost.Current()
	if book == nil || len(book.Entries) == 0 {
		return CheckResult{Name: "pricing_loaded", Status: "fail", Detail: "pricing book empty"}
	}
	return CheckResult{Name: "pricing_loaded", Status: "ok", Detail: fmt.Sprintf("%d models", len(book.Entries))}
}

func (s *State) checkCredentialsResolve(ctx context.Context) CheckResult {
	if s.app == nil {
		return CheckResult{Name: "credentials_resolve", Status: "fail", Detail: "app store unavailable"}
	}
	resp, err := s.app.ListCredentials(ctx, "", store.Page{Limit: 1000})
	if err != nil {
		return CheckResult{Name: "credentials_resolve", Status: "fail", Detail: err.Error()}
	}
	return CheckResult{Name: "credentials_resolve", Status: "ok", Detail: fmt.Sprintf("%d credentials reachable", len(resp.Items))}
}

func (s *State) checkConfigMerge(context.Context) CheckResult {
	diff, err := s.ConfigDiff()
	if err != nil {
		return CheckResult{Name: "config_merge", Status: "fail", Detail: err.Error()}
	}
	if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Changed) == 0 {
		return CheckResult{Name: "config_merge", Status: "ok"}
	}
	return CheckResult{Name: "config_merge", Status: "warn", Detail: "live config differs from disk"}
}

func ping(ctx context.Context, v any) error {
	pinger, ok := v.(interface{ Ping(context.Context) error })
	if !ok || pinger == nil {
		return fmt.Errorf("ping unavailable")
	}
	return pinger.Ping(ctx)
}
