package control

// Project is the tenancy workspace. Everything belongs to a project.
// A project belongs to one organization.
type Project struct {
	ID          string
	OrgID       string
	Name        string
	Slug        string
	Description string
	CreatedAt   int64 // UnixMilli
	UpdatedAt   int64 // UnixMilli
}
