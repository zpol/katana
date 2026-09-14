package admission

import (
	"fmt"

	"github.com/zpol/katana/internal/policy"
)

type securityContext struct {
	RunAsUser                *int64 `json:"runAsUser"`
	RunAsNonRoot             *bool  `json:"runAsNonRoot"`
	Privileged               *bool  `json:"privileged"`
	AllowPrivilegeEscalation *bool  `json:"allowPrivilegeEscalation"`
}

type containerSpec struct {
	Name            string           `json:"name"`
	Image           string           `json:"image"`
	SecurityContext *securityContext `json:"securityContext"`
}

func analyzePodSecurity(pod *podObject) policy.PodSecurityAnalysis {
	var out policy.PodSecurityAnalysis
	podSC := podSpecSecurityContext(pod)

	check := func(name string, sc *securityContext) {
		if isPrivileged(sc) {
			out.Privileged = true
			out.Reasons = append(out.Reasons, fmt.Sprintf("container %q is privileged", name))
		}
		if runsAsRoot(sc, podSC) {
			out.RunAsRoot = true
			out.Reasons = append(out.Reasons, fmt.Sprintf("container %q runs as root (UID 0)", name))
		}
		if allowsPrivilegeEscalation(sc, podSC) {
			out.AllowPrivilegeEscalation = true
			out.Reasons = append(out.Reasons, fmt.Sprintf("container %q allows privilege escalation", name))
		}
	}

	// Pod-level securityContext is inheritance only (runAsUser / runAsNonRoot).
	// Kubernetes always materializes spec.securityContext: {} and that object
	// has no allowPrivilegeEscalation — treating it as a container false-denies
	// every Pod as "Deny Unsafe Pod Security".
	for _, c := range pod.Spec.InitContainers {
		check(containerName(c.Name, c.Image, "init"), c.SecurityContext)
	}
	for _, c := range pod.Spec.Containers {
		check(containerName(c.Name, c.Image, "container"), c.SecurityContext)
	}
	for _, c := range pod.Spec.EphemeralContainers {
		check(containerName(c.Name, c.Image, "ephemeral"), c.SecurityContext)
	}

	out.Unsafe = out.Privileged || out.RunAsRoot || out.AllowPrivilegeEscalation
	return out
}

func podSpecSecurityContext(pod *podObject) *securityContext {
	return pod.Spec.SecurityContext
}

func containerName(name, image, fallback string) string {
	if name != "" {
		return name
	}
	if image != "" {
		return image
	}
	return fallback
}

func isPrivileged(sc *securityContext) bool {
	return sc != nil && sc.Privileged != nil && *sc.Privileged
}

func runsAsRoot(sc, podSC *securityContext) bool {
	if uid := effectiveRunAsUser(sc, podSC); uid != nil {
		return *uid == 0
	}
	if nonRoot := effectiveRunAsNonRoot(sc, podSC); nonRoot != nil {
		return !*nonRoot
	}
	// Unspecified UID typically runs as the image USER (often 0). Fail-closed.
	return true
}

func allowsPrivilegeEscalation(sc, podSC *securityContext) bool {
	if sc != nil && sc.AllowPrivilegeEscalation != nil {
		return *sc.AllowPrivilegeEscalation
	}
	if podSC != nil && podSC.AllowPrivilegeEscalation != nil {
		return *podSC.AllowPrivilegeEscalation
	}
	// Kubernetes default is true when the field is omitted.
	return true
}

func effectiveRunAsUser(sc, podSC *securityContext) *int64 {
	if sc != nil && sc.RunAsUser != nil {
		return sc.RunAsUser
	}
	if podSC != nil && podSC.RunAsUser != nil {
		return podSC.RunAsUser
	}
	return nil
}

func effectiveRunAsNonRoot(sc, podSC *securityContext) *bool {
	if sc != nil && sc.RunAsNonRoot != nil {
		return sc.RunAsNonRoot
	}
	if podSC != nil && podSC.RunAsNonRoot != nil {
		return podSC.RunAsNonRoot
	}
	return nil
}
