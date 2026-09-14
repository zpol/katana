package policystore

import (
	"context"
	"encoding/json"

	"github.com/zpol/katana/internal/policy"
)

// Store loads and persists admission policies (SQLite or Kubernetes CRDs).
type Store interface {
	List(ctx context.Context) ([]policy.Policy, error)
	Get(ctx context.Context, id string) (policy.Policy, error)
	Create(ctx context.Context, p policy.Policy) (policy.Policy, error)
	Update(ctx context.Context, p policy.Policy) (policy.Policy, error)
	Patch(ctx context.Context, id string, patch map[string]json.RawMessage) (policy.Policy, error)
	Delete(ctx context.Context, id string) error
	// Ready reports whether policies are available (CRD cache synced).
	Ready(ctx context.Context) bool
	Close() error
}
