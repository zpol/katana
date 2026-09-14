package policystore

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/goxray/goxray/internal/policy"
	"github.com/goxray/goxray/internal/store"
)

// SQLite wraps store.Store policy methods.
type SQLite struct {
	st *store.Store
}

func NewSQLite(st *store.Store) *SQLite {
	return &SQLite{st: st}
}

func (s *SQLite) List(ctx context.Context) ([]policy.Policy, error) {
	return s.st.ListPolicies(ctx)
}

func (s *SQLite) Get(ctx context.Context, id string) (policy.Policy, error) {
	return s.st.GetPolicy(ctx, id)
}

func (s *SQLite) Create(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	return s.st.CreatePolicy(ctx, p)
}

func (s *SQLite) Update(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	return s.st.UpdatePolicy(ctx, p)
}

func (s *SQLite) Patch(ctx context.Context, id string, patch map[string]json.RawMessage) (policy.Policy, error) {
	return s.st.PatchPolicy(ctx, id, patch)
}

func (s *SQLite) Delete(ctx context.Context, id string) error {
	return s.st.DeletePolicy(ctx, id)
}

func (s *SQLite) Ready(ctx context.Context) bool {
	_, err := s.st.ListPolicies(ctx)
	return err == nil
}

func (s *SQLite) Close() error {
	return nil
}

// ErrNotFound is returned when a policy id does not exist.
var ErrNotFound = sql.ErrNoRows
