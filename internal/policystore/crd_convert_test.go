package policystore

import (
	"testing"

	"github.com/goxray/goxray/internal/policy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSlugName(t *testing.T) {
	if got := SlugName("Block Critical"); got != "block-critical" {
		t.Fatalf("got %q", got)
	}
	if got := SlugName("Registry Allowlist"); got != "registry-allowlist" {
		t.Fatalf("got %q", got)
	}
}

func TestPolicyCRDRoundTrip(t *testing.T) {
	scanned := false
	in := policyFromTest()
	in.ID = "registry-allowlist"
	in.Match.Scanned = &scanned
	in.DenyMessage = "Image {image} blocked by {policy}."
	obj := policyToUnstructured(in)
	out, err := unstructuredToPolicy(obj)
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != in.Name || out.Action != in.Action || out.DenyMessage != in.DenyMessage {
		t.Fatalf("round trip mismatch: %+v vs %+v", out, in)
	}
}

func TestPoliciesFromCache_FailClosedOnBadCRD(t *testing.T) {
	good := policyToUnstructured(policyFromTest())
	good.SetName("registry-allowlist")
	bad := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "katana.dev/v1alpha1",
		"kind":       "ImagePolicy",
		"metadata":   map[string]interface{}{"name": "broken-deny"},
		"spec":       map[string]interface{}{"displayName": "Broken"},
	}}
	if _, err := policiesFromCache([]interface{}{good, bad}); err == nil {
		t.Fatal("expected error when a deny CRD is malformed")
	}
}

func policyFromTest() policy.Policy {
	return policy.Policy{
		Name:        "Registry Allowlist",
		Description: "test",
		Enabled:     true,
		Action:      policy.ActionDeny,
		Match:       policy.MatchCriteria{RegistryAllowlist: []string{"artifactory.example.com"}},
		Exceptions:  []string{"kube-system"},
	}
}
