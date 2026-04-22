package mappers

import (
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/runtime"
)

// ActionToRecord converts a runtime Action to a persisted ActionRecord.
func ActionToRecord(a *runtime.Action) *runtimemodel.ActionRecord {
	if a == nil {
		return nil
	}

	parentID := a.ParentID
	if parentID == "" && a.InvocationRef.ParentID != nil {
		parentID = *a.InvocationRef.ParentID
	}

	return &runtimemodel.ActionRecord{
		ID:        a.ID,
		RunID:     a.RunID,
		AgentID:   a.AgentID,
		ProjectID: a.ProjectID,
		EnvID:     a.EnvID,
		ParentID:  stringPtr(parentID),

		ActionType: string(a.Type),
		Connector:  a.Connector,
		Method:     a.Method,

		Input:  a.Input,
		Output: a.Output,
		Error:  a.Error,

		StartedAt: ptrMSFromTiming(a.StartedAt),
		EndedAt:   ptrTimingToMS(a.EndedAt),
		Depth:     a.Depth,
		Seq:       a.Seq,

		Status:   string(a.Status),
		Source:   string(runtime.ActionSourceIntercepted),
		Attempt:  1,
		Metadata: outcomeToMetadata(a.Outcome),

		CreatedAt: msSinceEpoch(),
	}
}

// ObservedActionToRecord converts a reported ObservedAction to a persisted ActionRecord.
func ObservedActionToRecord(a *runtime.ObservedAction) *runtimemodel.ActionRecord {
	if a == nil {
		return nil
	}

	return &runtimemodel.ActionRecord{
		ID:        a.ID,
		RunID:     a.RunID,
		AgentID:   a.AgentID,
		ProjectID: a.ProjectID,
		EnvID:     a.EnvID,
		ParentID:  a.ParentID,

		ActionType: string(a.Type),
		Connector:  a.Connector,
		Method:     a.Method,

		Input:  a.Input,
		Output: a.Output,
		Error:  a.Error,

		StartedAt: ptrMSFromTiming(a.StartedAt),
		EndedAt:   ptrTimingToMS(a.EndedAt),
		Depth:     a.Depth,
		Seq:       a.Seq,

		Status:  string(a.Status),
		Source:  string(runtime.ActionSourceObserved),
		Attempt: 1,

		CreatedAt: msSinceEpoch(),
	}
}

// outcomeToMetadata serialises Outcome fields into the Metadata map so they survive storage.
func outcomeToMetadata(o *runtime.Outcome) map[string]any {
	if o == nil {
		return nil
	}
	return map[string]any{
		"outcome_code":    o.Code,
		"outcome_message": o.Message,
		"outcome_reason":  o.Reason,
	}
}

// RecordToAction converts a stored ActionRecord back to a runtime Action.
func RecordToAction(r *runtimemodel.ActionRecord) *runtime.Action {
	if r == nil {
		return nil
	}

	return &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ID:        r.ID,
				RunID:     r.RunID,
				AgentID:   r.AgentID,
				ProjectID: r.ProjectID,
				EnvID:     r.EnvID,
				ParentID:  r.ParentID,
			},
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.ActionType(r.ActionType),
				Connector: r.Connector,
				Method:    r.Method,
			},
			InvocationData: runtime.InvocationData{
				Input:  r.Input,
				Output: r.Output,
				Error:  r.Error,
			},
			InvocationTiming: runtime.InvocationTiming{
				StartedAt: ptrMSToTimingValue(r.StartedAt),
				EndedAt:   ptrMSToTiming(r.EndedAt),
				Depth:     r.Depth,
				Seq:       r.Seq,
			},
		},
		Status:   runtime.ActionStatus(r.Status),
		ParentID: stringValue(r.ParentID),
	}
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
