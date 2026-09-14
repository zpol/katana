package fleet

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

func windowRange(w string) string {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "1h":
		return "1h"
	case "7d":
		return "7d"
	default:
		return "24h"
	}
}

func pct(part, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(part/total*1000) / 10
}

func segs(items map[string]float64, colors map[string]string, order []string) []Segment {
	var total float64
	for _, v := range items {
		if v > 0 {
			total += v
		}
	}
	var keys []string
	if len(order) > 0 {
		keys = append(keys, order...)
		for k := range items {
			found := false
			for _, o := range order {
				if o == k {
					found = true
					break
				}
			}
			if !found {
				keys = append(keys, k)
			}
		}
	} else {
		for k := range items {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	out := make([]Segment, 0, len(keys))
	for _, k := range keys {
		v := items[k]
		if v < 0 {
			v = 0
		}
		out = append(out, Segment{Label: k, Value: math.Round(v*10) / 10, Pct: pct(v, total), Color: colors[k]})
	}
	return out
}

func samplesOrRaw(samples func(string) []promSample, window, by, metric, extra string) []promSample {
	inc := samples("sum by (" + by + ") (increase(" + metric + extra + "[" + window + "]))")
	var total float64
	for _, sm := range inc {
		total += sampleValue(sm)
	}
	if total > 0.05 {
		return inc
	}
	return samples("sum by (" + by + ") (" + metric + extra + ")")
}

func (s *Server) buildOverview(ctx context.Context, window string) Overview {
	window = windowRange(window)
	ov := Overview{
		Window:        window,
		GeneratedAt:   time.Now().UTC(),
		Clusters:      []Cluster{},
		Posture:       []Segment{},
		ClusterHealth: []Segment{},
		ScanCoverage:  []Segment{},
		Policies:      []Segment{},
		Severity:      []Segment{},
		Mode:          []Segment{},
		Nodes:         []Segment{},
	}

	samples := func(q string) []promSample {
		out, err := s.prom.query(ctx, q)
		if err != nil {
			if ov.PrometheusError == "" {
				ov.PrometheusError = err.Error()
			}
			return nil
		}
		ov.PrometheusOK = true
		return out
	}

	info := samples(`katana_info`)
	ready := samples(`katana_policies_ready`)
	jfrog := samples(`katana_jfrog_up`)
	nodes := samples(`katana_cluster_nodes`)
	k8s := samples(`katana_cluster_info`)
	up := samples(`up{job="katana"}`)
	outcomes := samplesOrRaw(samples, window, "cluster,outcome", "katana_admission_outcomes_total", "")
	outcomesAll := samplesOrRaw(samples, window, "outcome", "katana_admission_outcomes_total", "")
	xray := samplesOrRaw(samples, window, "status", "katana_xray_lookups_total", "")
	pol := samplesOrRaw(samples, window, "policy", "katana_admission_policy_total", `{action="deny"}`)
	sev := samplesOrRaw(samples, window, "severity", "katana_admission_severity_total", "")

	byCluster := map[string]*Cluster{}
	ensure := func(name string) *Cluster {
		if name == "" {
			name = "unknown"
		}
		if c, ok := byCluster[name]; ok {
			return c
		}
		c := &Cluster{Name: name, KatanaStatus: "red", ClusterStatus: "red"}
		byCluster[name] = c
		return c
	}

	for _, s := range info {
		c := ensure(s.Metric["cluster"])
		c.Version = s.Metric["version"]
		c.DryRun = s.Metric["dry_run"] == "true"
		c.LastSeenUnix = time.Now().Unix()
	}
	upBy := map[string]bool{}
	for _, s := range up {
		name := s.Metric["cluster"]
		if name == "" {
			name = s.Metric["instance"]
		}
		upBy[name] = sampleValue(s) >= 1
	}
	for _, s := range ready {
		c := ensure(s.Metric["cluster"])
		c.PoliciesReady = sampleValue(s) >= 1
	}
	for _, s := range jfrog {
		c := ensure(s.Metric["cluster"])
		c.JFrogUp = sampleValue(s) >= 1
	}
	for _, s := range k8s {
		c := ensure(s.Metric["cluster"])
		c.K8sVersion = s.Metric["k8s_version"]
	}
	for _, s := range nodes {
		c := ensure(s.Metric["cluster"])
		n := int(math.Round(sampleValue(s)))
		if s.Metric["status"] == "not_ready" {
			c.NodesNotReady = n
		} else {
			c.NodesReady = n
		}
	}

	// If up metric has no cluster label, treat any katana_info cluster as up when any up==1.
	anyUp := false
	for _, v := range upBy {
		if v {
			anyUp = true
		}
	}
	for name, c := range byCluster {
		if v, ok := upBy[name]; ok {
			c.KatanaUp = v
		} else {
			c.KatanaUp = anyUp && c.Version != ""
		}
		if c.KatanaUp && c.PoliciesReady {
			c.KatanaStatus = "green"
		} else {
			c.KatanaStatus = "red"
		}
		switch {
		case !c.KatanaUp:
			c.ClusterStatus = "red"
		case c.NodesNotReady > 0:
			c.ClusterStatus = "amber"
		default:
			c.ClusterStatus = "green"
		}
	}

	postureByCluster := map[string]map[string]float64{}
	postureAll := map[string]float64{}
	for _, s := range outcomesAll {
		postureAll[s.Metric["outcome"]] += sampleValue(s)
	}
	for _, s := range outcomes {
		cl := s.Metric["cluster"]
		if postureByCluster[cl] == nil {
			postureByCluster[cl] = map[string]float64{}
		}
		postureByCluster[cl][s.Metric["outcome"]] += sampleValue(s)
		c := ensure(cl)
		o := s.Metric["outcome"]
		v := sampleValue(s)
		if o == "compliant" {
			c.Compliant += v
		} else if o != "" {
			c.NonCompliant += v
		}
	}

	colorsPosture := map[string]string{
		"compliant":          "#3cbe8c",
		"blocked":            "#e35d5d",
		"dry_run_would_deny": "#e6a23c",
		"unscanned_allowed":  "#4d94ff",
		"error":              "#8b95a5",
	}
	ov.Posture = segs(postureAll, colorsPosture, []string{"compliant", "blocked", "dry_run_would_deny", "unscanned_allowed", "error"})
	comp := postureAll["compliant"]
	non := postureAll["blocked"] + postureAll["dry_run_would_deny"] + postureAll["unscanned_allowed"] + postureAll["error"]
	ov.CompliantPct = pct(comp, comp+non)
	ov.NonCompliantPct = pct(non, comp+non)
	denies := postureAll["blocked"]
	ov.DenyRate = pct(denies, comp+non)

	scan := map[string]float64{}
	for _, s := range xray {
		scan[s.Metric["status"]] += sampleValue(s)
	}
	ov.ScanCoverage = segs(scan, map[string]string{
		"indexed":     "#3cbe8c",
		"not_indexed": "#e6a23c",
		"unavailable": "#e35d5d",
	}, []string{"indexed", "not_indexed", "unavailable"})
	ov.XrayUnavailable = scan["unavailable"]

	polMap := map[string]float64{}
	for _, s := range pol {
		name := s.Metric["policy"]
		if name == "" {
			name = "none"
		}
		polMap[name] += sampleValue(s)
	}
	ov.Policies = segs(polMap, nil, nil)

	sevMap := map[string]float64{}
	for _, s := range sev {
		sevMap[s.Metric["severity"]] += sampleValue(s)
	}
	ov.Severity = segs(sevMap, map[string]string{
		"critical": "#e35d5d",
		"high":     "#e6a23c",
		"medium":   "#c5ccd6",
		"low":      "#4d94ff",
		"info":     "#3cbe8c",
	}, []string{"critical", "high", "medium", "low", "info"})

	var names []string
	enforce, dry := 0, 0
	readyN, notReadyN := 0, 0
	versions := map[string]int{}
	for name, c := range byCluster {
		names = append(names, name)
		if c.KatanaUp {
			ov.KatanaUp++
		} else {
			ov.KatanaDown++
		}
		if c.DryRun {
			dry++
		} else {
			enforce++
		}
		readyN += c.NodesReady
		notReadyN += c.NodesNotReady
		if c.Version != "" {
			versions[c.Version]++
		}
		c.CompliantPct = pct(c.Compliant, c.Compliant+c.NonCompliant)
	}
	sort.Strings(names)
	for _, n := range names {
		ov.Clusters = append(ov.Clusters, *byCluster[n])
	}
	ov.NodesNotReady = notReadyN
	if ov.KatanaUp+ov.KatanaDown > 0 {
		ov.FleetHealthPct = pct(float64(ov.KatanaUp), float64(ov.KatanaUp+ov.KatanaDown))
	}
	if len(versions) > 1 {
		ov.VersionDrift = len(versions) - 1
	}
	ov.ClusterHealth = segs(map[string]float64{
		"up":   float64(ov.KatanaUp),
		"down": float64(ov.KatanaDown),
	}, map[string]string{"up": "#3cbe8c", "down": "#e35d5d"}, []string{"up", "down"})
	ov.Mode = segs(map[string]float64{
		"enforce": float64(enforce),
		"dry-run": float64(dry),
	}, map[string]string{"enforce": "#4d94ff", "dry-run": "#e6a23c"}, []string{"enforce", "dry-run"})
	ov.Nodes = segs(map[string]float64{
		"ready":     float64(readyN),
		"not_ready": float64(notReadyN),
	}, map[string]string{"ready": "#3cbe8c", "not_ready": "#e35d5d"}, []string{"ready", "not_ready"})

	if ov.PrometheusError == "" && (len(info) > 0 || len(up) > 0) {
		ov.PrometheusOK = true
	}
	return ov
}

func (s *Server) snapshotOnce(ctx context.Context) {
	ov := s.buildOverview(ctx, "1h")
	for _, c := range ov.Clusters {
		_ = s.store.RecordSnapshot(ctx, Snapshot{
			Ts:            time.Now().UTC(),
			Cluster:       c.Name,
			KatanaVersion: c.Version,
			K8sVersion:    c.K8sVersion,
			NodesReady:    c.NodesReady,
			NodesNotReady: c.NodesNotReady,
			KatanaUp:      c.KatanaUp,
			DryRun:        c.DryRun,
		})
	}
}
