package admission

import (
	"os"
	"strings"
)

const defaultEnvironment = "prod"

// resolveEnvironment returns the policy environment for a namespace.
// Pod labels are not trusted (they are attacker-controlled on CREATE).
// Unlisted namespaces default to prod (fail-closed for "Block High in Prod").
// Set KATANA_NONPROD_NAMESPACES to a comma list of exact names or prefix*
// patterns that should evaluate as "dev" (e.g. "dev,qa,*-sandbox").
func resolveEnvironment(namespace string) string {
	ns := strings.TrimSpace(namespace)
	for _, p := range nonprodNamespacePatterns() {
		if matchNamespacePattern(p, ns) {
			return "dev"
		}
	}
	return defaultEnvironment
}

func nonprodNamespacePatterns() []string {
	raw := strings.TrimSpace(os.Getenv("KATANA_NONPROD_NAMESPACES"))
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func matchNamespacePattern(pattern, namespace string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || namespace == "" {
		return false
	}
	p := strings.ToLower(pattern)
	ns := strings.ToLower(namespace)
	if p == "*" {
		return true
	}
	starPrefix := strings.HasPrefix(p, "*")
	starSuffix := strings.HasSuffix(p, "*")
	switch {
	case starPrefix && starSuffix:
		mid := strings.Trim(p, "*")
		return mid != "" && strings.Contains(ns, mid)
	case starSuffix:
		return strings.HasPrefix(ns, strings.TrimSuffix(p, "*"))
	case starPrefix:
		return strings.HasSuffix(ns, strings.TrimPrefix(p, "*"))
	default:
		return p == ns
	}
}
