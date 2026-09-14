package policy

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestEvaluate_DenyCritical(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:    "Block Critical",
		Enabled: true,
		Action:  ActionDeny,
		Match:   MatchCriteria{Severity: "critical"},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Severity: "critical"})
	if d.Allowed || d.Action != ActionDeny || d.MatchedPolicy != "Block Critical" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestEvaluate_DenyHighInProdOnly(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:    "Block High in Prod",
		Enabled: true,
		Action:  ActionDeny,
		Match:   MatchCriteria{Severity: "high", Environment: "prod"},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Severity: "high", Environment: "prod"})
	if d.Allowed {
		t.Fatalf("expected deny in prod: %+v", d)
	}
	d2 := ev.Evaluate(policies, EvaluationInput{Severity: "high", Environment: "dev"})
	if !d2.Allowed {
		t.Fatalf("expected allow in dev: %+v", d2)
	}
}

func TestEvaluate_RequireScannedImage(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:    "Require Scanned Image",
		Enabled: true,
		Action:  ActionDeny,
		Match:   MatchCriteria{Scanned: boolPtr(false)},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Scanned: false})
	if d.Allowed {
		t.Fatalf("expected deny unscanned: %+v", d)
	}
	d2 := ev.Evaluate(policies, EvaluationInput{Scanned: true})
	if !d2.Allowed {
		t.Fatalf("expected allow scanned: %+v", d2)
	}
}

func TestEvaluate_ExceptionsSkipDeny(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:       "Block Critical",
		Enabled:    true,
		Action:     ActionDeny,
		Match:      MatchCriteria{Severity: "critical"},
		Exceptions: []string{"kube-system", "cattle-*"},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Severity: "critical", Namespace: "kube-system"})
	if !d.Allowed || d.MatchedPolicy != "" {
		t.Fatalf("exception should skip deny: %+v", d)
	}
	d2 := ev.Evaluate(policies, EvaluationInput{Severity: "critical", Namespace: "cattle-system"})
	if !d2.Allowed {
		t.Fatalf("wildcard exception should skip: %+v", d2)
	}
	d3 := ev.Evaluate(policies, EvaluationInput{Severity: "critical", Namespace: "payments"})
	if d3.Allowed {
		t.Fatalf("expected deny outside exceptions: %+v", d3)
	}
}

func TestEvaluate_RegistryAllowlist(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:    "Registry Allowlist",
		Enabled: true,
		Action:  ActionDeny,
		Match: MatchCriteria{RegistryAllowlist: []string{
			"artifactory.example.com", "*.dkr.ecr.eu-west-3.amazonaws.com",
		}},
	}}
	// Exact allowlist only for containsFold — wildcards on registry are exact strings in MVP.
	d := ev.Evaluate(policies, EvaluationInput{Registry: "evil.example"})
	if d.Allowed {
		t.Fatalf("expected deny unknown registry: %+v", d)
	}
	d2 := ev.Evaluate(policies, EvaluationInput{Registry: "artifactory.example.com"})
	if !d2.Allowed {
		t.Fatalf("expected allow approved registry: %+v", d2)
	}
}

func TestEvaluate_DisabledIgnored(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:    "Block Critical",
		Enabled: false,
		Action:  ActionDeny,
		Match:   MatchCriteria{Severity: "critical"},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Severity: "critical"})
	if !d.Allowed || d.MatchedPolicy != "" {
		t.Fatalf("disabled policy should not match: %+v", d)
	}
}

func TestEvaluate_WarnAndAudit(t *testing.T) {
	ev := NewEvaluator()
	policies := []Policy{{
		Name:    "Warn High",
		Enabled: true,
		Action:  ActionWarn,
		Match:   MatchCriteria{Severity: "high"},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Severity: "high"})
	if !d.Allowed || d.Action != ActionWarn {
		t.Fatalf("expected warn: %+v", d)
	}
}

func TestEvaluate_RequireScannedDeniesWhenXrayUnavailable(t *testing.T) {
	ev := NewEvaluator()
	scannedFalse := false
	policies := []Policy{{
		Name:    "Require Scanned Image",
		Enabled: true,
		Action:  ActionDeny,
		Match:   MatchCriteria{Scanned: &scannedFalse},
	}}
	d := ev.Evaluate(policies, EvaluationInput{Scanned: false, XrayUnavailable: true})
	if d.Allowed {
		t.Fatalf("expected deny when xray unavailable: %+v", d)
	}
	d2 := ev.Evaluate(policies, EvaluationInput{Scanned: true, XrayUnavailable: true})
	if d2.Allowed {
		t.Fatalf("expected deny when scanned=true but xray unavailable: %+v", d2)
	}
	d3 := ev.Evaluate(policies, EvaluationInput{Scanned: false})
	if d3.Allowed {
		t.Fatalf("expected deny when unscanned: %+v", d3)
	}
}

func TestEvaluate_UnsafePodSecurity(t *testing.T) {
	ev := NewEvaluator()
	unsafe := true
	policies := []Policy{{
		Name:    "Deny Unsafe Pod Security",
		Enabled: true,
		Action:  ActionDeny,
		Match:   MatchCriteria{UnsafePodSecurity: &unsafe},
	}}
	d := ev.Evaluate(policies, EvaluationInput{
		Namespace: "demo",
		PodSecurity: PodSecurityAnalysis{RunAsRoot: true, Unsafe: true},
	})
	if d.Allowed || d.MatchedPolicy != "Deny Unsafe Pod Security" {
		t.Fatalf("expected deny unsafe pod: %+v", d)
	}
	d2 := ev.Evaluate(policies, EvaluationInput{
		Namespace:   "demo",
		PodSecurity: PodSecurityAnalysis{},
	})
	if !d2.Allowed {
		t.Fatalf("expected allow safe pod: %+v", d2)
	}
}
