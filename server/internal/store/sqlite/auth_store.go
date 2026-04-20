package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/store"
)

// ── Sessions ────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) InsertSession(ctx context.Context, session *control.Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.OrgID, session.UserID, session.TokenHash, session.ExpiresAt,
		session.CreatedAt, session.LastUsedAt, session.UserAgent, session.IP, session.RevokedAt)
	return err
}

func (s *SQLiteAppStore) GetSessionByHash(ctx context.Context, hash string) (*control.Session, error) {
	return s.scanSession(s.db.QueryRowContext(ctx, `
		SELECT id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at
		FROM sessions WHERE token_hash = ?`, []byte(hash)))
}

func (s *SQLiteAppStore) GetSession(ctx context.Context, id string) (*control.Session, error) {
	return s.scanSession(s.db.QueryRowContext(ctx, `
		SELECT id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at
		FROM sessions WHERE id = ?`, id))
}

func (s *SQLiteAppStore) ListSessions(ctx context.Context, userID string, page store.Page) (store.PageResult[*control.Session], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at
		FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return store.PageResult[*control.Session]{}, err
	}
	defer rows.Close()
	var items []*control.Session
	for rows.Next() {
		item, err := scanSessionRow(rows)
		if err != nil {
			return store.PageResult[*control.Session]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) RevokeSession(ctx context.Context, sessionID, revokedBy string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ?`, now, sessionID)
	return err
}

func (s *SQLiteAppStore) TouchSession(ctx context.Context, sessionID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_used_at = ? WHERE id = ?`, now, sessionID)
	return err
}

// ── API Tokens ──────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) InsertAPIToken(ctx context.Context, token *control.APIToken) error {
	scopesJSON, _ := json.Marshal(token.Scopes)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_tokens (id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.OrgID, token.UserID, token.Name, token.TokenHash, string(scopesJSON),
		token.ExpiresAt, token.LastUsedAt, token.RevokedAt, token.CreatedAt)
	return err
}

func (s *SQLiteAppStore) GetAPITokenByHash(ctx context.Context, hash string) (*control.APIToken, error) {
	return s.scanAPIToken(s.db.QueryRowContext(ctx, `
		SELECT id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE token_hash = ?`, []byte(hash)))
}

func (s *SQLiteAppStore) GetAPIToken(ctx context.Context, id string) (*control.APIToken, error) {
	return s.scanAPIToken(s.db.QueryRowContext(ctx, `
		SELECT id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE id = ?`, id))
}

func (s *SQLiteAppStore) ListAPITokens(ctx context.Context, userID string, page store.Page) (store.PageResult[*control.APIToken], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return store.PageResult[*control.APIToken]{}, err
	}
	defer rows.Close()
	var items []*control.APIToken
	for rows.Next() {
		item, err := scanAPITokenRow(rows)
		if err != nil {
			return store.PageResult[*control.APIToken]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) RevokeAPIToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ?`, now, tokenID)
	return err
}

func (s *SQLiteAppStore) TouchAPIToken(ctx context.Context, tokenID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, tokenID)
	return err
}

// ── Agent Tokens ────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) InsertAgentToken(ctx context.Context, token *control.AgentToken) error {
	scopesJSON, _ := json.Marshal(token.Scopes)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_tokens_new (id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.OrgID, token.AgentID, token.Name, token.TokenHash, string(scopesJSON),
		token.ExpiresAt, token.LastUsedAt, token.CreatedAt, token.RevokedAt)
	return err
}

func (s *SQLiteAppStore) GetAgentTokenByHash(ctx context.Context, hash string) (*control.AgentToken, error) {
	return s.scanAgentToken(s.db.QueryRowContext(ctx, `
		SELECT id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM agent_tokens_new WHERE token_hash = ?`, []byte(hash)))
}

func (s *SQLiteAppStore) GetAgentToken(ctx context.Context, id string) (*control.AgentToken, error) {
	return s.scanAgentToken(s.db.QueryRowContext(ctx, `
		SELECT id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM agent_tokens_new WHERE id = ?`, id))
}

func (s *SQLiteAppStore) ListAgentTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*control.AgentToken], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM agent_tokens_new WHERE agent_id = ? ORDER BY created_at DESC`, agentID)
	if err != nil {
		return store.PageResult[*control.AgentToken]{}, err
	}
	defer rows.Close()
	var items []*control.AgentToken
	for rows.Next() {
		item, err := scanAgentTokenRow(rows)
		if err != nil {
			return store.PageResult[*control.AgentToken]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) RevokeAgentToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE agent_tokens_new SET revoked_at = ? WHERE id = ?`, now, tokenID)
	return err
}

func (s *SQLiteAppStore) TouchAgentToken(ctx context.Context, tokenID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE agent_tokens_new SET last_used_at = ? WHERE id = ?`, now, tokenID)
	return err
}

// Legacy compatibility shims.
func (s *SQLiteAppStore) GetTokenByHash(ctx context.Context, hash string) (*control.AgentToken, error) {
	return s.GetAgentTokenByHash(ctx, hash)
}

func (s *SQLiteAppStore) GetToken(ctx context.Context, id string) (*control.AgentToken, error) {
	return s.GetAgentToken(ctx, id)
}

func (s *SQLiteAppStore) ListTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*control.AgentToken], error) {
	return s.ListAgentTokens(ctx, agentID, page)
}

func (s *SQLiteAppStore) RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return s.RevokeAgentToken(ctx, tokenID, revokedBy, reason)
}

func (s *SQLiteAppStore) TouchToken(ctx context.Context, tokenID string) error {
	return s.TouchAgentToken(ctx, tokenID)
}

// ── Roles / Bindings ────────────────────────────────────────────────────────

func (s *SQLiteAppStore) InsertRole(ctx context.Context, role *control.Role) error {
	permissionsJSON, _ := json.Marshal(role.Permissions)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO roles (id, org_id, name, permissions, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		role.ID, role.OrgID, role.Name, string(permissionsJSON), role.CreatedAt, role.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetRole(ctx context.Context, id string) (*control.Role, error) {
	return scanRole(s.db.QueryRowContext(ctx, `SELECT id, org_id, name, permissions, created_at, updated_at FROM roles WHERE id = ?`, id))
}

func (s *SQLiteAppStore) ListRoles(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Role], error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, org_id, name, permissions, created_at, updated_at FROM roles WHERE org_id = ? ORDER BY created_at DESC`, orgID)
	if err != nil {
		return store.PageResult[*control.Role]{}, err
	}
	defer rows.Close()
	var items []*control.Role
	for rows.Next() {
		item, err := scanRoleRow(rows)
		if err != nil {
			return store.PageResult[*control.Role]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) UpdateRole(ctx context.Context, id string, role *control.Role) error {
	permissionsJSON, _ := json.Marshal(role.Permissions)
	_, err := s.db.ExecContext(ctx, `UPDATE roles SET name = ?, permissions = ?, updated_at = ? WHERE id = ?`, role.Name, string(permissionsJSON), role.UpdatedAt, id)
	return err
}

func (s *SQLiteAppStore) DeleteRole(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id)
	return err
}

func (s *SQLiteAppStore) InsertBinding(ctx context.Context, binding *control.Binding) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO bindings (id, org_id, role_id, subject, scope, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		binding.ID, binding.OrgID, binding.RoleID, binding.Subject, binding.Scope, binding.CreatedAt)
	return err
}

func (s *SQLiteAppStore) GetBinding(ctx context.Context, id string) (*control.Binding, error) {
	return scanBinding(s.db.QueryRowContext(ctx, `SELECT id, org_id, role_id, subject, scope, created_at FROM bindings WHERE id = ?`, id))
}

func (s *SQLiteAppStore) ListBindings(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Binding], error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, org_id, role_id, subject, scope, created_at FROM bindings WHERE org_id = ? ORDER BY created_at DESC`, orgID)
	if err != nil {
		return store.PageResult[*control.Binding]{}, err
	}
	defer rows.Close()
	var items []*control.Binding
	for rows.Next() {
		item, err := scanBindingRow(rows)
		if err != nil {
			return store.PageResult[*control.Binding]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) DeleteBinding(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM bindings WHERE id = ?`, id)
	return err
}

func scanSession(r *sql.Row) (*control.Session, error) {
	var s control.Session
	if err := r.Scan(&s.ID, &s.OrgID, &s.UserID, &s.TokenHash, &s.ExpiresAt, &s.CreatedAt, &s.LastUsedAt, &s.UserAgent, &s.IP, &s.RevokedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func scanSessionRow(rows *sql.Rows) (*control.Session, error) {
	var s control.Session
	if err := rows.Scan(&s.ID, &s.OrgID, &s.UserID, &s.TokenHash, &s.ExpiresAt, &s.CreatedAt, &s.LastUsedAt, &s.UserAgent, &s.IP, &s.RevokedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *SQLiteAppStore) scanSession(row *sql.Row) (*control.Session, error) { return scanSession(row) }

func (s *SQLiteAppStore) scanAPIToken(row *sql.Row) (*control.APIToken, error) {
	var t control.APIToken
	var scopesJSON string
	if err := row.Scan(&t.ID, &t.OrgID, &t.UserID, &t.Name, &t.TokenHash, &scopesJSON, &t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt, &t.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
	return &t, nil
}

func scanAPITokenRow(rows *sql.Rows) (*control.APIToken, error) {
	var t control.APIToken
	var scopesJSON string
	if err := rows.Scan(&t.ID, &t.OrgID, &t.UserID, &t.Name, &t.TokenHash, &scopesJSON, &t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt, &t.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
	return &t, nil
}

func (s *SQLiteAppStore) scanAgentToken(row *sql.Row) (*control.AgentToken, error) {
	var t control.AgentToken
	var scopesJSON string
	if err := row.Scan(&t.ID, &t.OrgID, &t.AgentID, &t.Name, &t.TokenHash, &scopesJSON, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
	return &t, nil
}

func scanAgentTokenRow(rows *sql.Rows) (*control.AgentToken, error) {
	var t control.AgentToken
	var scopesJSON string
	if err := rows.Scan(&t.ID, &t.OrgID, &t.AgentID, &t.Name, &t.TokenHash, &scopesJSON, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopesJSON), &t.Scopes)
	return &t, nil
}

func scanRole(row *sql.Row) (*control.Role, error) {
	var role control.Role
	var permissionsJSON string
	if err := row.Scan(&role.ID, &role.OrgID, &role.Name, &permissionsJSON, &role.CreatedAt, &role.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(permissionsJSON), &role.Permissions)
	return &role, nil
}

func scanRoleRow(rows *sql.Rows) (*control.Role, error) {
	var role control.Role
	var permissionsJSON string
	if err := rows.Scan(&role.ID, &role.OrgID, &role.Name, &permissionsJSON, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(permissionsJSON), &role.Permissions)
	return &role, nil
}

func scanBinding(row *sql.Row) (*control.Binding, error) {
	var binding control.Binding
	if err := row.Scan(&binding.ID, &binding.OrgID, &binding.RoleID, &binding.Subject, &binding.Scope, &binding.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &binding, nil
}

func scanBindingRow(rows *sql.Rows) (*control.Binding, error) {
	var binding control.Binding
	if err := rows.Scan(&binding.ID, &binding.OrgID, &binding.RoleID, &binding.Subject, &binding.Scope, &binding.CreatedAt); err != nil {
		return nil, err
	}
	return &binding, nil
}
