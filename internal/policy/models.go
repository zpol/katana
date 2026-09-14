package policy

import "time"

// Action is the enforcement decision for a matched policy.
type Action string

const (
	ActionDeny  Action = "deny"
	ActionWarn  Action = "warn"
	ActionAudit Action = "audit"
)

// MatchCriteria describes when a policy applies.
type MatchCriteria struct {
	Severity           string   `json:"severity,omitempty"`
	Environment        string   `json:"environment,omitempty"`
	Scanned            *bool    `json:"scanned,omitempty"`
	NamespaceAllowlist []string `json:"namespace_allowlist,omitempty"`
	RegistryAllowlist  []string `json:"registry_allowlist,omitempty"`
	// Pod security (admission webhook only)
	UnsafePodSecurity        *bool `json:"unsafe_pod_security,omitempty"`
	Privileged               *bool `json:"privileged,omitempty"`
	RunAsRoot                *bool `json:"run_as_root,omitempty"`
	AllowPrivilegeEscalation *bool `json:"allow_privilege_escalation,omitempty"`
}

// Policy is a persisted admission/control rule.
type Policy struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
	Action      Action        `json:"action"`
	Match       MatchCriteria `json:"match"`
	Exceptions  []string      `json:"exceptions"`
	// DenyMessage is shown to developers when this policy blocks admission (supports {image}, {policy}, …).
	DenyMessage string `json:"deny_message,omitempty"`
	// WarnMessage is shown as an admission warning when action is warn.
	WarnMessage string `json:"warn_message,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// EvaluationInput is the subject under evaluation.
type EvaluationInput struct {
	Severity    string `json:"severity"`
	Environment string `json:"environment"`
	Scanned     bool   `json:"scanned"`
	Namespace   string `json:"namespace"`
	Registry    string `json:"registry"`
	Image       string `json:"image"`
	// Pod security flags (admission only; zero value = not evaluated).
	PodSecurity PodSecurityAnalysis `json:"pod_security,omitempty"`
	// XrayUnavailable means JFrog timed out or errored. Treated as unscanned (fail-closed).
	XrayUnavailable bool `json:"xray_unavailable,omitempty"`
}

// Decision is the result of evaluating policies against an input.
type Decision struct {
	Allowed         bool     `json:"allowed"`
	Action          Action   `json:"action"`
	MatchedPolicy   string   `json:"matched_policy,omitempty"`
	MatchedPolicyID string   `json:"matched_policy_id,omitempty"`
	UserMessage     string   `json:"user_message,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
}
