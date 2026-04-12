package mappers

import controlmodel "github.com/kave-io/kave/core/model/control"

// EnvironmentCreateInput is control-plane input for creating an environment.
type EnvironmentCreateInput struct {
	ID        string
	ProjectID string
	Name      string
	Slug      string
	Type      string // controlmodel.EnvType* constants
	CreatedAt *int64
}

// EnvironmentView is the app-layer response shape for an environment.
type EnvironmentView struct {
	ID        string
	ProjectID string
	Name      string
	Slug      string
	Type      string
	CreatedAt int64
	UpdatedAt int64
}

// EnvironmentCreateToModel converts create input to controlmodel.Environment.
func EnvironmentCreateToModel(in *EnvironmentCreateInput) *controlmodel.Environment {
	if in == nil {
		return nil
	}

	now := msSinceEpoch()
	if in.CreatedAt != nil {
		now = *in.CreatedAt
	}

	return &controlmodel.Environment{
		ID:        in.ID,
		ProjectID: in.ProjectID,
		Name:      in.Name,
		Slug:      in.Slug,
		Type:      in.Type,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// EnvironmentToView converts a stored environment to the app-layer response shape.
func EnvironmentToView(e *controlmodel.Environment) *EnvironmentView {
	if e == nil {
		return nil
	}
	return &EnvironmentView{
		ID:        e.ID,
		ProjectID: e.ProjectID,
		Name:      e.Name,
		Slug:      e.Slug,
		Type:      e.Type,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
