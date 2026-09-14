package policystore

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zpol/katana/internal/store"
)

// New opens a policy store based on KATANA_POLICY_SOURCE (sqlite | crd).
func New(ctx context.Context, sqlite *store.Store) (Store, string, error) {
	src := strings.ToLower(strings.TrimSpace(os.Getenv("KATANA_POLICY_SOURCE")))
	if src == "" {
		src = "sqlite"
	}
	switch src {
	case "crd", "kubernetes", "k8s":
		crd, err := NewCRD(ctx)
		if err != nil {
			return nil, src, err
		}
		return crd, "crd", nil
	case "sqlite", "db":
		return NewSQLite(sqlite), "sqlite", nil
	default:
		return nil, src, fmt.Errorf("unknown KATANA_POLICY_SOURCE %q (use sqlite or crd)", src)
	}
}
