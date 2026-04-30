package control

// Environment type constants.
const (
	EnvTypeDev     = "dev"
	EnvTypeStaging = "staging"
	EnvTypeProd    = "prod"
	EnvTypeCustom  = "custom"
)

// TrustMode controls whether anonymous guest access is permitted.
type TrustMode string

const (
	TrustStrict     TrustMode = "strict"     // default — auth required
	TrustPermissive TrustMode = "permissive" // anonymous allowed when other axes also permit
)

// Environment scopes agents, policies, credentials, and runtime data within a project.
// A project has at least one environment (default: "dev").
type Environment struct {
	ID        string
	ProjectID string
	Name      string
	Slug      string    // URL-safe; matches the type for built-ins, user-defined for custom
	Type      string    // EnvTypeDev | EnvTypeStaging | EnvTypeProd | EnvTypeCustom
	TrustMode TrustMode // TrustStrict (default) | TrustPermissive
	CreatedAt int64     // UnixMilli
	UpdatedAt int64     // UnixMilli
}
