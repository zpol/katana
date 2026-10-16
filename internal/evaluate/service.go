package evaluate

import (
	"context"
	"fmt"
	"strings"

	"github.com/zpol/katana/internal/detect"
	"github.com/zpol/katana/internal/imageutil"
	"github.com/zpol/katana/internal/policy"
	"github.com/zpol/katana/internal/policystore"
	"github.com/zpol/katana/internal/scan"
	"github.com/zpol/katana/internal/store"
)

// Request is an image evaluation request.
type Request struct {
	Image       string `json:"image"`
	Namespace   string `json:"namespace"`
	Environment string `json:"environment"`
	DryRun      bool   `json:"dry_run"`
	Record      bool   `json:"record"`
	Source      string `json:"source,omitempty"`
	Deployed    *bool  `json:"deployed,omitempty"`
}

// Result is the evaluation outcome for one image.
type Result struct {
	Image     string                 `json:"image"`
	Registry  string                 `json:"registry"`
	Namespace string                 `json:"namespace"`
	Scan      *scan.Summary          `json:"scan"`
	Decision  policy.Decision        `json:"decision"`
	Input     policy.EvaluationInput `json:"input"`
}

// Service evaluates images against policies using scanner data when available.
type Service struct {
	Policies  policystore.Store
	Events    *store.Store
	Scanner   scan.Client
	Evaluator *policy.Evaluator
}

// EvaluateImage loads policies, queries the routed scanner, and evaluates.
func (s *Service) EvaluateImage(ctx context.Context, req Request) (Result, error) {
	ref := imageutil.Parse(req.Image)
	policies, err := s.Policies.List(ctx)
	if err != nil {
		return Result{}, err
	}

	sum := scan.Unavailable(scan.ArtifactRef{ImageRef: req.Image}, "", "scanner client missing")
	if s.Scanner != nil {
		sum, err = s.Scanner.GetScanSummary(ctx, scan.ArtifactRef{
			ImageRef: req.Image,
			Sha:      strings.TrimPrefix(ref.Digest, "sha256:"),
		})
		if err != nil {
			return Result{}, err
		}
	}

	sev := highestSeverity(sum)
	if sev == "unknown" {
		sev = "info"
	}
	in := policy.EvaluationInput{
		Severity:        sev,
		Environment:     req.Environment,
		Scanned:         sum != nil && sum.Scanned,
		XrayUnavailable: sum != nil && sum.LookupStatus == scan.StatusUnavailable,
		Namespace:       req.Namespace,
		Registry:        ref.Registry,
		Image:           req.Image,
	}
	dec := s.Evaluator.Evaluate(policies, in)
	if sum != nil && sum.LookupStatus == scan.StatusUnavailable {
		if dec.Allowed {
			scanner := sum.Scanner
			if scanner == "" {
				scanner = "scanner"
			}
			dec.UserMessage = fmt.Sprintf("%s lookup failed or timed out; treated as unscanned.", scannerLabel(scanner))
			if dec.Action == policy.ActionAudit {
				dec.Action = policy.ActionWarn
			}
		}
	}

	out := Result{
		Image:     req.Image,
		Registry:  ref.Registry,
		Namespace: req.Namespace,
		Scan:      sum,
		Decision:  dec,
		Input:     in,
	}

	if req.Record {
		source := req.Source
		if source == "" {
			source = "evaluate"
		}
		_, _ = s.Events.CreateDetection(ctx, s.detectionFromResult(out, source, req.Deployed, req.DryRun))
	}
	return out, nil
}

func scannerLabel(name string) string {
	switch name {
	case "jfrog":
		return "JFrog Xray"
	case "ecr_inspector":
		return "ECR Inspector"
	default:
		return "Scanner"
	}
}

// RecordDetection persists an evaluation result (used by admission webhook after final outcome).
func (s *Service) RecordDetection(ctx context.Context, res Result, source string, deployed *bool, dryRun bool) error {
	_, err := s.Events.CreateDetection(ctx, s.detectionFromResult(res, source, deployed, dryRun))
	return err
}

func (s *Service) detectionFromResult(res Result, source string, deployed *bool, dryRun bool) detect.Detection {
	title := fmt.Sprintf("%s — %s", res.Image, res.Decision.Action)
	desc := res.Decision.UserMessage
	if desc == "" {
		desc = strings.Join(res.Decision.Reasons, "; ")
	}
	if res.Scan != nil && len(res.Scan.Violations) > 0 {
		max := 5
		if len(res.Scan.Violations) < max {
			max = len(res.Scan.Violations)
		}
		desc = desc + " | " + strings.Join(res.Scan.Violations[:max], ", ")
	}
	sev := res.Input.Severity
	if sev == "" || sev == "unknown" {
		sev = "info"
	}
	return detect.Detection{
		Title:        title,
		Description:  desc,
		Severity:     detect.Severity(sev),
		Environment:  res.Input.Environment,
		Namespace:    res.Namespace,
		Registry:     res.Registry,
		Image:        res.Image,
		Scanned:      res.Input.Scanned,
		Source:       source,
		PolicyAction: string(res.Decision.Action),
		Deployed:     deployed,
		DryRun:       dryRun,
	}
}

func highestSeverity(sum *scan.Summary) string {
	if sum == nil || !sum.Scanned {
		return "unknown"
	}
	switch {
	case sum.Critical > 0:
		return "critical"
	case sum.High > 0:
		return "high"
	case sum.Medium > 0:
		return "medium"
	case sum.Low > 0:
		return "low"
	default:
		return "info"
	}
}
