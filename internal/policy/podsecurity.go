package policy

// PodSecurityAnalysis summarizes risky Pod / container securityContext settings.
type PodSecurityAnalysis struct {
	Privileged               bool     `json:"privileged"`
	RunAsRoot                bool     `json:"run_as_root"`
	AllowPrivilegeEscalation bool     `json:"allow_privilege_escalation"`
	Unsafe                   bool     `json:"unsafe"`
	Reasons                  []string `json:"reasons,omitempty"`
}

// UnsafePodSecurity is true when any risky setting is detected.
func (p PodSecurityAnalysis) UnsafePodSecurity() bool {
	return p.Unsafe
}
