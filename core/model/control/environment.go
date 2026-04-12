package control

// Environment type constants.
const (
	EnvTypeDev     = "dev"
	EnvTypeStaging = "staging"
	EnvTypeProd    = "prod"
	EnvTypeCustom  = "custom"
)

// Environment scopes agents, policies, credentials, and runtime data within a project.
// A project has at least one environment (default: "dev").
type Environment struct {
	ID        string
	ProjectID string
	Name      string
	Slug      string // URL-safe; matches the type for built-ins, user-defined for custom
	Type      string // EnvTypeDev | EnvTypeStaging | EnvTypeProd | EnvTypeCustom
	CreatedAt int64  // UnixMilli
	UpdatedAt int64  // UnixMilli
}
