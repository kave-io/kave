package v2

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

// Namespace is Kave's immutable tenancy root: one account, application, and
// environment. Clinic/user concepts remain opaque request scopes.
type Namespace struct {
	ID          string `json:"id,omitempty"`
	Account     Ref    `json:"account"`
	Application Ref    `json:"application"`
	Environment Ref    `json:"environment"`
}

func (n Namespace) Validate() error {
	if err := n.Account.Validate("namespace.account", true); err != nil {
		return err
	}
	if err := n.Application.ValidateName("namespace.application", true); err != nil {
		return err
	}
	return n.Environment.ValidateName("namespace.environment", true)
}

type AgentKind string

const (
	AgentLLM       AgentKind = "llm"
	AgentEmbedding AgentKind = "embedding"
)

type AgentSpec struct {
	Name    Ref       `json:"name"`
	Kind    AgentKind `json:"kind"`
	Route   Ref       `json:"route"`
	Enabled bool      `json:"enabled"`
}

type RouteSpec struct {
	Name            Ref          `json:"name"`
	Provider        Ref          `json:"provider"`
	BaseURL         string       `json:"base_url,omitempty"`
	Secret          Ref          `json:"secret"`
	AllowedModels   []string     `json:"allowed_models,omitempty"`
	DefaultModel    string       `json:"default_model,omitempty"`
	PricingRevision int64        `json:"pricing_revision,omitempty"`
	Pricing         []ModelPrice `json:"pricing,omitempty"`
}

// ModelPrice is a versioned USD price snapshot used to reserve and settle
// provider-cost limits. Nano-USD per million tokens keeps all accounting in
// integers while retaining sub-cent precision.
type ModelPrice struct {
	Model                       Ref   `json:"model"`
	InputNanosPerMillionTokens  int64 `json:"input_nanos_per_million_tokens"`
	OutputNanosPerMillionTokens int64 `json:"output_nanos_per_million_tokens"`
}

// LimitSelector uses fixed admission-critical columns. Empty fields are
// wildcards. A selector with every field empty applies namespace-wide.
type LimitSelector struct {
	Tenant  Ref `json:"tenant,omitempty"`
	Actor   Ref `json:"actor,omitempty"`
	BillTo  Ref `json:"bill_to,omitempty"`
	Agent   Ref `json:"agent,omitempty"`
	Model   Ref `json:"model,omitempty"`
	Feature Ref `json:"feature,omitempty"`
}

func (s LimitSelector) Validate() error {
	for _, item := range []struct {
		name  string
		value Ref
	}{
		{name: "limit.selector.tenant", value: s.Tenant},
		{name: "limit.selector.actor", value: s.Actor},
		{name: "limit.selector.bill_to", value: s.BillTo},
		{name: "limit.selector.model", value: s.Model},
		{name: "limit.selector.feature", value: s.Feature},
	} {
		if err := item.value.Validate(item.name, false); err != nil {
			return err
		}
	}
	if err := s.Agent.ValidateName("limit.selector.agent", false); err != nil {
		return err
	}
	return nil
}

type Window string

const (
	WindowAllTime Window = "all_time"
	WindowDay     Window = "day"
	WindowMonth   Window = "month"
)

type LimitSpec struct {
	Key      Ref           `json:"key"`
	Metric   Metric        `json:"metric"`
	Selector LimitSelector `json:"selector,omitempty"`
	Window   Window        `json:"window"`
	HardCap  int64         `json:"hard_cap"`
	SoftCap  *int64        `json:"soft_cap,omitempty"`
	Enabled  bool          `json:"enabled"`
}

// Manifest describes static namespace configuration. It intentionally contains
// secret references, never secret values or encrypted blobs.
type Manifest struct {
	Namespace Namespace   `json:"namespace"`
	Routes    []RouteSpec `json:"routes,omitempty"`
	Agents    []AgentSpec `json:"agents,omitempty"`
	Limits    []LimitSpec `json:"limits,omitempty"`
}

func (m Manifest) Validate() error {
	if err := m.Namespace.Validate(); err != nil {
		return err
	}
	routes := make(map[Ref]struct{}, len(m.Routes))
	for i, route := range m.Routes {
		if err := route.validate(); err != nil {
			return fmt.Errorf("route[%d]: %w", i, err)
		}
		if _, exists := routes[route.Name]; exists {
			return invalid("routes", fmt.Sprintf("contains duplicate %q", route.Name))
		}
		routes[route.Name] = struct{}{}
	}

	agents := make(map[Ref]struct{}, len(m.Agents))
	for i, agent := range m.Agents {
		if err := agent.validate(); err != nil {
			return fmt.Errorf("agent[%d]: %w", i, err)
		}
		if _, exists := agents[agent.Name]; exists {
			return invalid("agents", fmt.Sprintf("contains duplicate %q", agent.Name))
		}
		if _, exists := routes[agent.Route]; !exists {
			return invalid("agent.route", fmt.Sprintf("%q does not reference a manifest route", agent.Route))
		}
		agents[agent.Name] = struct{}{}
	}

	limits := make(map[Ref]struct{}, len(m.Limits))
	for i, limit := range m.Limits {
		if err := limit.Validate(); err != nil {
			return fmt.Errorf("limit[%d]: %w", i, err)
		}
		if _, exists := limits[limit.Key]; exists {
			return invalid("limits", fmt.Sprintf("contains duplicate key %q", limit.Key))
		}
		if limit.Selector.Agent != "" {
			if _, exists := agents[limit.Selector.Agent]; !exists {
				return invalid("limit.selector.agent", fmt.Sprintf("%q does not reference a manifest agent", limit.Selector.Agent))
			}
		}
		limits[limit.Key] = struct{}{}
	}
	return nil
}

func (a AgentSpec) validate() error {
	if err := a.Name.ValidateName("agent.name", true); err != nil {
		return err
	}
	if err := a.Route.ValidateName("agent.route", true); err != nil {
		return err
	}
	if a.Kind != AgentLLM && a.Kind != AgentEmbedding {
		return invalid("agent.kind", "must be llm or embedding")
	}
	return nil
}

func (r RouteSpec) validate() error {
	if err := r.Name.ValidateName("route.name", true); err != nil {
		return err
	}
	if err := r.Provider.ValidateName("route.provider", true); err != nil {
		return err
	}
	if err := r.Secret.ValidateName("route.secret", true); err != nil {
		return err
	}
	if r.PricingRevision < 0 {
		return invalid("route.pricing_revision", "must not be negative")
	}
	if len(r.AllowedModels) == 0 {
		return invalid("route.allowed_models", "must declare at least one active model")
	}
	if r.DefaultModel == "" {
		return invalid("route.default_model", "is required")
	}
	if r.PricingRevision == 0 {
		return invalid("route.pricing_revision", "must be positive")
	}
	if r.BaseURL != "" {
		u, err := url.Parse(r.BaseURL)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return invalid("route.base_url", "must be an absolute HTTP(S) URL without userinfo, query, fragment, or encoded path")
		}
		if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
			return invalid("route.base_url", "plain HTTP is allowed only for loopback providers")
		}
	}
	if err := Ref(r.DefaultModel).Validate("route.default_model", false); err != nil {
		return err
	}
	if r.DefaultModel != "" && len(r.AllowedModels) > 0 && !slices.Contains(r.AllowedModels, r.DefaultModel) {
		return invalid("route.default_model", "must be included in allowed_models")
	}
	allowedModels := make(map[Ref]struct{}, len(r.AllowedModels))
	for _, model := range r.AllowedModels {
		if err := Ref(model).Validate("route.allowed_model", true); err != nil {
			return invalid("route.allowed_models", "contains an invalid model")
		}
		allowedModels[Ref(model)] = struct{}{}
	}
	pricedModels := make(map[Ref]struct{}, len(r.Pricing))
	for _, price := range r.Pricing {
		if err := price.Model.Validate("route.pricing.model", true); err != nil {
			return err
		}
		if price.InputNanosPerMillionTokens < 0 || price.OutputNanosPerMillionTokens < 0 {
			return invalid("route.pricing", "token prices must not be negative")
		}
		if _, exists := pricedModels[price.Model]; exists {
			return invalid("route.pricing", fmt.Sprintf("contains duplicate model %q", price.Model))
		}
		if len(r.AllowedModels) > 0 && !slices.Contains(r.AllowedModels, string(price.Model)) {
			return invalid("route.pricing.model", fmt.Sprintf("%q is not included in allowed_models", price.Model))
		}
		pricedModels[price.Model] = struct{}{}
	}
	for model := range allowedModels {
		if _, priced := pricedModels[model]; !priced {
			return invalid("route.pricing", fmt.Sprintf("does not cover allowed model %q", model))
		}
	}
	return nil
}

func (l LimitSpec) Validate() error {
	if err := l.Key.Validate("limit.key", true); err != nil {
		return err
	}
	if err := l.Metric.Validate(); err != nil {
		return err
	}
	if err := l.Selector.Validate(); err != nil {
		return err
	}
	if l.Window != WindowAllTime && l.Window != WindowDay && l.Window != WindowMonth {
		return invalid("limit.window", "must be all_time, day, or month")
	}
	if l.HardCap < 0 {
		return invalid("limit.hard_cap", "must not be negative")
	}
	if l.SoftCap != nil && (*l.SoftCap < 0 || *l.SoftCap > l.HardCap) {
		return invalid("limit.soft_cap", "must be between zero and hard_cap")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
