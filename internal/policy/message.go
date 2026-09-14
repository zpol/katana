package policy

import (
	"fmt"
	"strings"
)

// MessageVars holds template variables for policy user-facing messages.
type MessageVars struct {
	Policy      string
	Image       string
	Severity    string
	Environment string
	Namespace   string
	Registry    string
	Reason      string
}

// RenderMessage substitutes {policy}, {image}, {severity}, {environment}, {namespace}, {registry}, {reason}.
func RenderMessage(tmpl string, vars MessageVars) string {
	if strings.TrimSpace(tmpl) == "" {
		return ""
	}
	repl := strings.NewReplacer(
		"{policy}", vars.Policy,
		"{image}", vars.Image,
		"{severity}", vars.Severity,
		"{environment}", vars.Environment,
		"{namespace}", vars.Namespace,
		"{registry}", vars.Registry,
		"{reason}", vars.Reason,
	)
	return strings.TrimSpace(repl.Replace(tmpl))
}

// FormatAdmissionMessage builds the developer-facing admission message for a matched policy.
func FormatAdmissionMessage(p *Policy, in EvaluationInput, reasons []string) string {
	if p == nil {
		return defaultAdmissionMessage(in, "", reasons)
	}
	reason := firstReason(reasons)
	vars := MessageVars{
		Policy:      p.Name,
		Image:       in.Image,
		Severity:    in.Severity,
		Environment: in.Environment,
		Namespace:   in.Namespace,
		Registry:    in.Registry,
		Reason:      reason,
	}
	var tmpl string
	switch p.Action {
	case ActionDeny:
		tmpl = p.DenyMessage
	case ActionWarn:
		tmpl = p.WarnMessage
	default:
		tmpl = p.WarnMessage
	}
	if msg := RenderMessage(tmpl, vars); msg != "" {
		return msg
	}
	if strings.TrimSpace(p.Description) != "" && p.Action == ActionDeny {
		return p.Description
	}
	return defaultAdmissionMessage(in, p.Name, reasons)
}

func defaultAdmissionMessage(in EvaluationInput, policyName string, reasons []string) string {
	reason := firstReason(reasons)
	if policyName != "" && in.Image != "" {
		return fmt.Sprintf("KATANA policy %q blocked image %s. %s", policyName, in.Image, reason)
	}
	if policyName != "" {
		return fmt.Sprintf("KATANA policy %q denied this workload. %s", policyName, reason)
	}
	if reason != "" {
		return fmt.Sprintf("KATANA denied this workload. %s", reason)
	}
	return "KATANA denied this workload."
}

func firstReason(reasons []string) string {
	for _, r := range reasons {
		r = strings.TrimSpace(r)
		if r != "" && !strings.HasPrefix(r, "matched policy") {
			return r
		}
	}
	if len(reasons) > 0 {
		return strings.TrimSpace(reasons[0])
	}
	return ""
}

// DefaultDenyMessages returns suggested deny messages for built-in policy templates.
func DefaultDenyMessages() map[string]string {
	return map[string]string{
		"Block Critical": `Image {image} has CRITICAL vulnerabilities (JFrog Xray). Policy: {policy}.
Upgrade the base image or contact your platform security team for an exception.`,

		"Block High in Prod": `Image {image} has HIGH severity findings in production (env: {environment}). Policy: {policy}.
Use a patched image version or deploy to a non-prod namespace for testing.`,

		"Require Scanned Image": `Image {image} is not indexed in JFrog Xray (scanned=false). Policy: {policy}.
Ensure the image is scanned in Artifactory/Xray before deploying.`,

		"Registry Allowlist": `Image {image} uses registry "{registry}" which is not on the approved list. Policy: {policy}.
Pull from artifactory.example.com or an approved registry.`,

		"Deny Unsafe Pod Security": `Pod security violation in namespace {namespace}. Policy: {policy}. {reason}
Do not run as root (UID 0), privileged, or with allowPrivilegeEscalation.`,

		"Allowlist System NS": `System namespace activity recorded. Policy: {policy}.`,
	}
}
