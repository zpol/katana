package ecr

import "testing"

func TestIsECRHost(t *testing.T) {
	cases := map[string]bool{
		"123456789012.dkr.ecr.eu-west-1.amazonaws.com": true,
		"123456789012.dkr.ecr.us-east-1.amazonaws.com": true,
		"artifactory.example.com":                      false,
		"public.ecr.aws":                               false,
		"docker.io":                                    false,
	}
	for host, want := range cases {
		if got := IsECRHost(host); got != want {
			t.Errorf("IsECRHost(%q)=%v want %v", host, got, want)
		}
	}
}

func TestParseImage(t *testing.T) {
	ref, ok := ParseImage("123456789012.dkr.ecr.eu-west-1.amazonaws.com/team/app:1.2.3")
	if !ok {
		t.Fatal("expected ok")
	}
	if ref.AccountID != "123456789012" || ref.Region != "eu-west-1" || ref.Repository != "team/app" || ref.Tag != "1.2.3" {
		t.Fatalf("unexpected parse: %+v", ref)
	}
	ref2, ok := ParseImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/app@sha256:abc")
	if !ok || ref2.Digest != "abc" || ref2.Repository != "app" {
		t.Fatalf("digest parse: %+v ok=%v", ref2, ok)
	}
}
