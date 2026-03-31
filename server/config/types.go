package config

import (
	"fmt"
	"strings"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Ollama   OllamaConfig   `mapstructure:"ollama"`
	Models   ModelsConfig   `mapstructure:"models"`
	Output   OutputConfig   `mapstructure:"output"`
	Prompt   PromptConfig   `mapstructure:"prompt"`
	Email    EmailConfig    `mapstructure:"email"`
	Pools    PoolsConfig    `mapstructure:"pools"`
}

// ── Server ────────────────────────────────────────────────────────────────────

type ServerConfig struct {
	Port           int    `mapstructure:"port"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	Environment    string `mapstructure:"environment"`
	Domain         string `mapstructure:"domain"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf(":%d", s.Port)
}

func (s ServerConfig) IsDev() bool {
	return s.Environment == "development"
}

// ── Postgres ──────────────────────────────────────────────────────────────────

type PostgresConfig struct {
	Host     string          `mapstructure:"host"`
	Port     int             `mapstructure:"port"`
	User     string          `mapstructure:"user"`
	Password string          `mapstructure:"password"`
	DBName   string          `mapstructure:"dbname"`
	SSLMode  string          `mapstructure:"sslmode"`
	Pool     PoolConfig      `mapstructure:"pool"`
	Logging  DBLoggingConfig `mapstructure:"logging"`
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName, p.SSLMode,
	)
}

// UnixSocketDSN returns the faster unix socket path when host is localhost.
// pgx will use this automatically if you pass the socket directory.
func (p PostgresConfig) UnixSocketDSN() string {
	if p.Host == "localhost" || p.Host == "127.0.0.1" {
		return fmt.Sprintf(
			"host=/var/run/postgresql port=%d user=%s password=%s dbname=%s sslmode=%s",
			p.Port, p.User, p.Password, p.DBName, p.SSLMode,
		)
	}
	return p.DSN()
}

type PoolConfig struct {
	MaxOpenConns           int `mapstructure:"max_open_conns"`
	MaxIdleConns           int `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeMinutes int `mapstructure:"conn_max_lifetime_minutes"`
}

type DBLoggingConfig struct {
	Enabled              bool `mapstructure:"enabled"`
	SlowQueryThresholdMs int  `mapstructure:"slow_query_threshold_ms"`
}

// ── Storage ────────────────────────────────────────────────────────────────────

type StorageConfig struct {
	Backend     string `mapstructure:"backend"`       // "sqlite" | "postgres" (default: sqlite)
	SQLitePath  string `mapstructure:"sqlite_path"`   // path to SQLite DB file (default: kave.db)
	SpanBackend string `mapstructure:"span_backend"`  // "duckdb" | "postgres" (default: duckdb)
	DuckDBPath  string `mapstructure:"duckdb_path"`   // path to DuckDB file (default: kave-spans.duckdb)
}

// ── Ollama ────────────────────────────────────────────────────────────────────

type OllamaConfig struct {
	Host    string `mapstructure:"host"`
	Timeout int    `mapstructure:"timeout"`
	Stream  bool   `mapstructure:"stream"`
}

// ── Model routing ─────────────────────────────────────────────────────────────

type ModelsConfig struct {
	// Agent / delegation
	Summarize string `mapstructure:"summarize"`
	Review    string `mapstructure:"review"`
	TestGen   string `mapstructure:"testgen"`
	Architect string `mapstructure:"architect"`
	Diff      string `mapstructure:"diff"`
	DocGen    string `mapstructure:"docgen"`

	// Code
	Ask       string `mapstructure:"ask"`
	Fix       string `mapstructure:"fix"`
	Refactor  string `mapstructure:"refactor"`
	Commit    string `mapstructure:"commit"`
	Changelog string `mapstructure:"changelog"`

	// Writing
	Chat      string `mapstructure:"chat"`
	Explain   string `mapstructure:"explain"`
	Draft     string `mapstructure:"draft"`
	Improve   string `mapstructure:"improve"`
	Translate string `mapstructure:"translate"`

	// Intelligence
	Search string `mapstructure:"search"`
	Index  string `mapstructure:"index"`
	Plan   string `mapstructure:"plan"`

	// Embedding — separate from inference models
	Embed string `mapstructure:"embed"`

	// Vision
	Vision string `mapstructure:"vision"`

	// Heavy
	DeepReview string `mapstructure:"deep-review"`

	// Fallback
	Default string `mapstructure:"default"`
}

func (m ModelsConfig) ModelFor(skill string) string {
	if model := m.bySkill(skill); model != "" {
		return model
	}
	return m.Default
}

func (m ModelsConfig) bySkill(skill string) string {
	switch skill {
	case "summarize":
		return m.Summarize
	case "review":
		return m.Review
	case "testgen":
		return m.TestGen
	case "architect":
		return m.Architect
	case "diff":
		return m.Diff
	case "docgen":
		return m.DocGen
	case "ask":
		return m.Ask
	case "fix":
		return m.Fix
	case "refactor":
		return m.Refactor
	case "commit":
		return m.Commit
	case "changelog":
		return m.Changelog
	case "chat":
		return m.Chat
	case "explain":
		return m.Explain
	case "draft":
		return m.Draft
	case "improve":
		return m.Improve
	case "translate":
		return m.Translate
	case "search":
		return m.Search
	case "index":
		return m.Index
	case "plan":
		return m.Plan
	case "embed":
		return m.Embed
	case "vision":
		return m.Vision
	case "deep-review":
		return m.DeepReview
	default:
		return ""
	}
}

// ── Output ────────────────────────────────────────────────────────────────────

type OutputFormat string

const (
	OutputPlain    OutputFormat = "plain"
	OutputJSON     OutputFormat = "json"
	OutputMarkdown OutputFormat = "markdown"
)

type OutputConfig struct {
	Format    OutputFormat `mapstructure:"format"`
	MaxTokens int          `mapstructure:"max_tokens"`
}

// ── Prompt ────────────────────────────────────────────────────────────────────

type PromptConfig struct {
	Language       string `mapstructure:"language"`
	CodeStyle      string `mapstructure:"code_style"`
	OutputLength   string `mapstructure:"output_length"`
	PersianSupport bool   `mapstructure:"persian_support"`
}

// ── Email ─────────────────────────────────────────────────────────────────────

type EmailConfig struct {
	From string     `mapstructure:"from"`
	SMTP SMTPConfig `mapstructure:"smtp"`
}

type SMTPConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	UseTLS         bool   `mapstructure:"use_tls"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// ── Worker Pools (pond/v2) ────────────────────────────────────────────────

// TaskPoolConfig defines fine-grained behavior for a single pond pool.
// Each pool can have different concurrency, queue sizing, blocking behavior, etc.
type TaskPoolConfig struct {
	MaxConcurrency  int    `mapstructure:"max_concurrency"`   // Number of concurrent workers
	QueueSize       int    `mapstructure:"queue_size"`        // 0=no queue, -1=unbounded, >0=bounded
	NonBlocking     bool   `mapstructure:"non_blocking"`      // true=reject if queue full; false=block
	PanicRecovery   bool   `mapstructure:"panic_recovery"`    // true=recover from panics (default)
	ResultMode      string `mapstructure:"result_mode"`       // "fire-and-forget" or "result-returning"
	Description     string `mapstructure:"description"`       // Human-readable pool purpose
}

// PoolsConfig groups all pond pool configurations.
type PoolsConfig struct {
	// Per-pool fine-grained configuration (overrides defaults)
	Pools map[string]TaskPoolConfig `mapstructure:"pools"`

	// Global defaults for pools not explicitly configured
	EmbedWorkers     int `mapstructure:"embed_workers"`     // Default: 8 (network-bound)
	InferenceWorkers int `mapstructure:"inference_workers"` // Default: 4 (can be CPU-bound)
}

// ── Validation ────────────────────────────────────────────────────────────────

func (c *Config) Validate() error {
	// Ollama
	if c.Ollama.Host == "" {
		c.Ollama.Host = "http://localhost:11434"
	}
	if c.Ollama.Timeout == 0 {
		c.Ollama.Timeout = 120
	}

	// Models
	if c.Models.Default == "" {
		c.Models.Default = "qwen2.5:7b"
	}
	if c.Models.Embed == "" {
		c.Models.Embed = "qwen3-embedding:4b"
	}

	// Output
	if c.Output.MaxTokens == 0 {
		c.Output.MaxTokens = 2048
	}
	if c.Output.Format == "" {
		c.Output.Format = OutputPlain
	}

	// Prompt
	if c.Prompt.Language == "" {
		c.Prompt.Language = "english"
	}
	if c.Prompt.CodeStyle == "" {
		c.Prompt.CodeStyle = "idiomatic"
	}
	if c.Prompt.OutputLength == "" {
		c.Prompt.OutputLength = "concise"
	}

	// Server
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Environment == "" {
		c.Server.Environment = "development"
	}

	// Postgres
	if c.Postgres.SSLMode == "" {
		c.Postgres.SSLMode = "disable"
	}
	if c.Postgres.Pool.MaxOpenConns == 0 {
		c.Postgres.Pool.MaxOpenConns = 20
	}
	if c.Postgres.Pool.MaxIdleConns == 0 {
		c.Postgres.Pool.MaxIdleConns = 4
	}
	if c.Postgres.Pool.ConnMaxLifetimeMinutes == 0 {
		c.Postgres.Pool.ConnMaxLifetimeMinutes = 10
	}

	// Storage
	if c.Storage.Backend == "" {
		c.Storage.Backend = "sqlite"
	}
	if c.Storage.SQLitePath == "" {
		c.Storage.SQLitePath = "kave.db"
	}
	if c.Storage.SpanBackend == "" {
		c.Storage.SpanBackend = "duckdb"
	}
	if c.Storage.DuckDBPath == "" {
		c.Storage.DuckDBPath = "kave-spans.duckdb"
	}

	// Worker pools
	if c.Pools.EmbedWorkers == 0 {
		c.Pools.EmbedWorkers = 8 // Network-bound: embeddings benefit from more concurrency
	}
	if c.Pools.InferenceWorkers == 0 {
		c.Pools.InferenceWorkers = 4 // Inference can be CPU-bound on some models
	}

	// Validation
	validFormats := map[OutputFormat]bool{
		OutputPlain: true, OutputJSON: true, OutputMarkdown: true,
	}
	if !validFormats[c.Output.Format] {
		return fmt.Errorf("invalid output.format %q: must be plain, json, or markdown", c.Output.Format)
	}

	validLengths := map[string]bool{
		"concise": true, "detailed": true, "exhaustive": true,
	}
	if !validLengths[c.Prompt.OutputLength] {
		return fmt.Errorf("invalid prompt.output_length %q", c.Prompt.OutputLength)
	}

	return nil
}

func (c *Config) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "server:   %s (%s)\n", c.Server.Addr(), c.Server.Environment)
	fmt.Fprintf(&b, "ollama:   %s (timeout: %ds)\n", c.Ollama.Host, c.Ollama.Timeout)
	fmt.Fprintf(&b, "postgres: %s:%d/%s\n", c.Postgres.Host, c.Postgres.Port, c.Postgres.DBName)
	fmt.Fprintf(&b, "output:   format=%s max_tokens=%d\n", c.Output.Format, c.Output.MaxTokens)
	fmt.Fprintf(&b, "model:    default=%s embed=%s\n", c.Models.Default, c.Models.Embed)
	return b.String()
}
