package policy

import "testing"

func TestRenderMessage(t *testing.T) {
	got := RenderMessage("Image {image} blocked by {policy} in {namespace}", MessageVars{
		Policy:    "Block Critical",
		Image:     "artifactory.example.com/app:1.0",
		Namespace: "demo",
	})
	want := "Image artifactory.example.com/app:1.0 blocked by Block Critical in demo"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatAdmissionMessage_CustomDeny(t *testing.T) {
	p := &Policy{
		Name:        "Block Critical",
		Action:      ActionDeny,
		DenyMessage: "CVE alert on {image}. Policy: {policy}.",
	}
	msg := FormatAdmissionMessage(p, EvaluationInput{
		Image:    "artifactory.example.com/app:1.0",
		Severity: "critical",
	}, []string{"matched policy"})
	if msg != "CVE alert on artifactory.example.com/app:1.0. Policy: Block Critical." {
		t.Fatalf("unexpected: %q", msg)
	}
}

func TestFormatAdmissionMessage_FallbackDescription(t *testing.T) {
	p := &Policy{
		Name:        "Require Scanned Image",
		Description: "Deny images with no Xray scan result",
		Action:      ActionDeny,
	}
	msg := FormatAdmissionMessage(p, EvaluationInput{Image: "artifactory.example.com/x:y"}, nil)
	if msg != "Deny images with no Xray scan result" {
		t.Fatalf("unexpected: %q", msg)
	}
}

func TestFormatAdmissionMessage_DefaultGenerated(t *testing.T) {
	p := &Policy{Name: "Block High in Prod", Action: ActionDeny}
	msg := FormatAdmissionMessage(p, EvaluationInput{
		Image: "artifactory.example.com/api:2.0",
	}, []string{"severity high"})
	if msg == "" {
		t.Fatal("expected non-empty default message")
	}
}
