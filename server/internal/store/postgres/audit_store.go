package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	auditmodel "github.com/kave-io/kave/core/model/audit"
	"github.com/kave-io/kave/core/store"
)

// PostgresAuditStore implements store.AuditStore using Postgres.
type PostgresAuditStore struct {
	pool *pgxpool.Pool
}

// NewAuditStore creates a new Postgres audit store.
func NewAuditStore(pool *pgxpool.Pool) *PostgresAuditStore {
	return &PostgresAuditStore{pool: pool}
}

func (p *PostgresAuditStore) AppendAudit(ctx context.Context, entry *auditmodel.AuditLog) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO audit_logs (id, org_id, project_id, env_id, actor_id, actor_type, event, resource_type, resource_id, diff_before, diff_after, ip, provenance, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		entry.ID, entry.OrgID, entry.ProjectID, entry.EnvID, entry.ActorID, entry.ActorType,
		entry.Event, entry.ResourceType, entry.ResourceID, entry.DiffBefore, entry.DiffAfter,
		entry.IP, entry.Provenance, entry.CreatedAt)
	return err
}

func (p *PostgresAuditStore) QueryAudits(ctx context.Context, filter *auditmodel.AuditFilter, page store.Page) (store.PageResult[*auditmodel.AuditLog], error) {
	if filter == nil {
		filter = &auditmodel.AuditFilter{}
	}

	query := `SELECT id, org_id, project_id, env_id, actor_id, actor_type, event, resource_type, resource_id, diff_before, diff_after, ip, provenance, created_at FROM audit_logs WHERE 1=1`
	var args []interface{}
	argIdx := 1

	if filter.OrgID != "" {
		query += ` AND org_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.OrgID)
		argIdx++
	}
	if filter.ProjectID != "" {
		query += ` AND project_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.ProjectID)
		argIdx++
	}
	if filter.EnvID != "" {
		query += ` AND env_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.EnvID)
		argIdx++
	}
	if filter.ActorID != "" {
		query += ` AND actor_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.ActorID)
		argIdx++
	}
	if filter.ResourceType != "" {
		query += ` AND resource_type = $` + strconv.Itoa(argIdx)
		args = append(args, filter.ResourceType)
		argIdx++
	}
	if filter.ResourceID != "" {
		query += ` AND resource_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.ResourceID)
		argIdx++
	}
	if filter.Event != "" {
		query += ` AND event = $` + strconv.Itoa(argIdx)
		args = append(args, filter.Event)
		argIdx++
	}
	if filter.FromMs != nil {
		query += ` AND created_at >= $` + strconv.Itoa(argIdx)
		args = append(args, *filter.FromMs)
		argIdx++
	}
	if filter.ToMs != nil {
		query += ` AND created_at <= $` + strconv.Itoa(argIdx)
		args = append(args, *filter.ToMs)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return store.PageResult[*auditmodel.AuditLog]{}, fmt.Errorf("postgres: query audit logs: %w", err)
	}
	defer rows.Close()

	var items []*auditmodel.AuditLog
	for rows.Next() {
		var entry auditmodel.AuditLog
		if err := rows.Scan(
			&entry.ID, &entry.OrgID, &entry.ProjectID, &entry.EnvID,
			&entry.ActorID, &entry.ActorType, &entry.Event, &entry.ResourceType,
			&entry.ResourceID, &entry.DiffBefore, &entry.DiffAfter, &entry.IP,
			&entry.Provenance, &entry.CreatedAt); err != nil {
			return store.PageResult[*auditmodel.AuditLog]{}, fmt.Errorf("postgres: scan audit log: %w", err)
		}
		items = append(items, &entry)
	}

	if err := rows.Err(); err != nil {
		return store.PageResult[*auditmodel.AuditLog]{}, fmt.Errorf("postgres: iterate audit logs: %w", err)
	}

	return store.Paginate(items, page), nil
}

func (p *PostgresAuditStore) Migrate(ctx context.Context) error {
	return Migrate(ctx, p.pool)
}

func (p *PostgresAuditStore) Close() error {
	p.pool.Close()
	return nil
}
