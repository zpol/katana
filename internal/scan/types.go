package scan

import "context"

// Lookup statuses shared by all scanners.
const (
	StatusIndexed     = "indexed"
	StatusNotIndexed  = "not_indexed"
	StatusUnavailable = "unavailable"
)

// ArtifactRef identifies an image/artifact for a scanner lookup.
type ArtifactRef struct {
	Repo     string `json:"repo,omitempty"`
	Path     string `json:"path,omitempty"`
	Sha      string `json:"sha,omitempty"`
	ImageRef string `json:"image_ref,omitempty"`
}

// Summary is a normalized vulnerability scan result (JFrog, ECR Inspector, …).
type Summary struct {
	Artifact     ArtifactRef `json:"artifact"`
	Scanned      bool        `json:"scanned"`
	LookupStatus string      `json:"lookup_status,omitempty"` // indexed | not_indexed | unavailable
	Scanner      string      `json:"scanner,omitempty"`      // jfrog | ecr_inspector
	Critical     int         `json:"critical"`
	High         int         `json:"high"`
	Medium       int         `json:"medium"`
	Low          int         `json:"low"`
	Unknown      int         `json:"unknown"`
	Violations   []string    `json:"violations,omitempty"`
	RawError     string      `json:"error,omitempty"`
}

// Status describes integration readiness (no secrets).
type Status struct {
	Configured bool   `json:"configured"`
	BaseURL    string `json:"base_url,omitempty"`
	Reachable  bool   `json:"reachable"`
	Message    string `json:"message"`
	Scanner    string `json:"scanner,omitempty"`
}

// Client is a vulnerability scan backend.
type Client interface {
	Name() string
	Configured() bool
	Status(ctx context.Context) Status
	GetScanSummary(ctx context.Context, ref ArtifactRef) (*Summary, error)
}

// Unavailable builds a soft-failure summary (err is still nil at call sites).
func Unavailable(ref ArtifactRef, scanner, msg string, violations ...string) *Summary {
	if len(violations) == 0 && msg != "" {
		violations = []string{msg}
	}
	return &Summary{
		Artifact:     ref,
		Scanned:      false,
		LookupStatus: StatusUnavailable,
		Scanner:      scanner,
		RawError:     msg,
		Violations:   violations,
	}
}

// NotIndexed builds an unscanned-but-reachable summary.
func NotIndexed(ref ArtifactRef, scanner, msg string) *Summary {
	return &Summary{
		Artifact:     ref,
		Scanned:      false,
		LookupStatus: StatusNotIndexed,
		Scanner:      scanner,
		RawError:     msg,
		Violations:   []string{msg},
	}
}
