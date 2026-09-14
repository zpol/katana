package jfrog

import (
	"strings"
	"testing"
)

func TestDockerManifestPathsProjectRepo(t *testing.T) {
	image := "artifactory.example.com/example-docker-local/demo/app:1.0"
	paths := dockerManifestPaths(image)
	want := "example/demo/app/1.0/manifest.json"
	found := false
	for _, p := range paths {
		if p == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Xray project prefix path %q in %v", want, paths)
	}
}

func TestDockerManifestPathsSharedRepoUnchanged(t *testing.T) {
	// Repos whose first segment is a reserved slug (docker-/k8s-/global-) do not
	// get an extra Xray project-prefix alias.
	image := "artifactory.example.com/docker-remote-cache/nginxinc/nginx-unprivileged:1.29-alpine-slim"
	paths := dockerManifestPaths(image)
	want := "docker-remote-cache/nginxinc/nginx-unprivileged/1.29-alpine-slim/list.manifest.json"
	found := false
	for _, p := range paths {
		if p == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected shared repo path %q in %v", want, paths)
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "docker/") {
			t.Fatalf("reserved-slug repo must not get project alias: %q", p)
		}
	}
}

func TestDockerManifestPathsFederatedRepo(t *testing.T) {
	image := "artifactory.example.com/demo-docker-dev/legacy-registry/demo-app:latest"
	paths := dockerManifestPaths(image)
	want := "demo-docker-dev/legacy-registry/demo-app/latest/manifest.json"
	if len(paths) == 0 || paths[0] != want {
		t.Fatalf("expected first path %q, got %v", want, paths)
	}
}

func TestXrayProjectPrefix(t *testing.T) {
	cases := map[string]string{
		"example-docker-local": "example",
		"demo-docker-prod":     "demo",
		"docker-remote-cache":  "",
		"k8s-docker-remote":    "",
		"example-quay-cache":   "",
	}
	for repo, want := range cases {
		if got := xrayProjectPrefix(repo); got != want {
			t.Fatalf("xrayProjectPrefix(%q) = %q, want %q", repo, got, want)
		}
	}
}
