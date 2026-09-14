package detect

import "time"

// Severity levels for detections.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// Detection is a security finding associated with an image or workload.
type Detection struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Severity     Severity  `json:"severity"`
	Environment  string    `json:"environment"`
	Namespace    string    `json:"namespace"`
	Registry     string    `json:"registry"`
	Image        string    `json:"image"`
	Scanned      bool      `json:"scanned"`
	Source       string    `json:"source"`
	PolicyAction string    `json:"policy_action,omitempty"`
	Deployed     *bool     `json:"deployed,omitempty"`
	DryRun       bool      `json:"dry_run,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Filter selects detections for listing.
type Filter struct {
	Severity    string
	Environment string
	Namespace   string
	Registry    string
	Outcome     string
	Q           string
}

// StatsSummary aggregates admission and detection metrics for the dashboard.
type StatsSummary struct {
	TotalEvents     int            `json:"total_events"`
	AdmissionEvents int            `json:"admission_events"`
	Deployed        int            `json:"deployed"`
	Blocked         int            `json:"blocked"`
	DryRunWouldDeny int            `json:"dry_run_would_deny"`
	DeployedPct     float64        `json:"deployed_pct"`
	BlockedPct      float64        `json:"blocked_pct"`
	BySeverity      map[string]int `json:"by_severity"`
	ByNamespace     map[string]int `json:"by_namespace"`
	ByPolicyAction  map[string]int `json:"by_policy_action"`
	ScannedCount    int            `json:"scanned_count"`
	UnscannedCount  int            `json:"unscanned_count"`
}
