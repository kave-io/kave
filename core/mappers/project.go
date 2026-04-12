package mappers

import controlmodel "github.com/kave-io/kave/core/model/control"

// ProjectCreateInput is control-plane input for creating a project.
type ProjectCreateInput struct {
	ID          string
	OrgID       string
	Name        string
	Slug        string
	Description string
	CreatedAt   *int64
	UpdatedAt   *int64
}

// ProjectView is the app-layer response shape for a project.
type ProjectView struct {
	ID          string
	OrgID       string
	Name        string
	Slug        string
	Description string
	CreatedAt   int64
	UpdatedAt   int64
}

// ProjectCreateToModel converts create input to controlmodel.Project.
func ProjectCreateToModel(in *ProjectCreateInput) *controlmodel.Project {
	if in == nil {
		return nil
	}

	createdAt := msSinceEpoch()
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}
	updatedAt := createdAt
	if in.UpdatedAt != nil {
		updatedAt = *in.UpdatedAt
	}

	return &controlmodel.Project{
		ID:          in.ID,
		OrgID:       in.OrgID,
		Name:        in.Name,
		Slug:        in.Slug,
		Description: in.Description,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

// ProjectToView converts a stored project to the app-layer response shape.
func ProjectToView(p *controlmodel.Project) *ProjectView {
	if p == nil {
		return nil
	}
	return &ProjectView{
		ID:          p.ID,
		OrgID:       p.OrgID,
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
