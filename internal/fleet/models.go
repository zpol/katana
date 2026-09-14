package fleet

import "time"

type Segment struct {
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Pct    float64 `json:"pct"`
	Color  string  `json:"color,omitempty"`
}

type Cluster struct {
	Name           string  `json:"name"`
	KatanaStatus   string  `json:"katana_status"` // green|red
	ClusterStatus  string  `json:"cluster_status"` // green|amber|red
	KatanaUp       bool    `json:"katana_up"`
	PoliciesReady  bool    `json:"policies_ready"`
	JFrogUp        bool    `json:"jfrog_up"`
	DryRun         bool    `json:"dry_run"`
	Version        string  `json:"version"`
	K8sVersion     string  `json:"k8s_version"`
	NodesReady     int     `json:"nodes_ready"`
	NodesNotReady  int     `json:"nodes_not_ready"`
	Compliant      float64 `json:"compliant"`
	NonCompliant   float64 `json:"non_compliant"`
	CompliantPct   float64 `json:"compliant_pct"`
	LastSeenUnix   int64   `json:"last_seen_unix"`
}

type Overview struct {
	Window          string     `json:"window"`
	GeneratedAt     time.Time  `json:"generated_at"`
	PrometheusOK    bool       `json:"prometheus_ok"`
	PrometheusError string     `json:"prometheus_error,omitempty"`
	FleetHealthPct  float64    `json:"fleet_health_pct"`
	KatanaUp        int        `json:"katana_up"`
	KatanaDown      int        `json:"katana_down"`
	CompliantPct    float64    `json:"compliant_pct"`
	NonCompliantPct float64    `json:"non_compliant_pct"`
	DenyRate        float64    `json:"deny_rate"`
	XrayUnavailable float64    `json:"xray_unavailable"`
	NodesNotReady   int        `json:"nodes_not_ready"`
	VersionDrift    int        `json:"version_drift"`
	Posture         []Segment  `json:"posture"`
	ClusterHealth   []Segment  `json:"cluster_health"`
	ScanCoverage    []Segment  `json:"scan_coverage"`
	Policies        []Segment  `json:"policies"`
	Severity        []Segment  `json:"severity"`
	Mode            []Segment  `json:"mode"`
	Nodes           []Segment  `json:"nodes"`
	Clusters        []Cluster  `json:"clusters"`
}

type HistoryEvent struct {
	ID          int64     `json:"id"`
	Ts          time.Time `json:"ts"`
	Cluster     string    `json:"cluster"`
	FromVersion string    `json:"from_version"`
	ToVersion   string    `json:"to_version"`
}

type History struct {
	Deploys []HistoryEvent `json:"deploys"`
}

type Snapshot struct {
	Ts            time.Time
	Cluster       string
	KatanaVersion string
	K8sVersion    string
	NodesReady    int
	NodesNotReady int
	KatanaUp      bool
	DryRun        bool
}
