package mappers

import (
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime"
)

// RunToRecord converts a runtime Run to a persisted model RunRecord.
// Caller must provide budgetCap (typically from the resolved policy; 0 = no cap).
func RunToRecord(r *runtime.Run, budgetCap money.Amount) *runtimemodel.RunRecord {
	if r == nil {
		return nil
	}

	now := msSinceEpoch()
	return &runtimemodel.RunRecord{
		ID:        r.ID,
		ProjectID: r.ProjectID,
		EnvID:     r.EnvID,
		AgentID:   r.AgentID,
		PolicyID:  r.PolicyID,
		Name:      r.Name,
		Status:    string(r.Status),
		BudgetCap: budgetCap,
		Spent:     r.Spent,

		TriggerType:   r.TriggerType,
		CorrelationID: r.CorrelationID,
		SessionID:     r.SessionID,

		ErrorMessage: r.Error,
		StartedAt:    timingToMS(r.StartedAt),
		EndedAt:      ptrMSFromTiming(r.EndedAt),

		Metadata:  r.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// RecordToRun converts a stored model RunRecord back to a runtime Run.
func RecordToRun(r *runtimemodel.RunRecord) *runtime.Run {
	if r == nil {
		return nil
	}

	return &runtime.Run{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		EnvID:         r.EnvID,
		AgentID:       r.AgentID,
		PolicyID:      r.PolicyID,
		Name:          r.Name,
		Status:        runtime.RunStatus(r.Status),
		StartedAt:     msToTimingValue(r.StartedAt),
		EndedAt:       ptrMSToTimingValue(r.EndedAt),
		Spent:         r.Spent,
		Error:         r.ErrorMessage,
		TriggerType:   r.TriggerType,
		CorrelationID: r.CorrelationID,
		SessionID:     r.SessionID,
		Metadata:      r.Metadata,
	}
}

// RunUpdate converts a runtime Run state to a model RunUpdate.
// Only non-default fields are included (nil = not updated).
func RunUpdate(r *runtime.Run) *runtimemodel.RunUpdate {
	if r == nil {
		return nil
	}

	status := r.Status
	spent := r.Spent
	endedAt := r.EndedAt

	return &runtimemodel.RunUpdate{
		Status:       (*string)(&status),
		Spent:        &spent,
		ErrorMessage: r.Error,
		EndedAt:      ptrMS(endedAt),
		Metadata:     r.Metadata,
	}
}
