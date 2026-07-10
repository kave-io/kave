package gateway

import (
	"fmt"
	"strings"

	"github.com/kave-io/kave/core/connectors"
	"github.com/kave-io/kave/core/connectors/runtime"
	coreruntime "github.com/kave-io/kave/core/runtime"
	serverframework "github.com/kave-io/kave/server/internal/gateway/connectors/framework"
	serverllm "github.com/kave-io/kave/server/internal/gateway/connectors/llm"
	servertool "github.com/kave-io/kave/server/internal/gateway/connectors/tool"
)

// ConnectorConfig is the gateway-facing subset of the loaded connector config.
type ConnectorConfig struct {
	Name    string
	Enabled bool
	Config  map[string]any
}

// Registry resolves framework names to their LLM family bundles.
type Registry struct {
	frameworks map[string]serverframework.LLMFamily
	tools      map[string]runtime.ToolConnector
}

// NewRegistry builds the default framework registry.
func NewRegistry() *Registry {
	return NewRegistryWithConnectors(nil)
}

// NewRegistryWithConnectors builds the framework registry from loaded connector
// config. An empty connector list preserves the default provider set.
func NewRegistryWithConnectors(connectors []ConnectorConfig) *Registry {
	providers := serverllm.DefaultProviders()
	if len(connectors) > 0 {
		enabled, configs := providerConfig(connectors)
		providers = serverllm.BuildProviders(enabled, configs)
	}

	return &Registry{
		frameworks: map[string]serverframework.LLMFamily{
			"raw": serverframework.NewRawLLMFamily(providers),
		},
		tools: servertool.DefaultTools(),
	}
}

func providerConfig(connectors []ConnectorConfig) ([]string, map[string]serverllm.ProviderConfig) {
	enabled := make([]string, 0, len(connectors))
	configs := make(map[string]serverllm.ProviderConfig, len(connectors))

	for _, connector := range connectors {
		name := strings.TrimSpace(connector.Name)
		if name == "" || !connector.Enabled {
			continue
		}
		enabled = append(enabled, name)

		cfg := serverllm.ProviderConfig{}
		if baseURL, ok := stringConfig(connector.Config, "base_url"); ok {
			cfg.BaseURL = baseURL
		} else if baseURL, ok := stringConfig(connector.Config, "baseURL"); ok {
			cfg.BaseURL = baseURL
		}
		configs[name] = cfg
	}

	return enabled, configs
}

func stringConfig(config map[string]any, key string) (string, bool) {
	if config == nil {
		return "", false
	}
	value, ok := config[key]
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	return s, s != ""
}

// Register adds or replaces a framework family.
func (r *Registry) Register(family serverframework.LLMFamily) {
	if r.frameworks == nil {
		r.frameworks = make(map[string]serverframework.LLMFamily)
	}
	r.frameworks[family.Name] = family
}

// RegisterTool adds or replaces a tool connector.
func (r *Registry) RegisterTool(name string, connector runtime.ToolConnector) {
	if r.tools == nil {
		r.tools = make(map[string]runtime.ToolConnector)
	}
	r.tools[name] = connector
}

// ResolveConnector returns the LLMConnector for a given provider name (e.g. "openai", "ollama").
// It searches across all registered framework families.
func (r *Registry) ResolveConnector(provider string) (runtime.LLMConnector, error) {
	for _, family := range r.frameworks {
		if conn, ok := family.Providers[provider]; ok {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("no connector for provider %q", provider)
}

// ResolveTool returns the tool connector for a given name.
func (r *Registry) ResolveTool(name string) (runtime.ToolConnector, error) {
	if r == nil || len(r.tools) == 0 {
		return nil, fmt.Errorf("tool registry is empty")
	}
	connector, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool connector: %q", name)
	}
	return connector, nil
}

// Resolve returns the family for the given framework name.
func (r *Registry) Resolve(framework string) (serverframework.LLMFamily, error) {
	if r == nil || len(r.frameworks) == 0 {
		return serverframework.LLMFamily{}, fmt.Errorf("framework registry is empty")
	}
	family, ok := r.frameworks[framework]
	if !ok {
		return serverframework.LLMFamily{}, fmt.Errorf("unknown framework: %q", framework)
	}
	return family, nil
}

// Descriptors returns the active connector read model for CLI/dashboard display.
func (r *Registry) Descriptors() []connectors.StaticDescriptor {
	if r == nil {
		return nil
	}
	var out []connectors.StaticDescriptor
	seen := map[string]struct{}{}
	for _, family := range r.frameworks {
		out = append(out, connectors.StaticDescriptor{
			ID: family.Name,
			Caps: connectors.Capabilities{
				Kind:             connectors.KindInbound,
				SupportedActions: []coreruntime.ActionType{coreruntime.TypeLLM},
				CanProxy:         true,
			},
		})
		for name, provider := range family.Providers {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if d, ok := provider.(connectors.Descriptor); ok {
				out = append(out, connectors.StaticDescriptor{ID: d.Name(), Caps: d.Capabilities()})
			}
		}
	}
	for name, tool := range r.tools {
		if d, ok := tool.(connectors.Descriptor); ok {
			out = append(out, connectors.StaticDescriptor{ID: d.Name(), Caps: d.Capabilities()})
			continue
		}
		out = append(out, connectors.StaticDescriptor{ID: name, Caps: connectors.Capabilities{Kind: connectors.KindTool}})
	}
	return out
}
