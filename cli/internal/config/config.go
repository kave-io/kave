package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

const (
	BuiltinLayer   = "builtin"
	SystemLayer    = "system"
	UserLayer      = "user"
	ProjectLayer   = "project"
	EnvLayer       = "env"
	ConfigNameYML  = "kave.yml"
	ConfigNameYAML = "kave.yaml"
)

type LayerFile struct {
	Layer   string `json:"layer" yaml:"layer"`
	Path    string `json:"path" yaml:"path"`
	Exists  bool   `json:"exists" yaml:"exists"`
	Loaded  bool   `json:"loaded" yaml:"loaded"`
	WhySkip string `json:"why_skip,omitempty" yaml:"why_skip,omitempty"`
}

type RootOptions struct {
	ConfigPath string
	Context    string
	Profile    string
	Server     string
	Output     string
	NoColor    bool
	Timeout    string
	Verbose    int
}

type Resolution struct {
	Options      RootOptions `json:"options" yaml:"options"`
	ProjectPath  string      `json:"project_path" yaml:"project_path"`
	UserPath     string      `json:"user_path" yaml:"user_path"`
	SystemPath   string      `json:"system_path" yaml:"system_path"`
	ConfigPath   string      `json:"config_path" yaml:"config_path"`
	LayerFiles   []LayerFile `json:"layer_files" yaml:"layer_files"`
	LoadedConfig *Document   `json:"loaded_config,omitempty" yaml:"loaded_config,omitempty"`
}

func (r *Resolution) ActiveContextName() string {
	if r == nil {
		return ""
	}
	for _, candidate := range []string{r.Options.Context, r.Options.Profile} {
		if candidate != "" {
			return candidate
		}
	}
	if r.LoadedConfig != nil && r.LoadedConfig.CurrentContext != "" {
		return r.LoadedConfig.CurrentContext
	}
	return ""
}

func (r *Resolution) ActiveContext() *ContextConfig {
	if r == nil || r.LoadedConfig == nil {
		return nil
	}
	name := r.ActiveContextName()
	if name == "" {
		if len(r.LoadedConfig.Contexts) > 0 {
			ctx := r.LoadedConfig.Contexts[0]
			return &ctx
		}
		return nil
	}
	for _, ctx := range r.LoadedConfig.Contexts {
		if ctx.Name == name {
			c := ctx
			return &c
		}
	}
	return nil
}

func (r *Resolution) ActiveServer() string {
	if r == nil {
		return ""
	}
	if server := strings.TrimSpace(r.Options.Server); server != "" {
		return server
	}
	if ctx := r.ActiveContext(); ctx != nil && strings.TrimSpace(ctx.Server) != "" {
		return ctx.Server
	}
	if r.LoadedConfig != nil && r.LoadedConfig.Daemon != nil {
		if addr := strings.TrimSpace(r.LoadedConfig.Daemon.Address); addr != "" {
			return addr
		}
		if proxy := strings.TrimSpace(r.LoadedConfig.Daemon.ProxyAddress); proxy != "" {
			return proxy
		}
	}
	return ""
}

type Document struct {
	APIVersion     string             `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind           string             `json:"kind,omitempty" yaml:"kind,omitempty"`
	Daemon         *DaemonConfig      `json:"daemon,omitempty" yaml:"daemon,omitempty"`
	Storage        *StorageConfig     `json:"storage,omitempty" yaml:"storage,omitempty"`
	Contexts       []ContextConfig    `json:"contexts,omitempty" yaml:"contexts,omitempty"`
	CurrentContext string             `json:"currentContext,omitempty" yaml:"currentContext,omitempty"`
	Project        *ProjectConfig     `json:"project,omitempty" yaml:"project,omitempty"`
	Connectors     []ConnectorConfig  `json:"connectors,omitempty" yaml:"connectors,omitempty"`
	Credentials    []CredentialConfig `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Policies       []PolicyConfig     `json:"policies,omitempty" yaml:"policies,omitempty"`
	Agents         []AgentConfig      `json:"agents,omitempty" yaml:"agents,omitempty"`
	Pricing        *PricingConfig     `json:"pricing,omitempty" yaml:"pricing,omitempty"`
	UI             *UIConfig          `json:"ui,omitempty" yaml:"ui,omitempty"`
	Extra          map[string]any     `json:"-" yaml:",inline"`
}

type DaemonConfig struct {
	Address      string `json:"address,omitempty" yaml:"address,omitempty"`
	ProxyAddress string `json:"proxy_address,omitempty" yaml:"proxy_address,omitempty"`
	DataDir      string `json:"data_dir,omitempty" yaml:"data_dir,omitempty"`
	LogLevel     string `json:"log_level,omitempty" yaml:"log_level,omitempty"`
	LogFormat    string `json:"log_format,omitempty" yaml:"log_format,omitempty"`
}

type ContextConfig struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Server  string `json:"server,omitempty" yaml:"server,omitempty"`
	User    string `json:"user,omitempty" yaml:"user,omitempty"`
	Project string `json:"project,omitempty" yaml:"project,omitempty"`
	Env     string `json:"env,omitempty" yaml:"env,omitempty"`
}

type StoreSpec struct {
	Kind         string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Path         string `json:"path,omitempty" yaml:"path,omitempty"`
	PathTemplate string `json:"path_template,omitempty" yaml:"path_template,omitempty"`
	DSN          string `json:"dsn,omitempty" yaml:"dsn,omitempty"`
}

type StorageDefaults struct {
	App  StoreSpec `json:"app,omitempty" yaml:"app,omitempty"`
	Span StoreSpec `json:"span,omitempty" yaml:"span,omitempty"`
}

type AgentStorageBinding struct {
	Span *StoreSpec `json:"span,omitempty" yaml:"span,omitempty"`
}

type StorageConfig struct {
	Defaults StorageDefaults                `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Agents   map[string]AgentStorageBinding `json:"agents,omitempty" yaml:"agents,omitempty"`
}

type ProjectEnv struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	Slug string `json:"slug,omitempty" yaml:"slug,omitempty"`
}

type ProjectConfig struct {
	Name        string       `json:"name,omitempty" yaml:"name,omitempty"`
	Slug        string       `json:"slug,omitempty" yaml:"slug,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Envs        []ProjectEnv `json:"envs,omitempty" yaml:"envs,omitempty"`
	DefaultEnv  string       `json:"defaultEnv,omitempty" yaml:"defaultEnv,omitempty"`
}

type ConnectorConfig struct {
	Name    string         `json:"name,omitempty" yaml:"name,omitempty"`
	Enabled bool           `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Config  map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

type CredentialConfig struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Connector string `json:"connector,omitempty" yaml:"connector,omitempty"`
	Label     string `json:"label,omitempty" yaml:"label,omitempty"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
	Env       string `json:"env,omitempty" yaml:"env,omitempty"`
	File      string `json:"file,omitempty" yaml:"file,omitempty"`
	Mode      string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Ref       string `json:"ref,omitempty" yaml:"ref,omitempty"`
}

type PolicyConfig struct {
	Name        string         `json:"name,omitempty" yaml:"name,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Mode        string         `json:"mode,omitempty" yaml:"mode,omitempty"`
	Auth        map[string]any `json:"auth,omitempty" yaml:"auth,omitempty"`
	Cost        map[string]any `json:"cost,omitempty" yaml:"cost,omitempty"`
	Trace       map[string]any `json:"trace,omitempty" yaml:"trace,omitempty"`
	Validation  map[string]any `json:"validation,omitempty" yaml:"validation,omitempty"`
}

type AgentConfig struct {
	Name          string         `json:"name,omitempty" yaml:"name,omitempty"`
	Description   string         `json:"description,omitempty" yaml:"description,omitempty"`
	Env           string         `json:"env,omitempty" yaml:"env,omitempty"`
	Policy        string         `json:"policy,omitempty" yaml:"policy,omitempty"`
	Credentials   []string       `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	MonthlyBudget *Money         `json:"monthlyBudget,omitempty" yaml:"monthlyBudget,omitempty"`
	Status        string         `json:"status,omitempty" yaml:"status,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type Money struct {
	Amount   string `json:"amount,omitempty" yaml:"amount,omitempty"`
	Currency string `json:"currency,omitempty" yaml:"currency,omitempty"`
}

type PricingConfig struct {
	PriceBook string           `json:"priceBook,omitempty" yaml:"priceBook,omitempty"`
	Overrides []map[string]any `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

type UIConfig struct {
	PreferredCurrency string `json:"preferredCurrency,omitempty" yaml:"preferredCurrency,omitempty"`
	Locale            string `json:"locale,omitempty" yaml:"locale,omitempty"`
	ColorMode         string `json:"colorMode,omitempty" yaml:"colorMode,omitempty"`
	DateStyle         string `json:"dateStyle,omitempty" yaml:"dateStyle,omitempty"`
}

type LoadOptions struct {
	ConfigPath string
	Cwd        string
	HomeDir    string
}

func DiscoverProjectConfig(startDir string) (string, error) {
	dir := startDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	home, _ := os.UserHomeDir()
	for {
		for _, name := range []string{ConfigNameYAML, ConfigNameYML} {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if home != "" {
			if absDir, err := filepath.Abs(dir); err == nil {
				if absHome, err := filepath.Abs(home); err == nil && strings.EqualFold(absDir, absHome) {
					break
				}
			}
		}
		dir = parent
	}

	return "", errors.New("project config not found")
}

func Resolve(opts RootOptions) (*Resolution, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	projectPath := opts.ConfigPath
	selectedFromEnv := false
	envConfigPath := os.Getenv("KAVE_CONFIG")
	if projectPath == "" && envConfigPath != "" {
		projectPath = envConfigPath
		selectedFromEnv = true
	}
	if projectPath == "" {
		if discovered, err := DiscoverProjectConfig(""); err == nil {
			projectPath = discovered
		}
	}

	systemPath := filepath.Join(string(os.PathSeparator), "etc", "kave", ConfigNameYAML)
	if runtime.GOOS == "windows" {
		programData := os.Getenv("PROGRAMDATA")
		if programData != "" {
			systemPath = filepath.Join(programData, "kave", ConfigNameYAML)
		}
	}

	resolution := &Resolution{
		Options:     opts,
		ProjectPath: projectPath,
		UserPath:    filepath.Join(home, ".kave", ConfigNameYAML),
		SystemPath:  systemPath,
		ConfigPath:  projectPath,
		LayerFiles: []LayerFile{
			{Layer: BuiltinLayer, Path: "<compiled>", Exists: true, Loaded: true},
		},
	}

	if resolution.SystemPath != "" {
		resolution.LayerFiles = append(resolution.LayerFiles, LayerFile{
			Layer:  SystemLayer,
			Path:   resolution.SystemPath,
			Exists: pathExists(resolution.SystemPath),
		})
	}
	if resolution.UserPath != "" {
		resolution.LayerFiles = append(resolution.LayerFiles, LayerFile{
			Layer:  UserLayer,
			Path:   resolution.UserPath,
			Exists: pathExists(resolution.UserPath),
		})
	}
	if resolution.ProjectPath != "" {
		resolution.LayerFiles = append(resolution.LayerFiles, LayerFile{
			Layer:  ProjectLayer,
			Path:   resolution.ProjectPath,
			Exists: pathExists(resolution.ProjectPath),
			Loaded: pathExists(resolution.ProjectPath),
		})
	}

	if resolution.ProjectPath == "" && opts.ConfigPath != "" {
		return nil, fmt.Errorf("config file %q not found", opts.ConfigPath)
	}

	if envConfigPath != "" {
		resolution.LayerFiles = append(resolution.LayerFiles, LayerFile{
			Layer:  EnvLayer,
			Path:   envConfigPath,
			Exists: pathExists(envConfigPath),
			Loaded: pathExists(envConfigPath),
		})
		if selectedFromEnv {
			resolution.ConfigPath = envConfigPath
		}
	}

	if resolution.ConfigPath != "" && pathExists(resolution.ConfigPath) {
		raw, err := os.ReadFile(resolution.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read config %q: %w", resolution.ConfigPath, err)
		}
		var doc Document
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse config %q: %w", resolution.ConfigPath, err)
		}
		if _, ok := doc.Extra["stores"]; ok {
			return nil, fmt.Errorf("configuration key \"stores\" has been removed; use \"storage.defaults.app\" and \"storage.defaults.span\"")
		}
		resolution.LoadedConfig = &doc
	}

	return resolution, nil
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
