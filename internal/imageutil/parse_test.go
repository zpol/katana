package imageutil

import "testing"

func TestParse(t *testing.T) {
	r := Parse("artifactory.example.com/team/app:1.2.3")
	if r.Registry != "artifactory.example.com" || r.Name != "team/app" || r.Tag != "1.2.3" {
		t.Fatalf("unexpected: %+v", r)
	}
	r2 := Parse("nginx")
	if r2.Registry != "docker.io" || r2.Name != "nginx" || r2.Tag != "latest" {
		t.Fatalf("unexpected: %+v", r2)
	}
	r3 := Parse("123456789012.dkr.ecr.us-east-1.amazonaws.com/pause@sha256:abc")
	if r3.Digest != "sha256:abc" || r3.Name != "pause" {
		t.Fatalf("unexpected: %+v", r3)
	}
}
