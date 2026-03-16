package k8s

import (
	corev1 "k8s.io/api/core/v1"
)

// restrictedPodSecurityContext returns a PodSecurityContext that satisfies the
// Kubernetes "restricted" Pod Security Standard.
func restrictedPodSecurityContext() *corev1.PodSecurityContext {
	uid := int64(1000)
	return &corev1.PodSecurityContext{
		RunAsNonRoot: boolPtr(true),
		RunAsUser:    &uid,
		FSGroup:      &uid,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// restrictedContainerSecurityContext returns a SecurityContext that satisfies the
// Kubernetes "restricted" Pod Security Standard for individual containers.
func restrictedContainerSecurityContext() *corev1.SecurityContext {
	uid := int64(1000)
	return &corev1.SecurityContext{
		RunAsNonRoot:             boolPtr(true),
		RunAsUser:                &uid,
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// hardenPodSpec applies restricted security defaults to a PodSpec:
//   - Pod-level SecurityContext (runAsNonRoot, runAsUser, fsGroup, seccomp)
//   - automountServiceAccountToken: false
func hardenPodSpec(podSpec *corev1.PodSpec) {
	podSpec.SecurityContext = restrictedPodSecurityContext()

	noMount := false
	podSpec.AutomountServiceAccountToken = &noMount
}

// hardenContainer applies restricted security defaults to a Container:
//   - Container-level SecurityContext
func hardenContainer(c *corev1.Container) {
	c.SecurityContext = restrictedContainerSecurityContext()
}

func boolPtr(b bool) *bool {
	return &b
}
