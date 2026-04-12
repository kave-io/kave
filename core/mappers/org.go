package mappers

import controlmodel "github.com/kave-io/kave/core/model/control"

// OrgCreateInput is control-plane input for creating an organization.
type OrgCreateInput struct {
	ID        string
	Name      string
	Slug      string
	Plan      string
	CreatedAt *int64
}

// OrgView is the app-layer response shape for an organization.
type OrgView struct {
	ID        string
	Name      string
	Slug      string
	Plan      string
	CreatedAt int64
	UpdatedAt int64
}

// OrgCreateToModel converts create input to controlmodel.Organization.
func OrgCreateToModel(in *OrgCreateInput) *controlmodel.Organization {
	if in == nil {
		return nil
	}

	now := msSinceEpoch()
	if in.CreatedAt != nil {
		now = *in.CreatedAt
	}

	return &controlmodel.Organization{
		ID:        in.ID,
		Name:      in.Name,
		Slug:      in.Slug,
		Plan:      in.Plan,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// OrgToView converts a stored organization to the app-layer response shape.
func OrgToView(o *controlmodel.Organization) *OrgView {
	if o == nil {
		return nil
	}
	return &OrgView{
		ID:        o.ID,
		Name:      o.Name,
		Slug:      o.Slug,
		Plan:      o.Plan,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}

// UserCreateInput is control-plane input for creating a user.
type UserCreateInput struct {
	ID           string
	OrgID        string
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    *int64
}

// UserView is the response-safe shape for a user (no password hash).
type UserView struct {
	ID          string
	OrgID       string
	Email       string
	Name        string
	Status      string
	LastLoginAt *int64
	CreatedAt   int64
	UpdatedAt   int64
}

// UserCreateToModel converts create input to controlmodel.User.
func UserCreateToModel(in *UserCreateInput) *controlmodel.User {
	if in == nil {
		return nil
	}

	now := msSinceEpoch()
	if in.CreatedAt != nil {
		now = *in.CreatedAt
	}

	return &controlmodel.User{
		ID:           in.ID,
		OrgID:        in.OrgID,
		Email:        in.Email,
		Name:         in.Name,
		PasswordHash: in.PasswordHash,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// UserToView converts a stored user to the response-safe shape.
func UserToView(u *controlmodel.User) *UserView {
	if u == nil {
		return nil
	}
	return &UserView{
		ID:          u.ID,
		OrgID:       u.OrgID,
		Email:       u.Email,
		Name:        u.Name,
		Status:      u.Status,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
