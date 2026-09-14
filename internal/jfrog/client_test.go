package jfrog

import (
	"strings"
	"testing"
)

func TestDockerManifestPathsProjectRepo(t *testing.T) {
	image := "artifactory.example.com/myproj-docker-prod-local/build-docker-images/dev/generic-base:1.5.7"
	paths := dockerManifestPaths(image)
	want := "myproj/build-docker-images/dev/generic-base/1.5.7/manifest.json"
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
	image := "artifactory.example.com/docker-prod-remote-cache/nginxinc/nginx-unprivileged:1.29-alpine-slim"
	paths := dockerManifestPaths(image)
	want := "docker-prod-remote-cache/nginxinc/nginx-unprivileged/1.29-alpine-slim/list.manifest.json"
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
			t.Fatalf("shared repo must not get docker/ project alias: %q", p)
		}
	}
}

func TestDockerManifestPathsFederatedRepo(t *testing.T) {
	image := "artifactory.example.com/team-docker-dev-federated/legacy-registry/demo-app:latest"
	paths := dockerManifestPaths(image)
	want := "team-docker-dev-federated/legacy-registry/demo-app/latest/manifest.json"
	if len(paths) == 0 || paths[0] != want {
		t.Fatalf("expected first path %q, got %v", want, paths)
	}
}

func TestXrayProjectPrefix(t *testing.T) {
	cases := map[string]string{
		"myproj-docker-prod-local":       "myproj",
		"docker-prod-remote-cache":       "",
		"k8s-docker-prod-remote":         "",
		"docker-quay-prod-remote-cache":  "",
	}
	for repo, want := range cases {
		if got := xrayProjectPrefix(repo); got != want {
			t.Fatalf("xrayProjectPrefix(%q) = %q, want %q", repo, got, want)
		}
	}
}
