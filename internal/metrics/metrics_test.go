package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveAdmissionIncrementsCounters(t *testing.T) {
	ObserveAdmission("deny", "blocked", 10*time.Millisecond)
	ObserveSeverity("critical")
	ObservePolicy("Block Critical", "deny")
	ObserveXrayLookup("indexed", 20*time.Millisecond)

	if n := testutil.ToFloat64(admissionRequests.WithLabelValues(cluster, "deny")); n < 1 {
		t.Fatalf("admission_requests_total deny=%v", n)
	}
	if n := testutil.ToFloat64(admissionOutcomes.WithLabelValues(cluster, "blocked")); n < 1 {
		t.Fatalf("admission_outcomes_total blocked=%v", n)
	}
	if n := testutil.ToFloat64(admissionSeverity.WithLabelValues(cluster, "critical")); n < 1 {
		t.Fatalf("severity critical=%v", n)
	}
	if n := testutil.ToFloat64(admissionPolicy.WithLabelValues(cluster, "Block Critical", "deny")); n < 1 {
		t.Fatalf("policy=%v", n)
	}
	if n := testutil.ToFloat64(xrayLookups.WithLabelValues(cluster, "indexed")); n < 1 {
		t.Fatalf("xray indexed=%v", n)
	}
}

func TestClusterNameFallback(t *testing.T) {
	if ClusterName() == "" {
		t.Fatal("empty cluster name")
	}
}

func TestMustRegisterIdempotentOnCustomRegistry(t *testing.T) {
	r := prometheus.NewPedanticRegistry()
	MustRegister(r)
	c := NewRuntimeCollector(Runtime{Version: "test", PolicySource: "sqlite", DryRun: true})
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	mfs, err := r.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"katana_info", "katana_policies_ready", "katana_jfrog_up", "katana_cluster_nodes", "katana_cluster_info"}
	found := map[string]bool{}
	for _, mf := range mfs {
		found[mf.GetName()] = true
	}
	for _, n := range want {
		if !found[n] {
			t.Fatalf("missing metric %s in %v", n, found)
		}
	}
	_ = strings.TrimSpace
}
