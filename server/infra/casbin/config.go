package casbin

// Config holds configuration for the authorization system.
type Config struct {
	// CasbinModelPath is the path to the Casbin model configuration file.
	CasbinModelPath string

	// EnableAudit enables audit logging for all authorization decisions.
	EnableAudit bool

	// SuperAdminBypass allows RolePlatformSuperAdmin in "sys" domain to bypass checks.
	// Keep enabled for operational break-glass.
	SuperAdminBypass bool

	// DatabaseDSN is the postgres connection string for the casbin policy store.
	// When empty the enforcer runs in-memory only (useful in tests).
	DatabaseDSN string

	// PolicySyncEnabled wires a Redis pub/sub watcher so that policy changes
	// made on any instance are immediately reflected in all others.
	// Requires WatcherRedisAddr to be set.
	PolicySyncEnabled bool

	// WatcherRedisAddr is the Redis address used by the pub/sub watcher
	// (e.g. "redis:6379").  Only used when PolicySyncEnabled is true.
	WatcherRedisAddr string

	// WatcherRedisPassword is the Redis password for the watcher connection.
	WatcherRedisPassword string
}
