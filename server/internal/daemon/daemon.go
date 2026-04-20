package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/kave-io/kave/core/bus"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
	"github.com/kave-io/kave/server/internal/contract"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/fx"
)

// State owns the live daemon process state.
type State struct {
	PID           int
	StartedAt     time.Time
	Version       string
	SchemaVersion string

	loadOpts config.LoadOpts

	mu  sync.RWMutex
	res *config.LoadResult

	watchOnce sync.Once

	resourceMu     sync.RWMutex
	resourceSource map[string]config.Source

	app  store.AppStore
	span store.SpanStore
	fx   *fx.Service
	cost *cost.Service
	bus  *bus.Bus
}

// New creates a daemon state wrapper around the live runtime dependencies.
func New(opts config.LoadOpts, res *config.LoadResult, app store.AppStore, span store.SpanStore, fxSvc *fx.Service, costSvc *cost.Service, b *bus.Bus, version string) *State {
	if version == "" {
		version = "dev"
	}
	if res == nil {
		res = &config.LoadResult{Config: &config.Config{}, Origin: map[string]config.Source{}}
	}
	return &State{
		PID:           os.Getpid(),
		StartedAt:     time.Now().UTC(),
		Version:       version,
		SchemaVersion: strconv.Itoa(contract.SchemaVersion),
		loadOpts:      opts,
		res:           res,
		resourceSource: map[string]config.Source{},
		app:           app,
		span:          span,
		fx:            fxSvc,
		cost:          costSvc,
		bus:           b,
	}
}

// Snapshot returns the current daemon status envelope.
func (s *State) Snapshot(ctx context.Context) Status {
	cfg := s.currentConfig()

	stores := map[string]string{
		"app":          probeStatus(ctx, s.app),
		"span_default": probeStatus(ctx, s.span),
	}

	status := Status{
		PID:           s.PID,
		Version:       s.Version,
		SchemaVersion: s.SchemaVersion,
		UptimeMS:      time.Since(s.StartedAt).Milliseconds(),
		StartedAt:     s.StartedAt.Format(time.RFC3339Nano),
		StartedAtMS:   s.StartedAt.UnixMilli(),
		GRPCAddr:      cfg.GRPC.Addr(),
		HTTPAddr:      cfg.Server.Addr(),
		Stores:        stores,
		FX:            map[string]any{},
		Pricing:       map[string]any{},
		Connectors:    registeredConnectors(),
		Bus:           map[string]any{"subscribers": 0},
	}

	if s.fx != nil {
		status.FX = s.fx.Snapshot()
	}
	if s.cost != nil {
		if book := s.cost.Current(); book != nil {
			status.Pricing = map[string]any{
				"version": book.Version,
				"models":  len(book.Entries),
			}
		}
	}
	if s.bus != nil {
		status.Bus = map[string]any{"subscribers": s.bus.SubscriberCount()}
	}
	return status
}

// AdminStore returns detailed per-store stats for the admin endpoint.
func (s *State) AdminStore(ctx context.Context) (StoreReport, error) {
	appStats, err := collectStats(ctx, s.app)
	if err != nil {
		return StoreReport{}, err
	}
	spanStats, err := collectStats(ctx, s.span)
	if err != nil {
		return StoreReport{}, err
	}
	return StoreReport{
		App:         appStats,
		SpanDefault: spanStats,
	}, nil
}

// ConfigView returns the live config with secret-like keys redacted.
func (s *State) ConfigView() (map[string]any, error) {
	cfg := s.currentConfig()
	return redactConfigValue(cfg), nil
}

// ConfigPaths returns the origin map for the live config.
func (s *State) ConfigPaths() map[string]config.Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.res == nil || s.res.Origin == nil {
		return map[string]config.Source{}
	}
	out := make(map[string]config.Source, len(s.res.Origin))
	for key, value := range s.res.Origin {
		out[key] = value
	}
	return out
}

// ConfigDiff compares the on-disk config against the live config.
func (s *State) ConfigDiff() (ConfigDiffReport, error) {
	disk, err := config.Load(s.loadOpts)
	if err != nil {
		return ConfigDiffReport{}, err
	}
	return diffConfigs(s.currentConfig(), disk.Config), nil
}

// Reload rereads the config from disk and swaps the live config if valid.
func (s *State) Reload(ctx context.Context) (ReloadReport, error) {
	nextRes, err := config.Load(s.loadOpts)
	if err != nil {
		return ReloadReport{}, &InvalidConfigError{Err: err}
	}

	current := s.currentConfig()
	diff := diffConfigPaths(current, nextRes.Config)
	if len(diff) == 0 {
		return ReloadReport{}, nil
	}

	report := ReloadReport{}
	for _, path := range diff {
		switch path {
		case "fx.refresh_interval_seconds":
			report.Applied = append(report.Applied, path)
			if s.fx != nil {
				s.fx.SetInterval(time.Duration(nextRes.Config.FX.RefreshIntervalSeconds) * time.Second)
			} else {
				report.Warnings = append(report.Warnings, Warning{
					Code:    "fx.unavailable",
					Message: "fx service is not available for hot reload",
				})
			}
		default:
			report.RequiresRestart = append(report.RequiresRestart, path)
		}
	}

	plan, err := s.buildPlanForConfig(ctx, nextRes.Config)
	if err != nil {
		return ReloadReport{}, err
	}

	s.mu.Lock()
	s.res = nextRes
	s.mu.Unlock()
	config.GlobalConf = nextRes.Config

	if _, err := s.Apply(ctx, plan, false); err != nil {
		return report, err
	}

	return report, nil
}

func (s *State) currentConfig() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.res == nil || s.res.Config == nil {
		return &config.Config{}
	}
	return s.res.Config
}

func probeStatus(ctx context.Context, p any) string {
	if p == nil {
		return "unavailable"
	}
	pinger, ok := p.(interface{ Ping(context.Context) error })
	if !ok {
		return "unavailable"
	}
	if err := pinger.Ping(ctx); err != nil {
		return "fail"
	}
	return "ok"
}

func collectStats(ctx context.Context, s any) (map[string]any, error) {
	if s == nil {
		return map[string]any{}, nil
	}
	statter, ok := s.(interface {
		Stats(context.Context) (map[string]any, error)
	})
	if !ok {
		return map[string]any{}, nil
	}
	stats, err := statter.Stats(ctx)
	if err != nil {
		return nil, err
	}
	if stats == nil {
		stats = map[string]any{}
	}
	return stats, nil
}

func registeredConnectors() map[string]string {
	return map[string]string{
		"autogen":       "ready",
		"claude-code":   "ready",
		"crewai":        "ready",
		"langchain":     "ready",
		"openai-agents": "ready",
		"openclaw":      "ready",
	}
}

// InvalidConfigError marks reload failures caused by invalid config content.
type InvalidConfigError struct {
	Err error
}

func (e *InvalidConfigError) Error() string {
	if e == nil || e.Err == nil {
		return "invalid config"
	}
	return e.Err.Error()
}

func (e *InvalidConfigError) Unwrap() error { return e.Err }

func IsInvalidConfig(err error) bool {
	var invalid *InvalidConfigError
	return errors.As(err, &invalid)
}

// Status is the daemon status envelope.
type Status struct {
	PID           int               `json:"pid"`
	Version       string            `json:"version"`
	SchemaVersion string            `json:"schema_version"`
	UptimeMS      int64             `json:"uptime_ms"`
	StartedAt     string            `json:"started_at"`
	StartedAtMS   int64             `json:"started_at_ms"`
	GRPCAddr      string            `json:"grpc_addr"`
	HTTPAddr      string            `json:"http_addr"`
	Stores        map[string]string `json:"stores"`
	FX            map[string]any    `json:"fx"`
	Pricing       map[string]any    `json:"pricing"`
	Connectors    map[string]string `json:"connectors"`
	Bus           map[string]any    `json:"bus"`
}

// Warning is a non-fatal reload note.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ReloadReport describes a reload attempt.
type ReloadReport struct {
	Applied         []string  `json:"applied"`
	RequiresRestart []string  `json:"requires_restart"`
	Warnings        []Warning `json:"warnings"`
}

// StoreReport contains admin store details.
type StoreReport struct {
	App         map[string]any `json:"app"`
	SpanDefault map[string]any `json:"span_default"`
}

// DoctorReport is the result of the daemon health checks.
type DoctorReport struct {
	Checks  []CheckResult `json:"checks"`
	Overall string        `json:"overall"`
}

// CheckResult is one doctor check result.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// configSnapshot returns the live config as a generic map.
func configSnapshot(cfg *config.Config) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	if out, ok := valueSnapshot(reflect.ValueOf(cfg)).(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

func valueSnapshot(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return valueSnapshot(v.Elem())
	}
	switch v.Kind() {
	case reflect.Struct:
		out := make(map[string]any)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			tag := field.Tag.Get("mapstructure")
			if tag == "" || tag == "-" {
				continue
			}
			out[tag] = valueSnapshot(v.Field(i))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[formatKey(iter.Key())] = valueSnapshot(iter.Value())
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = valueSnapshot(v.Index(i))
		}
		return out
	default:
		return v.Interface()
	}
}

func formatKey(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		return formatKey(v.Elem())
	}
	if v.Kind() == reflect.String {
		return v.String()
	}
	return fmt.Sprint(v.Interface())
}
