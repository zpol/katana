package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// User is a KATANA UI/API identity.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Source      string `json:"source"`
	Email       string `json:"email,omitempty"`
	ExternalSub string `json:"external_sub,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	HasPassword bool   `json:"has_password"`
}

// Session represents an authenticated browser session.
type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

const usersDDL = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT,
  role TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'local',
  email TEXT NOT NULL DEFAULT '',
  external_sub TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_external_sub ON users(external_sub) WHERE external_sub IS NOT NULL AND external_sub != '';

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`

func (s *Store) migrateAuth(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, usersDDL); err != nil {
		return fmt.Errorf("migrate auth: %w", err)
	}
	return nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// CountLocalUsers returns the number of local-source users.
func (s *Store) CountLocalUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE source = 'local'`).Scan(&n)
	return n, err
}

// CountAdminUsers returns users with admin role.
func (s *Store) CountAdminUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n, err
}

// CreateUser inserts a user record.
func (s *Store) CreateUser(ctx context.Context, u User, passwordHash string) (User, error) {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	ts := nowRFC3339()
	if u.CreatedAt == "" {
		u.CreatedAt = ts
	}
	u.UpdatedAt = ts
	var hash sql.NullString
	if passwordHash != "" {
		hash = sql.NullString{String: passwordHash, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users (id, username, password_hash, role, source, email, external_sub, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, hash, u.Role, u.Source, u.Email, nullStr(u.ExternalSub), u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return User{}, err
	}
	u.HasPassword = passwordHash != ""
	return u, nil
}

func nullStr(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

// GetUserByUsername fetches a user by username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, string, error) {
	var u User
	var hash sql.NullString
	var ext sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, source, email, external_sub, created_at, updated_at
FROM users WHERE username = ?`, username).Scan(
		&u.ID, &u.Username, &hash, &u.Role, &u.Source, &u.Email, &ext, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return User{}, "", err
	}
	if ext.Valid {
		u.ExternalSub = ext.String
	}
	u.HasPassword = hash.Valid && hash.String != ""
	pw := ""
	if hash.Valid {
		pw = hash.String
	}
	return u, pw, nil
}

// GetUserByID fetches a user by id.
func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	var u User
	var hash sql.NullString
	var ext sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, source, email, external_sub, created_at, updated_at
FROM users WHERE id = ?`, id).Scan(
		&u.ID, &u.Username, &hash, &u.Role, &u.Source, &u.Email, &ext, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return User{}, err
	}
	if ext.Valid {
		u.ExternalSub = ext.String
	}
	u.HasPassword = hash.Valid && hash.String != ""
	return u, nil
}

// GetUserByExternalSub fetches an OIDC user by subject claim.
func (s *Store) GetUserByExternalSub(ctx context.Context, sub string) (User, error) {
	var u User
	var hash sql.NullString
	var ext sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, source, email, external_sub, created_at, updated_at
FROM users WHERE external_sub = ?`, sub).Scan(
		&u.ID, &u.Username, &hash, &u.Role, &u.Source, &u.Email, &ext, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return User{}, err
	}
	if ext.Valid {
		u.ExternalSub = ext.String
	}
	u.HasPassword = hash.Valid && hash.String != ""
	return u, nil
}

// ListUsers returns all users (no password hashes).
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, password_hash, role, source, email, external_sub, created_at, updated_at
FROM users ORDER BY username ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var hash sql.NullString
		var ext sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &hash, &u.Role, &u.Source, &u.Email, &ext, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		if ext.Valid {
			u.ExternalSub = ext.String
		}
		u.HasPassword = hash.Valid && hash.String != ""
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUserRole sets role for a user.
func (s *Store) UpdateUserRole(ctx context.Context, id, role string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role = ?, updated_at = ? WHERE id = ?`, role, nowRFC3339(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateUserPassword sets bcrypt hash for a local user.
func (s *Store) UpdateUserPassword(ctx context.Context, id, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ? AND source = 'local'`, passwordHash, nowRFC3339(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateOIDCUser updates role/email for an OIDC-linked user.
func (s *Store) UpdateOIDCUser(ctx context.Context, id, role, email string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role = ?, email = ?, updated_at = ? WHERE id = ?`, role, email, nowRFC3339(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteUser removes a user and their sessions.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// CreateSession stores a new session.
func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (Session, error) {
	sess := Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(ttl),
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.ExpiresAt.Format(time.RFC3339), sess.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

// GetSession loads a session if not expired.
func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	var sess Session
	var exp, created string
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?`, id).Scan(
		&sess.ID, &sess.UserID, &exp, &created,
	)
	if err != nil {
		return Session{}, err
	}
	sess.ExpiresAt, err = time.Parse(time.RFC3339, exp)
	if err != nil {
		return Session{}, err
	}
	sess.CreatedAt, err = time.Parse(time.RFC3339, created)
	if err != nil {
		return Session{}, err
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
		return Session{}, sql.ErrNoRows
	}
	return sess, nil
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// SSOSettings holds non-secret OIDC overrides editable from the UI.
type SSOSettings struct {
	Enabled         bool     `json:"enabled"`
	Issuer          string   `json:"issuer"`
	ClientID        string   `json:"client_id"`
	RedirectURI     string   `json:"redirect_uri"`
	Scopes          []string `json:"scopes"`
	GroupClaim      string   `json:"group_claim"`
	EmailClaim      string   `json:"email_claim"`
	AdminGroups     []string `json:"admin_groups"`
	ReadonlyGroups  []string `json:"readonly_groups"`
	ClientSecretSet bool     `json:"client_secret_configured"`
}

const ssoSettingsKey = "sso"

// GetSSOSettings reads SSO overrides from app_settings.
func (s *Store) GetSSOSettings(ctx context.Context) (SSOSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_settings WHERE key = ?`, ssoSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return SSOSettings{}, nil
	}
	if err != nil {
		return SSOSettings{}, err
	}
	var out SSOSettings
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return SSOSettings{}, err
	}
	return out, nil
}

// SaveSSOSettings persists non-secret SSO overrides.
func (s *Store) SaveSSOSettings(ctx context.Context, cfg SSOSettings) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	ts := nowRFC3339()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO app_settings (key, value_json, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		ssoSettingsKey, string(b), ts,
	)
	return err
}
