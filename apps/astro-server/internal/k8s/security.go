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
		ReadOnlyRootFilesystem:   boolPtr(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// tmpVolume returns an emptyDir Volume named "tmp" for containers that need
// writable scratch space when readOnlyRootFilesystem is true.
func tmpVolume() corev1.Volume {
	return corev1.Volume{
		Name: "tmp",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// tmpVolumeMount returns a VolumeMount for /tmp backed by the "tmp" emptyDir.
func tmpVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "tmp",
		MountPath: "/tmp",
	}
}

// hardenPodSpec applies restricted security defaults to a PodSpec:
//   - Pod-level SecurityContext (runAsNonRoot, runAsUser, fsGroup, seccomp)
//   - automountServiceAccountToken: false
//   - A shared /tmp emptyDir volume (for readOnlyRootFilesystem compatibility)
func hardenPodSpec(podSpec *corev1.PodSpec) {
	podSpec.SecurityContext = restrictedPodSecurityContext()

	noMount := false
	podSpec.AutomountServiceAccountToken = &noMount

	podSpec.Volumes = append(podSpec.Volumes, tmpVolume())
}

// hardenContainer applies restricted security defaults to a Container:
//   - Container-level SecurityContext
//   - /tmp VolumeMount for writable scratch space
func hardenContainer(c *corev1.Container) {
	c.SecurityContext = restrictedContainerSecurityContext()
	c.VolumeMounts = append(c.VolumeMounts, tmpVolumeMount())
}

func boolPtr(b bool) *bool {
	return &b
}
