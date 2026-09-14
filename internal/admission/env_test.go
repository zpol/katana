package admission

import "testing"

func TestResolveEnvironment_DefaultsProd(t *testing.T) {
	t.Setenv("KATANA_NONPROD_NAMESPACES", "")
	if got := resolveEnvironment("payments"); got != "prod" {
		t.Fatalf("got %q", got)
	}
	if got := resolveEnvironment(""); got != "prod" {
		t.Fatalf("empty ns: got %q", got)
	}
}

func TestResolveEnvironment_NonprodPatterns(t *testing.T) {
	t.Setenv("KATANA_NONPROD_NAMESPACES", "dev,qa,*-sandbox")
	if got := resolveEnvironment("dev"); got != "dev" {
		t.Fatalf("exact: got %q", got)
	}
	if got := resolveEnvironment("team-sandbox"); got != "dev" {
		t.Fatalf("suffix glob: got %q", got)
	}
	if got := resolveEnvironment("prod"); got != "prod" {
		t.Fatalf("prod ns must stay prod: got %q", got)
	}
}
