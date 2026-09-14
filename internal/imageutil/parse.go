package imageutil

import (
	"strings"
)

// Ref is a parsed container image reference.
type Ref struct {
	Raw      string
	Registry string
	Name     string
	Tag      string
	Digest   string
}

// Parse splits a container image reference into registry/name/tag/digest.
func Parse(image string) Ref {
	out := Ref{Raw: strings.TrimSpace(image), Tag: "latest"}
	if out.Raw == "" {
		return out
	}
	s := out.Raw
	if i := strings.Index(s, "@"); i >= 0 {
		out.Digest = s[i+1:]
		s = s[:i]
	}
	host, rest := "", s
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		host = parts[0]
		rest = parts[1]
	} else {
		host = "docker.io"
	}
	name, tag := rest, "latest"
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		name = rest[:i]
		tag = rest[i+1:]
	}
	out.Registry = host
	out.Name = name
	out.Tag = tag
	return out
}
