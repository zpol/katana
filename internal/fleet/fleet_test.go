package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowRange(t *testing.T) {
	if windowRange("1h") != "1h" || windowRange("") != "24h" || windowRange("7d") != "7d" {
		t.Fatal(windowRange("x"))
	}
}

func TestSegsPercent(t *testing.T) {
	out := segs(map[string]float64{"compliant": 75, "blocked": 25}, map[string]string{"compliant": "#0"}, []string{"compliant", "blocked"})
	if len(out) != 2 {
		t.Fatalf("%d", len(out))
	}
	if out[0].Pct != 75 || out[1].Pct != 25 {
		t.Fatalf("%+v", out)
	}
}

func TestStoreDeployHistory(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.RecordSnapshot(ctx, Snapshot{Ts: time.Now(), Cluster: "c1", KatanaVersion: "0.4.1", KatanaUp: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordSnapshot(ctx, Snapshot{Ts: time.Now().Add(time.Minute), Cluster: "c1", KatanaVersion: "0.5.0", KatanaUp: true}); err != nil {
		t.Fatal(err)
	}
	ev, err := st.ListDeploys(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 || ev[0].FromVersion != "0.4.1" || ev[0].ToVersion != "0.5.0" {
		t.Fatalf("%+v", ev)
	}
}

func TestOverviewEmptyPrometheus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": []any{}},
		})
	}))
	defer ts.Close()
	srv, err := New(ts.URL, filepath.Join(t.TempDir(), "f.db"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ov := srv.buildOverview(context.Background(), "1h")
	if !ov.PrometheusOK {
		t.Fatalf("expected ok empty: %+v", ov)
	}
	if ov.CompliantPct != 0 {
		t.Fatalf("pct=%v", ov.CompliantPct)
	}
}
