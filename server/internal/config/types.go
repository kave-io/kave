package config

import (
	"fmt"
	"strings"
	"time"
)

type Config struct {
	Daemon         *DaemonConfig      `mapstructure:"daemon"`
	Contexts       []ContextConfig    `mapstructure:"contexts"`
	CurrentContext string             `mapstructure:"currentContext"`
	Project        *ProjectConfig     `mapstructure:"project"`
	Connectors     []ConnectorConfig  `mapstructure:"connectors"`
	Credentials    []CredentialConfig `mapstructure:"credentials"`
	Policies       []PolicyConfig     `mapstructure:"policies"`
	Agents         []AgentConfig      `mapstructure:"agents"`
	Pricing        *PricingConfig     `mapstructure:"pricing"`
	UI             *UIConfig          `mapstructure:"ui"`

	Server   ServerConfig   `mapstructure:"server"`
	GRPC     GRPCConfig     `mapstructure:"grpc"`
	FX       FXConfig       `mapstructure:"fx"`
	Security SecurityConfig `mapstructure:"security"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Ollama   OllamaConfig   `mapstructure:"ollama"`
	Models   ModelsConfig   `mapstructure:"models"`
	Output   OutputConfig   `mapstructure:"output"`
	Prompt   PromptConfig   `mapstructure:"prompt"`
	Email    EmailConfig    `mapstructure:"email"`
}

type DaemonConfig struct {
	Address      string `mapstructure:"address"`
	ProxyAddress string `mapstructure:"proxy_address"`
	DataDir      string `mapstructure:"data_dir"`
	LogLevel     string `mapstructure:"log_level"`
	LogFormat    string `mapstructure:"log_format"`
}

type ContextConfig struct {
	Name    string `mapstructure:"name"`
	Server  string `mapstructure:"server"`
	User    string `mapstructure:"user"`
	Project string `mapstructure:"project"`
	Env     string `mapstructure:"env"`
}

type ProjectEnv struct {
	Name string `mapstructure:"name"`
	Type string `mapstructure:"type"`
	Slug string `mapstructure:"slug"`
}

type ProjectConfig struct {
	Name        string       `mapstructure:"name"`
	Slug        string       `mapstructure:"slug"`
	Description string       `mapstructure:"description"`
	Envs        []ProjectEnv `mapstructure:"envs"`
	DefaultEnv  string       `mapstructure:"defaultEnv"`
}

type ConnectorConfig struct {
	Name    string         `mapstructure:"name"`
	Enabled bool           `mapstructure:"enabled"`
	Config  map[string]any `mapstructure:"config"`
}

type CredentialConfig struct {
	Name      string `mapstructure:"name"`
	Connector string `mapstructure:"connector"`
	Label     string `mapstructure:"label"`
	Source    string `mapstructure:"source"`
	Env       string `mapstructure:"env"`
	File      string `mapstructure:"file"`
	Mode      string `mapstructure:"mode"`
	Ref       string `mapstructure:"ref"`
}

type PolicyConfig struct {
	Name        string         `mapstructure:"name"`
	Description string         `mapstructure:"description"`
	Mode        string         `mapstructure:"mode"`
	Auth        map[string]any `mapstructure:"auth"`
	Cost        map[string]any `mapstructure:"cost"`
	Trace       map[string]any `mapstructure:"trace"`
	Validation  map[string]any `mapstructure:"validation"`
}

type AgentConfig struct {
	Name          string         `mapstructure:"name"`
	Description   string         `mapstructure:"description"`
	Env           string         `mapstructure:"env"`
	Policy        string         `mapstructure:"policy"`
	Credentials   []string       `mapstructure:"credentials"`
	MonthlyBudget *Money         `mapstructure:"monthlyBudget"`
	Status        string         `mapstructure:"status"`
	Metadata      map[string]any `mapstructure:"metadata"`
}

type Money struct {
	Amount   string `mapstructure:"amount"`
	Currency string `mapstructure:"currency"`
}

type PricingConfig struct {
	PriceBook string           `mapstructure:"priceBook"`
	Overrides []map[string]any `mapstructure:"overrides"`
}

type UIConfig struct {
	PreferredCurrency string `mapstructure:"preferredCurrency"`
	Locale            string `mapstructure:"locale"`
	ColorMode         string `mapstructure:"colorMode"`
	DateStyle         string `mapstructure:"dateStyle"`
}

type SecurityConfig struct {
	EncryptionKey     string        `mapstructure:"encryption_key"`
	AllowAnonymous    bool          `mapstructure:"allow_anonymous"`
	AllowLegacyTokens bool          `mapstructure:"allow_legacy_tokens"`
	SessionTTL        time.Duration `mapstructure:"session_ttl"`
	TokenTTL          time.Duration `mapstructure:"token_ttl"`
	Vault             *VaultConfig  `mapstructure:"vault"`
	Casbin            *CasbinConfig `mapstructure:"casbin"`
}

type CasbinConfig struct {
	ModelPath        string `mapstructure:"model_path"`
	DatabaseDSN      string `mapstructure:"database_dsn"`
	SuperAdminBypass bool   `mapstructure:"super_admin_bypass"`
}

type VaultConfig struct {
	Addr  string `mapstructure:"addr"`
	Token string `mapstructure:"token"`
	Mount string `mapstructure:"mount"`
}

type ServerConfig struct {
	Port           int    `mapstructure:"port"`
	Address        string `mapstructure:"addr"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	Environment    string `mapstructure:"environment"`
	Domain         string `mapstructure:"domain"`
}

func (s ServerConfig) Addr() string {
	if s.Address != "" {
		return s.Address
	}
	return fmt.Sprintf(":%d", s.Port)
}

func (s ServerConfig) IsDev() bool {
	return s.Environment == "development"
}

type GRPCConfig struct {
	Port    int    `mapstructure:"port"`
	Address string `mapstructure:"addr"`
}

func (g GRPCConfig) Addr() string {
	if g.Address != "" {
		return g.Address
	}
	return fmt.Sprintf(":%d", g.Port)
}

type FXConfig struct {
	RefreshIntervalSeconds int `mapstructure:"refresh_interval_seconds"`
}

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

type StoreSpec struct {
	Kind         string `mapstructure:"kind"`
	Path         string `mapstructure:"path"`
	PathTemplate string `mapstructure:"path_template"`
	DSN          string `mapstructure:"dsn"`
}

type StorageDefaults struct {
	App  StoreSpec `mapstructure:"app"`
	Span StoreSpec `mapstructure:"span"`
}

type AgentStorageBinding struct {
	Span *StoreSpec `mapstructure:"span"`
}

type StorageConfig struct {
	Defaults StorageDefaults                `mapstructure:"defaults"`
	Agents   map[string]AgentStorageBinding `mapstructure:"agents"`
}

func (c StorageConfig) AppDefault() StoreSpec {
	return c.Defaults.App
}

func (c StorageConfig) SpanDefault() StoreSpec {
	return c.Defaults.Span
}

func (c StorageConfig) SpanForAgent(agentID string) StoreSpec {
	spec := c.Defaults.Span
	if binding, ok := c.Agents[agentID]; ok && binding.Span != nil {
		spec = *binding.Span
	}
	if spec.PathTemplate != "" {
		spec.Path = strings.ReplaceAll(spec.PathTemplate, "{agent_id}", agentID)
	}
	return spec
}

type OllamaConfig struct {
	Host    string `mapstructure:"host"`
	Timeout int    `mapstructure:"timeout"`
	Stream  bool   `mapstructure:"stream"`
}

type ModelsConfig struct {
	Summarize string `mapstructure:"summarize"`
	Review    string `mapstructure:"review"`
	TestGen   string `mapstructure:"testgen"`
	Architect string `mapstructure:"architect"`
	Diff      string `mapstructure:"diff"`
	DocGen    string `mapstructure:"docgen"`

	Ask       string `mapstructure:"ask"`
	Fix       string `mapstructure:"fix"`
	Refactor  string `mapstructure:"refactor"`
	Commit    string `mapstructure:"commit"`
	Changelog string `mapstructure:"changelog"`

	Chat      string `mapstructure:"chat"`
	Explain   string `mapstructure:"explain"`
	Draft     string `mapstructure:"draft"`
	Improve   string `mapstructure:"improve"`
	Translate string `mapstructure:"translate"`

	Search string `mapstructure:"search"`
	Index  string `mapstructure:"index"`
	Plan   string `mapstructure:"plan"`

	Embed string `mapstructure:"embed"`

	Vision string `mapstructure:"vision"`

	DeepReview string `mapstructure:"deep-review"`

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

type PromptConfig struct {
	Language       string `mapstructure:"language"`
	CodeStyle      string `mapstructure:"code_style"`
	OutputLength   string `mapstructure:"output_length"`
	PersianSupport bool   `mapstructure:"persian_support"`
}

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

func (c *Config) Validate() error {
	if c.Ollama.Host == "" {
		c.Ollama.Host = "http://localhost:11434"
	}
	if c.Ollama.Timeout == 0 {
		c.Ollama.Timeout = 120
	}

	if c.Models.Default == "" {
		c.Models.Default = "qwen2.5:7b"
	}
	if c.Models.Embed == "" {
		c.Models.Embed = "qwen3-embedding:4b"
	}

	if c.Output.MaxTokens == 0 {
		c.Output.MaxTokens = 2048
	}
	if c.Output.Format == "" {
		c.Output.Format = OutputPlain
	}

	if c.Prompt.Language == "" {
		c.Prompt.Language = "english"
	}
	if c.Prompt.CodeStyle == "" {
		c.Prompt.CodeStyle = "idiomatic"
	}
	if c.Prompt.OutputLength == "" {
		c.Prompt.OutputLength = "concise"
	}

	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Environment == "" {
		c.Server.Environment = "development"
	}
	if c.GRPC.Port == 0 {
		c.GRPC.Port = 9090
	}
	if c.FX.RefreshIntervalSeconds == 0 {
		c.FX.RefreshIntervalSeconds = 3600
	}
	if c.Security.SessionTTL == 0 {
		c.Security.SessionTTL = 24 * time.Hour
	}
	if c.Security.TokenTTL == 0 {
		c.Security.TokenTTL = 30 * 24 * time.Hour
	}
	if c.Security.Vault != nil {
		if c.Security.Vault.Mount == "" {
			c.Security.Vault.Mount = "secret/data/kave"
		}
	}

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

	if c.Storage.Defaults.App.Kind == "" {
		c.Storage.Defaults.App.Kind = "sqlite"
	}
	if c.Storage.Defaults.App.Path == "" && c.Storage.Defaults.App.Kind == "sqlite" {
		c.Storage.Defaults.App.Path = "kave.db"
	}
	if c.Storage.Defaults.Span.Kind == "" {
		c.Storage.Defaults.Span.Kind = "duckdb"
	}
	if c.Storage.Defaults.Span.Path == "" && c.Storage.Defaults.Span.Kind == "duckdb" {
		c.Storage.Defaults.Span.Path = "kave-spans.duckdb"
	}
	if c.Storage.Agents == nil {
		c.Storage.Agents = map[string]AgentStorageBinding{}
	}

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
	validAppKinds := map[string]bool{"sqlite": true, "postgres": true}
	if !validAppKinds[c.Storage.Defaults.App.Kind] {
		return fmt.Errorf("invalid storage.defaults.app.kind %q", c.Storage.Defaults.App.Kind)
	}
	validSpanKinds := map[string]bool{"duckdb": true, "postgres": true, "clickhouse": true}
	if !validSpanKinds[c.Storage.Defaults.Span.Kind] {
		return fmt.Errorf("invalid storage.defaults.span.kind %q", c.Storage.Defaults.Span.Kind)
	}
	for agentID, binding := range c.Storage.Agents {
		if binding.Span == nil {
			continue
		}
		if !validSpanKinds[binding.Span.Kind] {
			return fmt.Errorf("invalid storage.agents.%s.span.kind %q", agentID, binding.Span.Kind)
		}
	}

	return nil
}

func (c *Config) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "server:   %s (%s)\n", c.Server.Addr(), c.Server.Environment)
	fmt.Fprintf(&b, "grpc:     %s\n", c.GRPC.Addr())
	fmt.Fprintf(&b, "ollama:   %s (timeout: %ds)\n", c.Ollama.Host, c.Ollama.Timeout)
	fmt.Fprintf(&b, "postgres: %s:%d/%s\n", c.Postgres.Host, c.Postgres.Port, c.Postgres.DBName)
	fmt.Fprintf(&b, "output:   format=%s max_tokens=%d\n", c.Output.Format, c.Output.MaxTokens)
	fmt.Fprintf(&b, "model:    default=%s embed=%s\n", c.Models.Default, c.Models.Embed)
	return b.String()
}
