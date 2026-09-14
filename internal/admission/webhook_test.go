package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/goxray/goxray/internal/evaluate"
	"github.com/goxray/goxray/internal/jfrog"
	"github.com/goxray/goxray/internal/policy"
	"github.com/goxray/goxray/internal/policystore"
	"github.com/goxray/goxray/internal/store"
)

func TestWriteReviewDenySetsFailureStatus(t *testing.T) {
	h := &Handler{}
	rr := httptest.NewRecorder()
	msg := "Image evil.example/app:1 has CRITICAL vulnerabilities (JFrog Xray). Policy: Block Critical."
	h.writeReview(rr, "uid-1", false, msg)

	var rev admissionReview
	if err := json.Unmarshal(rr.Body.Bytes(), &rev); err != nil {
		t.Fatal(err)
	}
	if rev.Response == nil || rev.Response.Allowed {
		t.Fatalf("expected deny: %+v", rev.Response)
	}
	got := rev.Response.Result
	if got == nil {
		t.Fatal("missing result")
	}
	if got.Status != "Failure" || got.Reason != "Forbidden" || got.Code != http.StatusForbidden {
		t.Fatalf("result status fields: %+v", got)
	}
	if got.Message != msg {
		t.Fatalf("message=%q", got.Message)
	}
	raw := rr.Body.Bytes()
	// Kubernetes AdmissionResponse encodes metav1.Status as JSON key "status", not "result".
	if bytes.Contains(raw, []byte(`"result"`)) {
		t.Fatalf("must not use json key result: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"status":{"status":"Failure"`)) {
		t.Fatalf("expected response.status.status=Failure, got %s", raw)
	}
}

func TestAdmitPodDenyIncludesFailureAndPolicyMessage(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SeedDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{
		Eval: &evaluate.Service{
			Policies:  policystore.NewSQLite(st),
			Events:    st,
			JFrog:     jfrog.NewHTTPClient("", ""),
			Evaluator: policy.NewEvaluator(),
		},
		DryRun: false,
	}
	body := []byte(`{
		"apiVersion":"admission.k8s.io/v1",
		"kind":"AdmissionReview",
		"request":{
			"uid":"deny-1",
			"namespace":"katana-poc-demo",
			"kind":{"kind":"Pod"},
			"object":{
				"spec":{
					"containers":[{
						"name":"app",
						"image":"docker.io/library/nginx:1.25",
						"securityContext":{"runAsNonRoot":true,"runAsUser":101,"allowPrivilegeEscalation":false}
					}]
				}
			}
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var rev admissionReview
	if err := json.Unmarshal(rr.Body.Bytes(), &rev); err != nil {
		t.Fatal(err)
	}
	if rev.Response == nil || rev.Response.Allowed || rev.Response.Result == nil {
		t.Fatalf("expected deny with result: %+v", rev.Response)
	}
	if rev.Response.Result.Status != "Failure" || rev.Response.Result.Reason != "Forbidden" {
		t.Fatalf("result=%+v", rev.Response.Result)
	}
	if rev.Response.Result.Message == "" {
		t.Fatal("empty deny message")
	}
	if !bytes.Contains([]byte(rev.Response.Result.Message), []byte("Policy:")) {
		t.Fatalf("expected policy denyMessage, got %q", rev.Response.Result.Message)
	}
}

func TestFailClosedTimeoutMessage(t *testing.T) {
	msg := failClosedTimeoutMessage(&admissionRequest{
		Namespace: "payments",
		Object:    json.RawMessage(`{"spec":{"containers":[{"image":"artifactory.example.com/app:1"}]}}`),
	})
	if !bytes.Contains([]byte(msg), []byte("Require Scanned Image")) {
		t.Fatalf("expected Require Scanned denyMessage, got %q", msg)
	}
	if !bytes.Contains([]byte(msg), []byte("artifactory.example.com/app:1")) {
		t.Fatalf("expected image in message, got %q", msg)
	}
}

func TestAdmitNonPodAllowed(t *testing.T) {
	h := &Handler{Eval: nil, DryRun: false}
	body := []byte(`{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"1","kind":{"kind":"ConfigMap"},"object":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var rev admissionReview
	if err := json.Unmarshal(rr.Body.Bytes(), &rev); err != nil {
		t.Fatal(err)
	}
	if rev.Response == nil || !rev.Response.Allowed {
		t.Fatalf("expected allow non-pod: %+v", rev.Response)
	}
}

func TestIsPodAdmissionIncludesEphemeral(t *testing.T) {
	if isPodAdmission(&admissionRequest{Kind: metaGroupKind{Kind: "ConfigMap"}}) {
		t.Fatal("configmap must not be treated as pod")
	}
	if !isPodAdmission(&admissionRequest{Kind: metaGroupKind{Kind: "Pod"}}) {
		t.Fatal("pod kind")
	}
	if !isPodAdmission(&admissionRequest{SubResource: "ephemeralcontainers"}) {
		t.Fatal("ephemeralcontainers subresource")
	}
}

func TestEvaluateServiceDeniesUnscannedEvilRegistry(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SeedDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := &evaluate.Service{
		Policies:  policystore.NewSQLite(st),
		Events:    st,
		JFrog:     jfrog.NewHTTPClient("", ""),
		Evaluator: policy.NewEvaluator(),
	}
	res, err := svc.EvaluateImage(context.Background(), evaluate.Request{
		Image:     "evil.example/app:1",
		Namespace: "payments",
		Record:    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Allowed {
		t.Fatalf("expected deny for unscanned evil registry: %+v", res.Decision)
	}
}

func TestClassifyOutcome(t *testing.T) {
	scanned := evaluate.Result{
		Decision: policy.Decision{Allowed: true},
		Scan:     &jfrog.ScanSummary{Scanned: true, LookupStatus: "indexed"},
	}
	blocked := evaluate.Result{
		Decision: policy.Decision{Allowed: false, Action: policy.ActionDeny, MatchedPolicy: "Block Critical"},
		Scan:     &jfrog.ScanSummary{Scanned: true, LookupStatus: "indexed"},
	}
	unscanned := evaluate.Result{
		Decision: policy.Decision{Allowed: true},
		Scan:     &jfrog.ScanSummary{Scanned: false, LookupStatus: "not_indexed"},
	}
	cases := []struct {
		dry, allowed, failed bool
		res                  []evaluate.Result
		want                 string
	}{
		{false, true, false, []evaluate.Result{scanned}, "compliant"},
		{false, false, false, []evaluate.Result{blocked}, "blocked"},
		{true, false, false, []evaluate.Result{blocked}, "blocked"},
		{true, true, false, []evaluate.Result{blocked}, "dry_run_would_deny"},
		{false, true, false, []evaluate.Result{unscanned}, "unscanned_allowed"},
		{false, true, true, nil, "error"},
	}
	for _, tc := range cases {
		got := classifyOutcome(tc.dry, tc.allowed, tc.failed, tc.res)
		if got != tc.want {
			t.Fatalf("classify(dry=%v allowed=%v fail=%v)=%q want %q", tc.dry, tc.allowed, tc.failed, got, tc.want)
		}
	}
}
