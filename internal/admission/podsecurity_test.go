package admission

import (
	"testing"
)

func TestAnalyzePodSecurity_RootAndPrivileged(t *testing.T) {
	uid0 := int64(0)
	priv := true
	esc := true
	pod := podObject{}
	pod.Spec.Containers = []containerSpec{{
		Name:  "app",
		Image: "demo:latest",
		SecurityContext: &securityContext{
			RunAsUser:                &uid0,
			Privileged:               &priv,
			AllowPrivilegeEscalation: &esc,
		},
	}}
	ps := analyzePodSecurity(&pod)
	if !ps.Unsafe || !ps.RunAsRoot || !ps.Privileged || !ps.AllowPrivilegeEscalation {
		t.Fatalf("expected unsafe pod: %+v", ps)
	}
}

func TestAnalyzePodSecurity_SafeNonRoot(t *testing.T) {
	uid1000 := int64(1000)
	nonRoot := true
	escFalse := false
	pod := podObject{}
	pod.Spec.Containers = []containerSpec{{
		Name:  "app",
		Image: "demo:latest",
		SecurityContext: &securityContext{
			RunAsUser:                &uid1000,
			RunAsNonRoot:             &nonRoot,
			AllowPrivilegeEscalation: &escFalse,
		},
	}}
	ps := analyzePodSecurity(&pod)
	if ps.Unsafe {
		t.Fatalf("expected safe pod: %+v", ps)
	}
}

func TestAnalyzePodSecurity_EmptyContextUnsafe(t *testing.T) {
	pod := podObject{}
	pod.Spec.Containers = []containerSpec{{
		Name:  "app",
		Image: "demo:latest",
	}}
	ps := analyzePodSecurity(&pod)
	if !ps.Unsafe || !ps.RunAsRoot || !ps.AllowPrivilegeEscalation {
		t.Fatalf("empty securityContext should be unsafe (implicit root + default privilege escalation): %+v", ps)
	}
}

func TestAnalyzePodSecurity_EmptyPodLevelContextIgnored(t *testing.T) {
	uid1000 := int64(1000)
	nonRoot := true
	escFalse := false
	pod := podObject{}
	pod.Spec.SecurityContext = &securityContext{}
	pod.Spec.Containers = []containerSpec{{
		Name:  "app",
		Image: "demo:latest",
		SecurityContext: &securityContext{
			RunAsUser:                &uid1000,
			RunAsNonRoot:             &nonRoot,
			AllowPrivilegeEscalation: &escFalse,
		},
	}}
	ps := analyzePodSecurity(&pod)
	if ps.Unsafe {
		t.Fatalf("empty pod securityContext must not mark a restricted container unsafe: %+v", ps)
	}
}

func TestAnalyzePodSecurity_RunAsNonRootFalse(t *testing.T) {
	nonRoot := false
	pod := podObject{}
	pod.Spec.Containers = []containerSpec{{
		Name:  "app",
		Image: "demo:latest",
		SecurityContext: &securityContext{RunAsNonRoot: &nonRoot},
	}}
	ps := analyzePodSecurity(&pod)
	if !ps.RunAsRoot || !ps.Unsafe {
		t.Fatalf("runAsNonRoot=false should count as root risk: %+v", ps)
	}
}
