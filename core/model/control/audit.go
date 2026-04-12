package control

// AuditLog is an append-only record of every control-plane mutation.
// Never updated or deleted — compliance requirement.
type AuditLog struct {
	ID           string
	OrgID        string
	ProjectID    *string // nil for org-level events
	EnvID        *string // nil for project-level events
	ActorID      string  // user ID, machine identity ID, or "system"
	ActorType    string  // "user" | "api_key" | "system"
	Event        string  // "policy.created" | "credential.rotated" | "token.revoked" | ...
	ResourceType string  // "policy" | "credential" | "agent" | "token" | "user" | ...
	ResourceID   string
	Diff         []byte  // JSON: {"before":{...},"after":{...}}; null for creates/deletes
	IP           *string // source IP; nil for system events
	CreatedAt    int64   // UnixMilli
}

// AuditFilter filters audit log queries.
type AuditFilter struct {
	OrgID        string
	ProjectID    string
	EnvID        string
	ActorID      string
	ResourceType string
	ResourceID   string
	Event        string
	FromMs       *int64
	ToMs         *int64
	Limit        int
}
