package v2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultReadPageSize = 50
	MaxReadPageSize     = 200
	MaxReadPageToken    = 512
	MaxReadRange        = 366 * 24 * time.Hour
)

var ErrNamespaceNotFound = errors.New("kave v2: namespace not found")

// State is the current operator-owned declarative state. It contains secret
// names only; secret values and encrypted material are never part of this API.
type State struct {
	NamespaceID Ref
	Revision    int64
	Manifest    Manifest
}

type GetStateRequest struct {
	Caller      Caller
	NamespaceID Ref
}

func (r GetStateRequest) Validate() error {
	if r.Caller.Bootstrap {
		return ErrUnauthorized
	}
	return r.Caller.AuthorizeControl(r.NamespaceID, OperationConfigApply)
}

type LimitStatus struct {
	LimitID  Ref
	LimitKey Ref
	Metric   Metric
	Used     int64
	Reserved int64
	HardCap  int64
	SoftCap  *int64
	ResetAt  time.Time
}

type GetLimitStatusRequest struct {
	Caller Caller
	Scope  Scope
	Agent  Ref
	Model  Ref
	Metric Metric
}

func (r GetLimitStatusRequest) Validate() error {
	if err := validateNamespaceCaller(r.Caller); err != nil {
		return err
	}
	if err := r.Scope.ValidateAdmission(); err != nil {
		return err
	}
	if err := r.Agent.ValidateName("agent", true); err != nil {
		return err
	}
	if err := r.Model.Validate("model", false); err != nil {
		return err
	}
	if err := r.Metric.Validate(); err != nil {
		return err
	}
	if !r.Caller.CanAssertScope {
		return fmt.Errorf("%w: service key cannot assert the requested scope", ErrUnauthorized)
	}
	if !r.Caller.Allows(OperationUsageRead, "") &&
		!r.Caller.Allows(OperationConsume, r.Agent) &&
		!r.Caller.Allows(OperationInvoke, r.Agent) {
		return ErrUnauthorized
	}
	return nil
}

type TimeRange struct {
	From time.Time
	To   time.Time
}

func (r TimeRange) validate() error {
	if r.From.IsZero() || r.To.IsZero() {
		return invalid("time_range", "from and to are required")
	}
	from, to := r.From.UTC(), r.To.UTC()
	if !from.Before(to) {
		return invalid("time_range", "from must be before to")
	}
	if to.Sub(from) > MaxReadRange {
		return invalid("time_range", "must not exceed 366 days")
	}
	return nil
}

type Page struct {
	Size  int
	Token string
}

func (p Page) Validate() error {
	if p.Size < 0 || p.Size > MaxReadPageSize {
		return invalid("page_size", fmt.Sprintf("must be between zero and %d", MaxReadPageSize))
	}
	if len(p.Token) > MaxReadPageToken || strings.ContainsAny(p.Token, "\r\n \t") {
		return invalid("page_token", "is invalid")
	}
	return nil
}

func (p Page) EffectiveSize() int {
	if p.Size == 0 {
		return DefaultReadPageSize
	}
	return p.Size
}

type UsageEntry struct {
	ID               Ref
	InvocationID     Ref
	Metric           Metric
	Quantity         int64
	RequestCount     int64
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostNanoUSD      int64
	Estimated        bool
	Provider         Ref
	Model            Ref
	Attempt          int32
	EventKind        Ref
	CreatedAt        time.Time
}

type QueryUsageRequest struct {
	Caller Caller
	Scope  Scope
	Agent  Ref
	Metric Metric
	Range  TimeRange
	Page   Page
}

func (r QueryUsageRequest) Validate() error {
	if err := authorizeReporting(r.Caller, OperationUsageRead); err != nil {
		return err
	}
	if err := r.Scope.ValidateAdmission(); err != nil {
		return err
	}
	if err := r.Agent.ValidateName("agent", false); err != nil {
		return err
	}
	if r.Metric != "" {
		if err := r.Metric.Validate(); err != nil {
			return err
		}
	}
	if err := r.Range.validate(); err != nil {
		return err
	}
	return r.Page.Validate()
}

type QueryUsageResult struct {
	Entries       []UsageEntry
	NextPageToken string
}

type Invocation struct {
	ID             Ref
	Agent          Ref
	Model          Ref
	Scope          Scope
	Decision       DecisionStatus
	Status         Ref
	IdempotencyKey Ref
	CreatedAt      time.Time
	SettledAt      *time.Time
}

type QueryInvocationsRequest struct {
	Caller Caller
	Scope  Scope
	Agent  Ref
	Status DecisionStatus
	Range  TimeRange
	Page   Page
}

func (r QueryInvocationsRequest) Validate() error {
	if err := authorizeReporting(r.Caller, OperationUsageRead); err != nil {
		return err
	}
	if err := r.Scope.ValidateAdmission(); err != nil {
		return err
	}
	if err := r.Agent.ValidateName("agent", false); err != nil {
		return err
	}
	if r.Status != "" && r.Status != DecisionAdmitted && r.Status != DecisionRejected {
		return invalid("status", "must be admitted, rejected, or omitted")
	}
	if err := r.Range.validate(); err != nil {
		return err
	}
	return r.Page.Validate()
}

type QueryInvocationsResult struct {
	Invocations   []Invocation
	NextPageToken string
}

// TenantStatus is operational state inferred from namespace-scoped accounting
// records. It is not an identity, lifecycle, or customer-record status.
type TenantStatus string

const (
	// TenantStatusActive means at least one current limit explicitly targets the
	// tenant or billing reference.
	TenantStatusActive TenantStatus = "active"
	// TenantStatusObserved means activity was seen in the requested interval,
	// but no current tenant-targeted limit exists.
	TenantStatusObserved TenantStatus = "observed"
)

// TenantSummary contains only opaque application-provided scope references and
// bounded operational aggregates. Kave deliberately does not attach names,
// email addresses, or any other human identity data to these references.
type TenantSummary struct {
	Tenant          Ref
	BillTo          Ref
	Status          TenantStatus
	LastSeenAt      *time.Time
	InvocationCount int64
	RequestCount    int64
	CostNanoUSD     int64
	ActiveLimits    int32
}

type ListTenantsRequest struct {
	Caller Caller
	Range  TimeRange
	Page   Page
}

func (r ListTenantsRequest) Validate() error {
	if err := authorizeReporting(r.Caller, OperationUsageRead); err != nil {
		return err
	}
	if err := r.Range.validate(); err != nil {
		return err
	}
	return r.Page.Validate()
}

type ListTenantsResult struct {
	Tenants       []TenantSummary
	NextPageToken string
}

type AuditEvent struct {
	ID           Ref
	EventKind    Ref
	ActorKind    Ref
	ActorID      Ref
	ResourceKind Ref
	ResourceID   Ref
	Outcome      Ref
	Metadata     map[string]string
	CreatedAt    time.Time
}

type QueryAuditEventsRequest struct {
	Caller    Caller
	EventKind Ref
	Range     TimeRange
	Page      Page
}

func (r QueryAuditEventsRequest) Validate() error {
	if err := authorizeReporting(r.Caller, OperationAuditRead); err != nil {
		return err
	}
	if err := r.EventKind.Validate("event_kind", false); err != nil {
		return err
	}
	if err := r.Range.validate(); err != nil {
		return err
	}
	return r.Page.Validate()
}

type QueryAuditEventsResult struct {
	Events        []AuditEvent
	NextPageToken string
}

func validateNamespaceCaller(c Caller) error {
	if c.Bootstrap {
		return ErrUnauthorized
	}
	if err := c.AccountID.Validate("caller.account_id", true); err != nil {
		return err
	}
	if err := c.NamespaceID.Validate("caller.namespace_id", true); err != nil {
		return err
	}
	return c.ServiceKeyID.Validate("caller.service_key_id", true)
}

func authorizeReporting(c Caller, operation Operation) error {
	if err := validateNamespaceCaller(c); err != nil {
		return err
	}
	if !c.Allows(operation, "") {
		return ErrUnauthorized
	}
	return nil
}

type ReadStore interface {
	GetState(context.Context, GetStateRequest) (State, error)
	GetLimitStatus(context.Context, GetLimitStatusRequest) ([]LimitStatus, error)
	QueryUsage(context.Context, QueryUsageRequest) (QueryUsageResult, error)
	QueryInvocations(context.Context, QueryInvocationsRequest) (QueryInvocationsResult, error)
	ListTenants(context.Context, ListTenantsRequest) (ListTenantsResult, error)
	QueryAuditEvents(context.Context, QueryAuditEventsRequest) (QueryAuditEventsResult, error)
}

type ReadService struct{ store ReadStore }

func NewReadService(store ReadStore) *ReadService { return &ReadService{store: store} }

func (s *ReadService) GetState(ctx context.Context, req GetStateRequest) (State, error) {
	if err := req.Validate(); err != nil {
		return State{}, err
	}
	if s == nil || s.store == nil {
		return State{}, errors.New("kave v2: read store unavailable")
	}
	return s.store.GetState(ctx, req)
}

func (s *ReadService) GetLimitStatus(ctx context.Context, req GetLimitStatusRequest) ([]LimitStatus, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if s == nil || s.store == nil {
		return nil, errors.New("kave v2: read store unavailable")
	}
	return s.store.GetLimitStatus(ctx, req)
}

func (s *ReadService) QueryUsage(ctx context.Context, req QueryUsageRequest) (QueryUsageResult, error) {
	if err := req.Validate(); err != nil {
		return QueryUsageResult{}, err
	}
	if s == nil || s.store == nil {
		return QueryUsageResult{}, errors.New("kave v2: read store unavailable")
	}
	return s.store.QueryUsage(ctx, req)
}

func (s *ReadService) QueryInvocations(ctx context.Context, req QueryInvocationsRequest) (QueryInvocationsResult, error) {
	if err := req.Validate(); err != nil {
		return QueryInvocationsResult{}, err
	}
	if s == nil || s.store == nil {
		return QueryInvocationsResult{}, errors.New("kave v2: read store unavailable")
	}
	return s.store.QueryInvocations(ctx, req)
}

func (s *ReadService) ListTenants(ctx context.Context, req ListTenantsRequest) (ListTenantsResult, error) {
	if err := req.Validate(); err != nil {
		return ListTenantsResult{}, err
	}
	if s == nil || s.store == nil {
		return ListTenantsResult{}, errors.New("kave v2: read store unavailable")
	}
	return s.store.ListTenants(ctx, req)
}

func (s *ReadService) QueryAuditEvents(ctx context.Context, req QueryAuditEventsRequest) (QueryAuditEventsResult, error) {
	if err := req.Validate(); err != nil {
		return QueryAuditEventsResult{}, err
	}
	if s == nil || s.store == nil {
		return QueryAuditEventsResult{}, errors.New("kave v2: read store unavailable")
	}
	return s.store.QueryAuditEvents(ctx, req)
}
