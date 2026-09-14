package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/goxray/goxray/internal/evaluate"
	"github.com/goxray/goxray/internal/metrics"
	"github.com/goxray/goxray/internal/policy"
)

const (
	defaultAdmissionTimeout     = 8 * time.Second
	defaultAdmissionMaxInflight = 16
)

// Handler is a Kubernetes ValidatingAdmissionWebhook for Pods.
type Handler struct {
	Eval        *evaluate.Service
	DryRun      bool
	evalTimeout time.Duration
	inflight    chan struct{}
}

// NewHandler builds an admission handler. DryRun can also be forced via KATANA_ADMISSION_DRY_RUN=true.
func NewHandler(eval *evaluate.Service, dryRun bool) *Handler {
	if strings.EqualFold(os.Getenv("KATANA_ADMISSION_DRY_RUN"), "true") {
		dryRun = true
	}
	return &Handler{
		Eval:        eval,
		DryRun:      dryRun,
		evalTimeout: admissionTimeout(),
		inflight:    make(chan struct{}, admissionMaxInflight()),
	}
}

func admissionTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("KATANA_ADMISSION_TIMEOUT"))
	if raw == "" {
		return defaultAdmissionTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Second {
		return defaultAdmissionTimeout
	}
	return d
}

func admissionMaxInflight() int {
	raw := strings.TrimSpace(os.Getenv("KATANA_ADMISSION_MAX_INFLIGHT"))
	if raw == "" {
		return defaultAdmissionMaxInflight
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultAdmissionMaxInflight
	}
	return n
}

type admissionReview struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Request    *admissionRequest `json:"request,omitempty"`
	Response   *admissionResponse `json:"response,omitempty"`
}

type admissionRequest struct {
	UID         string          `json:"uid"`
	Kind        metaGroupKind   `json:"kind"`
	SubResource string          `json:"subResource"`
	Namespace   string          `json:"namespace"`
	Object      json.RawMessage `json:"object"`
}

type metaGroupKind struct {
	Kind string `json:"kind"`
}

type admissionResponse struct {
	UID      string   `json:"uid"`
	Allowed  bool     `json:"allowed"`
	Warnings []string `json:"warnings,omitempty"`
	Result   *status  `json:"status,omitempty"`
}

type status struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Code    int32  `json:"code,omitempty"`
}

type podObject struct {
	Metadata struct {
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		SecurityContext     *securityContext `json:"securityContext"`
		InitContainers      []containerSpec  `json:"initContainers"`
		Containers          []containerSpec  `json:"containers"`
		EphemeralContainers []containerSpec  `json:"ephemeralContainers"`
	} `json:"spec"`
}

// ServeHTTP handles AdmissionReview requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		metrics.ObserveAdmission("error", "error", time.Since(start))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var review admissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		metrics.ObserveAdmission("error", "error", time.Since(start))
		http.Error(w, "invalid AdmissionReview", http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		metrics.ObserveAdmission("error", "error", time.Since(start))
		http.Error(w, "missing request", http.StatusBadRequest)
		return
	}
	uid := review.Request.UID

	if h.inflight != nil {
		select {
		case h.inflight <- struct{}{}:
			defer func() { <-h.inflight }()
		default:
			msg := "admission overloaded"
			metrics.ObserveAdmission("overloaded", "error", time.Since(start))
			h.writeReview(w, uid, h.DryRun, msg)
			return
		}
	}

	timeout := h.evalTimeout
	if timeout <= 0 {
		timeout = defaultAdmissionTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	timedOut := false
	allowed, msg, results := h.admit(ctx, review.Request)
	if ctx.Err() != nil && allowed {
		allowed = false
		timedOut = true
		msg = failClosedTimeoutMessage(review.Request)
	}
	origAllowed := allowed
	deployed := allowed
	if h.DryRun {
		allowed = true
		deployed = true
		if msg == "" {
			msg = "dry-run: would allow"
		} else {
			msg = "dry-run: " + msg
		}
	}
	h.recordResults(r.Context(), results, deployed)
	h.observeReview(time.Since(start), origAllowed, timedOut, results)
	h.writeReview(w, uid, allowed, msg)
}

func (h *Handler) writeReview(w http.ResponseWriter, uid string, allowed bool, msg string) {
	resp := &admissionResponse{UID: uid, Allowed: allowed}
	if !allowed {
		resp.Result = &status{
			Status:  "Failure",
			Message: msg,
			Reason:  "Forbidden",
			Code:    http.StatusForbidden,
		}
	} else if msg != "" {
		resp.Warnings = []string{msg}
	}
	out := admissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Response:   resp,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func isPodAdmission(req *admissionRequest) bool {
	if req.Kind.Kind == "Pod" {
		return true
	}
	return strings.EqualFold(req.SubResource, "ephemeralcontainers")
}

func (h *Handler) admit(ctx context.Context, req *admissionRequest) (bool, string, []evaluate.Result) {
	if !isPodAdmission(req) {
		return true, "", nil
	}
	var pod podObject
	if err := json.Unmarshal(req.Object, &pod); err != nil {
		return false, "unable to parse pod object", nil
	}
	if h.Eval == nil {
		return false, "evaluator not ready", nil
	}
	ns := req.Namespace
	if ns == "" {
		ns = pod.Metadata.Namespace
	}
	env := resolveEnvironment(ns)

	ps := analyzePodSecurity(&pod)
	if ps.Unsafe && h.Eval != nil {
		policies, err := h.Eval.Policies.List(ctx)
		if err != nil {
			return false, fmt.Sprintf("list policies: %v", err), nil
		}
		podDec := h.Eval.Evaluator.Evaluate(policies, policy.EvaluationInput{
			Namespace:   ns,
			Environment: env,
			Severity:    "info",
			Scanned:     true,
			Registry:    "artifactory.example.com",
			PodSecurity: ps,
		})
		if !podDec.Allowed {
			res := evaluate.Result{
				Namespace: ns,
				Decision:  podDec,
				Input:     policy.EvaluationInput{Namespace: ns, Environment: env, PodSecurity: ps},
			}
			if len(ps.Reasons) > 0 {
				res.Decision.Reasons = append(res.Decision.Reasons, ps.Reasons...)
			}
			if len(collectImages(&pod)) > 0 {
				res.Image = collectImages(&pod)[0]
				res.Input.Image = res.Image
			}
			msg := podDec.UserMessage
			if msg == "" {
				msg = policy.FormatAdmissionMessage(findPolicyByName(policies, podDec.MatchedPolicy), res.Input, res.Decision.Reasons)
			}
			return false, msg, []evaluate.Result{res}
		}
	}

	images := collectImages(&pod)
	var warnings []string
	var results []evaluate.Result
	for _, img := range images {
		res, err := h.Eval.EvaluateImage(ctx, evaluate.Request{
			Image:       img,
			Namespace:   ns,
			Environment: env,
			Record:      false,
		})
		if err != nil {
			return false, fmt.Sprintf("evaluate %s: %v", img, err), results
		}
		results = append(results, res)
		if !res.Decision.Allowed {
			msg := res.Decision.UserMessage
			if msg == "" {
				msg = policy.FormatAdmissionMessage(nil, res.Input, res.Decision.Reasons)
			}
			return false, msg, results
		}
		if res.Decision.UserMessage != "" && (res.Decision.Action == policy.ActionWarn || res.Decision.Action == policy.ActionAudit) {
			warnings = append(warnings, res.Decision.UserMessage)
		} else if string(res.Decision.Action) == "warn" || string(res.Decision.Action) == "audit" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", img, res.Decision.MatchedPolicy))
		}
	}
	return true, strings.Join(warnings, " | "), results
}

func (h *Handler) observeReview(d time.Duration, origAllowed, timedOut bool, results []evaluate.Result) {
	result := "allow"
	if timedOut {
		result = "timeout"
	} else if !origAllowed {
		result = "deny"
	}
	outcome := classifyOutcome(h.DryRun, origAllowed, timedOut, results)
	metrics.ObserveAdmission(result, outcome, d)
	for _, res := range results {
		if res.Input.Severity != "" {
			metrics.ObserveSeverity(res.Input.Severity)
		}
		if res.Decision.MatchedPolicy != "" {
			action := string(res.Decision.Action)
			if action == "" && res.Decision.Allowed {
				action = "allow"
			}
			metrics.ObservePolicy(res.Decision.MatchedPolicy, action)
		}
	}
}

func classifyOutcome(dryRun, origAllowed, failed bool, results []evaluate.Result) string {
	if failed {
		return "error"
	}
	if !origAllowed {
		return "blocked"
	}
	wouldDeny := false
	scanned := true
	hasEval := false
	for _, res := range results {
		hasEval = true
		if !res.Decision.Allowed {
			wouldDeny = true
		}
		if res.Scan == nil || !res.Scan.Scanned {
			scanned = false
		}
	}
	if dryRun && wouldDeny {
		return "dry_run_would_deny"
	}
	if hasEval && !scanned {
		return "unscanned_allowed"
	}
	return "compliant"
}

func (h *Handler) recordResults(ctx context.Context, results []evaluate.Result, deployed bool) {
	if h.Eval == nil || len(results) == 0 {
		return
	}
	d := deployed
	for _, res := range results {
		_ = h.Eval.RecordDetection(ctx, res, "admission", &d, h.DryRun)
	}
}

func collectImages(pod *podObject) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(img string) {
		img = strings.TrimSpace(img)
		if img == "" {
			return
		}
		if _, ok := seen[img]; ok {
			return
		}
		seen[img] = struct{}{}
		out = append(out, img)
	}
	for _, c := range pod.Spec.InitContainers {
		add(c.Image)
	}
	for _, c := range pod.Spec.Containers {
		add(c.Image)
	}
	for _, c := range pod.Spec.EphemeralContainers {
		add(c.Image)
	}
	return out
}

func failClosedTimeoutMessage(req *admissionRequest) string {
	ns := ""
	img := ""
	if req != nil {
		ns = req.Namespace
		var pod podObject
		if json.Unmarshal(req.Object, &pod) == nil {
			if ns == "" {
				ns = pod.Metadata.Namespace
			}
			if imgs := collectImages(&pod); len(imgs) > 0 {
				img = imgs[0]
			}
		}
	}
	p := &policy.Policy{
		Name:        "Require Scanned Image",
		Action:      policy.ActionDeny,
		DenyMessage: policy.DefaultDenyMessages()["Require Scanned Image"],
	}
	return policy.FormatAdmissionMessage(p, policy.EvaluationInput{
		Image:           img,
		Namespace:       ns,
		Scanned:         false,
		XrayUnavailable: true,
	}, []string{"JFrog Xray lookup timed out; treated as unscanned"})
}

func findPolicyByName(policies []policy.Policy, name string) *policy.Policy {
	for i := range policies {
		if policies[i].Name == name || policies[i].ID == name {
			return &policies[i]
		}
	}
	return nil
}
