package policy

import (
	"fmt"
	"strings"
)

// Evaluator applies policies to an evaluation input.
type Evaluator struct{}

// NewEvaluator returns a policy evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate walks enabled policies and returns the strongest applicable decision.
// Precedence: deny > warn > audit > default allow.
func (e *Evaluator) Evaluate(policies []Policy, in EvaluationInput) Decision {
	var (
		denyHit  *Policy
		warnHit  *Policy
		auditHit *Policy
		reasons  []string
	)

	for i := range policies {
		p := &policies[i]
		if !p.Enabled {
			continue
		}
		if matchesException(p.Exceptions, in.Namespace) {
			continue
		}
		matched, reason := matchPolicy(p, in)
		if !matched {
			continue
		}
		reasons = append(reasons, reason)
		switch p.Action {
		case ActionDeny:
			if denyHit == nil {
				cp := *p
				denyHit = &cp
			}
		case ActionWarn:
			if warnHit == nil {
				cp := *p
				warnHit = &cp
			}
		case ActionAudit:
			if auditHit == nil {
				cp := *p
				auditHit = &cp
			}
		}
	}

	if denyHit != nil {
		dec := Decision{
			Allowed:         false,
			Action:          ActionDeny,
			MatchedPolicy:   denyHit.Name,
			MatchedPolicyID: denyHit.ID,
			Reasons:         reasons,
		}
		dec.UserMessage = FormatAdmissionMessage(denyHit, in, reasons)
		return dec
	}
	if warnHit != nil {
		dec := Decision{
			Allowed:         true,
			Action:          ActionWarn,
			MatchedPolicy:   warnHit.Name,
			MatchedPolicyID: warnHit.ID,
			Reasons:         reasons,
		}
		dec.UserMessage = FormatAdmissionMessage(warnHit, in, reasons)
		return dec
	}
	if auditHit != nil {
		return Decision{
			Allowed:         true,
			Action:          ActionAudit,
			MatchedPolicy:   auditHit.Name,
			MatchedPolicyID: auditHit.ID,
			Reasons:         reasons,
		}
	}
	return Decision{
		Allowed: true,
		Action:  ActionAudit,
		Reasons: []string{"no matching policy; default allow"},
	}
}

func matchPolicy(p *Policy, in EvaluationInput) (bool, string) {
	m := p.Match
	checks := 0

	if m.Severity != "" {
		checks++
		if !strings.EqualFold(m.Severity, in.Severity) {
			return false, ""
		}
	}
	if m.Environment != "" {
		checks++
		if !strings.EqualFold(m.Environment, in.Environment) {
			return false, ""
		}
	}
	if m.Scanned != nil {
		checks++
		// Xray timeout/error is treated as unscanned (fail-closed).
		scanned := in.Scanned && !in.XrayUnavailable
		if scanned != *m.Scanned {
			return false, ""
		}
	}
	if m.UnsafePodSecurity != nil {
		checks++
		if in.PodSecurity.UnsafePodSecurity() != *m.UnsafePodSecurity {
			return false, ""
		}
	}
	if m.Privileged != nil {
		checks++
		if in.PodSecurity.Privileged != *m.Privileged {
			return false, ""
		}
	}
	if m.RunAsRoot != nil {
		checks++
		if in.PodSecurity.RunAsRoot != *m.RunAsRoot {
			return false, ""
		}
	}
	if m.AllowPrivilegeEscalation != nil {
		checks++
		if in.PodSecurity.AllowPrivilegeEscalation != *m.AllowPrivilegeEscalation {
			return false, ""
		}
	}
	if len(m.NamespaceAllowlist) > 0 {
		checks++
		if !matchesException(m.NamespaceAllowlist, in.Namespace) {
			return false, ""
		}
	}
	if len(m.RegistryAllowlist) > 0 {
		checks++
		// Deny action: match when registry is NOT on the allowlist.
		if p.Action == ActionDeny {
			if containsFold(m.RegistryAllowlist, in.Registry) {
				return false, ""
			}
		} else if !containsFold(m.RegistryAllowlist, in.Registry) {
			return false, ""
		}
	}

	if checks == 0 {
		return false, ""
	}
	return true, fmt.Sprintf("matched policy %q (%s)", p.Name, p.Action)
}

func matchesException(patterns []string, namespace string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(strings.ToLower(namespace), strings.ToLower(prefix)) {
				return true
			}
			continue
		}
		if strings.EqualFold(p, namespace) {
			return true
		}
	}
	return false
}

func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}
