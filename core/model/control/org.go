package control

// Organization is the top-level container for all Kave resources.
// In single-org deployments (default) exactly one organization exists.
type Organization struct {
	ID        string
	Name      string
	Slug      string
	Plan      string // "free" | "team" | "enterprise"
	CreatedAt int64  // UnixMilli
	UpdatedAt int64  // UnixMilli
}

// Role constants for membership within an organization.
const (
	RoleAdmin   = "admin"   // full control
	RoleDev     = "dev"     // create/edit agents and policies; no billing
	RoleViewer  = "viewer"  // read-only
	RoleBilling = "billing" // cost and budget views only
)

// User is a human identity that authenticates to the control plane.
// v1: local email+password only. SSO/OAuth deferred.
type User struct {
	ID           string
	OrgID        string
	Email        string
	Name         string
	PasswordHash string // bcrypt
	Status       string // "active" | "suspended"
	LastLoginAt  *int64 // UnixMilli; nil = never logged in
	CreatedAt    int64  // UnixMilli
	UpdatedAt    int64  // UnixMilli
}

// UserUpdate holds partial update fields for a user.
type UserUpdate struct {
	Name        *string
	Status      *string
	LastLoginAt *int64 // UnixMilli
}

// Membership links a User to an Organization with a role.
type Membership struct {
	ID        string
	OrgID     string
	UserID    string
	Role      string  // RoleAdmin | RoleDev | RoleViewer | RoleBilling
	InvitedBy *string // user ID of inviter; nil = bootstrapped
	CreatedAt int64   // UnixMilli
}
