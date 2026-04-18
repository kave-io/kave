package mappers

import (
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime/policy"
)

// RecordToPolicy converts a stored model PolicyRecord to a runtime Policy.
func RecordToPolicy(r *controlmodel.PolicyRecord) *policy.Policy {
	if r == nil {
		return nil
	}

	p := &policy.Policy{
		ID:        r.ID,
		ProjectID: r.ProjectID,
		Name:      r.Name,
		Mode:      policy.Mode(r.Mode),
		CreatedAt: msToTimingValue(r.CreatedAt),
		UpdatedAt: msToTimingValue(r.UpdatedAt),
	}
	if p.Mode == "" {
		p.Mode = policy.ModeEnforce
	}

	cfg := r.Config
	if cfg == nil {
		cfg = make(map[string]any)
	}

	p.Auth = &policy.AuthPolicy{
		AllowedTypes:      r.AllowedTypes,
		AllowedConnectors: r.AllowedConnectors,
		AllowedMethods:    r.AllowedMethods,
	}

	var budgetCap *money.Amount
	if r.BudgetCap > 0 {
		cap := r.BudgetCap
		budgetCap = &cap
	}

	budgetPeriod := policy.BudgetPeriod(r.BudgetPeriod)
	if budgetPeriod == "" {
		budgetPeriod = policy.BudgetPerRun
	}
	budgetBehavior := policy.BudgetBehavior(r.BudgetBehavior)
	if budgetBehavior == "" {
		budgetBehavior = policy.BudgetBlock
	}

	p.Cost = &policy.CostPolicy{
		BudgetCap:      budgetCap,
		BudgetPeriod:   budgetPeriod,
		BudgetBehavior: budgetBehavior,
	}

	retentionDays := r.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}

	p.Trace = &policy.TracePolicy{
		Input:         r.TraceInput,
		Output:        r.TraceOutput,
		RetentionDays: retentionDays,
	}

	p.Validation = &policy.ValidationPolicy{
		Enabled:   false,
		Retryable: false,
		Config:    cfg,
	}

	return p
}

// PolicyToRecord converts a runtime Policy to a stored model PolicyRecord.
func PolicyToRecord(p *policy.Policy) *controlmodel.PolicyRecord {
	if p == nil {
		return nil
	}

	cfg := make(map[string]any)
	if p.Validation != nil && p.Validation.Config != nil {
		for k, v := range p.Validation.Config {
			cfg[k] = v
		}
	}

	var budgetCap money.Amount
	var budgetPeriod, budgetBehavior string
	if p.Cost != nil {
		if p.Cost.BudgetCap != nil {
			budgetCap = *p.Cost.BudgetCap
		}
		budgetPeriod = string(p.Cost.BudgetPeriod)
		budgetBehavior = string(p.Cost.BudgetBehavior)
	}

	var traceInput, traceOutput bool
	var retentionDays int
	if p.Trace != nil {
		traceInput = p.Trace.Input
		traceOutput = p.Trace.Output
		retentionDays = p.Trace.RetentionDays
	}

	var allowedTypes, allowedConnectors, allowedMethods []string
	if p.Auth != nil {
		allowedTypes = p.Auth.AllowedTypes
		allowedConnectors = p.Auth.AllowedConnectors
		allowedMethods = p.Auth.AllowedMethods
	}

	mode := p.Mode
	if mode == "" {
		mode = policy.ModeEnforce
	}

	return &controlmodel.PolicyRecord{
		ID:                p.ID,
		ProjectID:         p.ProjectID,
		Name:              p.Name,
		AllowedTypes:      allowedTypes,
		AllowedConnectors: allowedConnectors,
		AllowedMethods:    allowedMethods,
		BudgetCap:         budgetCap,
		BudgetPeriod:      budgetPeriod,
		BudgetBehavior:    budgetBehavior,
		TraceInput:        traceInput,
		TraceOutput:       traceOutput,
		RetentionDays:     retentionDays,
		Config:            cfg,
		Mode:              string(mode),
		Status:            string(controlmodel.PolicyStatusActive),
		CreatedAt:         timingToMS(p.CreatedAt),
		UpdatedAt:         timingToMS(p.UpdatedAt),
	}
}
