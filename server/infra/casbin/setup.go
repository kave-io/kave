package casbin

import (
	"fmt"
	"log/slog"

	casbinpkg "github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	pgxadapter "github.com/noho-digital/casbin-pgx-adapter"
	rds "github.com/redis/go-redis/v9"
)

// NewEnforcer creates a fully-wired Casbin enforcer.
//
// Behaviour by config:
//   - DatabaseDSN == ""     → in-memory only (for tests and local dev without infra)
//   - DatabaseDSN != ""     → policies persisted to the casbin postgres DB via pgx adapter
//   - PolicySyncEnabled     → a Redis pub/sub watcher is added so every policy mutation
//     is broadcast to all instances, which update their in-memory state incrementally
func NewEnforcer(cfg Config) (Casbin, error) {
	m, err := model.NewModelFromFile(cfg.CasbinModelPath)
	if err != nil {
		return nil, fmt.Errorf("casbin: load model %q: %w", cfg.CasbinModelPath, err)
	}

	e, err := casbinpkg.NewDistributedEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("casbin: create enforcer: %w", err)
	}
	e.EnableAutoSave(true)
	e.EnableAutoNotifyWatcher(true)

	if cfg.DatabaseDSN != "" {
		adapter, err := pgxadapter.NewAdapter(cfg.DatabaseDSN, pgxadapter.WithPool())
		if err != nil {
			return nil, fmt.Errorf("casbin: create pg adapter: %w", err)
		}

		e.SetAdapter(adapter)

		if err := e.LoadPolicy(); err != nil {
			return nil, fmt.Errorf("casbin: load policy from db: %w", err)
		}

		slog.Info("casbin: policy loaded from db")

		if cfg.PolicySyncEnabled && cfg.WatcherRedisAddr != "" {
			if err := wireWatcher(e, cfg); err != nil {
				return nil, err
			}
			slog.Info("casbin: policy sync watcher wired", "redis_addr", cfg.WatcherRedisAddr)
		}
	} else {
		slog.Info("casbin: running in-memory (no database DSN)")
	}

	superAdminRole := Role("")
	if cfg.SuperAdminBypass {
		superAdminRole = RolePlatformSuperAdmin
	}

	return &csbn{enforcer: e, superAdminRole: superAdminRole}, nil
}

// wireWatcher attaches a Redis pub/sub watcher to the enforcer.
// After any policy mutation, the watcher publishes a message; all instances
// receive it and update their in-memory state incrementally (no full reload).
func wireWatcher(e *casbinpkg.DistributedEnforcer, cfg Config) error {
	w, err := rediswatcher.NewWatcher(cfg.WatcherRedisAddr, rediswatcher.WatcherOptions{
		Options: rds.Options{
			Password: cfg.WatcherRedisPassword,
		},
		Channel:    "/casbin",
		IgnoreSelf: true,
	})
	if err != nil {
		return fmt.Errorf("casbin: create redis watcher: %w", err)
	}

	if err := e.SetWatcher(w); err != nil {
		return fmt.Errorf("casbin: set watcher: %w", err)
	}

	// Override the default full-reload callback with the smart incremental one:
	// UpdateForAddPolicy → SelfAddPolicy, UpdateForRemovePolicy → SelfRemovePolicy, etc.
	if err := w.SetUpdateCallback(rediswatcher.DefaultUpdateCallback(e)); err != nil {
		return fmt.Errorf("casbin: set watcher callback: %w", err)
	}

	return nil
}
