package policystore

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/goxray/goxray/internal/policy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	crdGroup    = "katana.dev"
	crdVersion  = "v1alpha1"
	crdResource = "imagepolicies"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SlugName converts a display name to a valid Kubernetes resource name.
func SlugName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "policy"
	}
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-")
	}
	return s
}

func unstructuredToPolicy(obj *unstructured.Unstructured) (policy.Policy, error) {
	if obj == nil {
		return policy.Policy{}, fmt.Errorf("nil object")
	}
	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return policy.Policy{}, fmt.Errorf("missing spec")
	}
	displayName, _ := spec["displayName"].(string)
	if displayName == "" {
		displayName = obj.GetName()
	}
	desc, _ := spec["description"].(string)
	denyMsg, _ := spec["denyMessage"].(string)
	warnMsg, _ := spec["warnMessage"].(string)
	enabled := true
	if v, ok := spec["enabled"].(bool); ok {
		enabled = v
	}
	actionStr, _ := spec["action"].(string)
	if actionStr == "" {
		return policy.Policy{}, fmt.Errorf("missing action")
	}

	match := policy.MatchCriteria{}
	if raw, ok := spec["match"].(map[string]interface{}); ok {
		if v, ok := raw["severity"].(string); ok {
			match.Severity = v
		}
		if v, ok := raw["environment"].(string); ok {
			match.Environment = v
		}
		if v, ok := raw["scanned"].(bool); ok {
			match.Scanned = &v
		}
		match.NamespaceAllowlist = stringSlice(raw["namespaceAllowlist"])
		match.RegistryAllowlist = stringSlice(raw["registryAllowlist"])
		if v, ok := raw["unsafePodSecurity"].(bool); ok {
			match.UnsafePodSecurity = &v
		}
		if v, ok := raw["privileged"].(bool); ok {
			match.Privileged = &v
		}
		if v, ok := raw["runAsRoot"].(bool); ok {
			match.RunAsRoot = &v
		}
		if v, ok := raw["allowPrivilegeEscalation"].(bool); ok {
			match.AllowPrivilegeEscalation = &v
		}
	}

	var exceptions []string
	switch v := spec["exceptions"].(type) {
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				exceptions = append(exceptions, s)
			}
		}
	case []string:
		exceptions = append(exceptions, v...)
	}

	ts := obj.GetCreationTimestamp()
	created := ts.Time
	if created.IsZero() {
		created = time.Now().UTC()
	}

	return policy.Policy{
		ID:          obj.GetName(),
		Name:        displayName,
		Description: desc,
		Enabled:     enabled,
		Action:      policy.Action(actionStr),
		Match:       match,
		Exceptions:  exceptions,
		DenyMessage: denyMsg,
		WarnMessage: warnMsg,
		CreatedAt:   created,
		UpdatedAt:   created,
	}, nil
}

func policyToUnstructured(p policy.Policy) *unstructured.Unstructured {
	name := p.ID
	if name == "" {
		name = SlugName(p.Name)
	}
	match := map[string]interface{}{}
	if p.Match.Severity != "" {
		match["severity"] = p.Match.Severity
	}
	if p.Match.Environment != "" {
		match["environment"] = p.Match.Environment
	}
	if p.Match.Scanned != nil {
		match["scanned"] = *p.Match.Scanned
	}
	if len(p.Match.NamespaceAllowlist) > 0 {
		match["namespaceAllowlist"] = p.Match.NamespaceAllowlist
	}
	if len(p.Match.RegistryAllowlist) > 0 {
		match["registryAllowlist"] = p.Match.RegistryAllowlist
	}
	if p.Match.UnsafePodSecurity != nil {
		match["unsafePodSecurity"] = *p.Match.UnsafePodSecurity
	}
	if p.Match.Privileged != nil {
		match["privileged"] = *p.Match.Privileged
	}
	if p.Match.RunAsRoot != nil {
		match["runAsRoot"] = *p.Match.RunAsRoot
	}
	if p.Match.AllowPrivilegeEscalation != nil {
		match["allowPrivilegeEscalation"] = *p.Match.AllowPrivilegeEscalation
	}
	spec := map[string]interface{}{
		"displayName": p.Name,
		"description": p.Description,
		"enabled":     p.Enabled,
		"action":      string(p.Action),
		"match":       match,
	}
	if p.DenyMessage != "" {
		spec["denyMessage"] = p.DenyMessage
	}
	if p.WarnMessage != "" {
		spec["warnMessage"] = p.WarnMessage
	}
	if len(p.Exceptions) > 0 {
		spec["exceptions"] = p.Exceptions
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": crdGroup + "/" + crdVersion,
			"kind":       "ImagePolicy",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": spec,
		},
	}
}

func applyPatchToPolicy(p policy.Policy, patch map[string]json.RawMessage) policy.Policy {
	if v, ok := patch["name"]; ok {
		_ = json.Unmarshal(v, &p.Name)
	}
	if v, ok := patch["description"]; ok {
		_ = json.Unmarshal(v, &p.Description)
	}
	if v, ok := patch["enabled"]; ok {
		_ = json.Unmarshal(v, &p.Enabled)
	}
	if v, ok := patch["action"]; ok {
		var a string
		_ = json.Unmarshal(v, &a)
		p.Action = policy.Action(a)
	}
	if v, ok := patch["match"]; ok {
		_ = json.Unmarshal(v, &p.Match)
	}
	if v, ok := patch["exceptions"]; ok {
		_ = json.Unmarshal(v, &p.Exceptions)
	}
	if v, ok := patch["deny_message"]; ok {
		_ = json.Unmarshal(v, &p.DenyMessage)
	}
	if v, ok := patch["warn_message"]; ok {
		_ = json.Unmarshal(v, &p.WarnMessage)
	}
	return p
}

func stringSlice(v interface{}) []string {
	switch arr := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string{}, arr...)
	default:
		return nil
	}
}
