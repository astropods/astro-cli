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

// providerWritablePaths returns emptyDir volume mounts that a provider's container
// needs beyond /tmp. Many third-party images (neo4j, redis, qdrant, etc.) write to
// paths outside /tmp on startup, which fails with readOnlyRootFilesystem: true.
func providerWritablePaths(provider string) []corev1.VolumeMount {
	switch provider {
	case "neo4j":
		return []corev1.VolumeMount{
			{Name: "neo4j-data", MountPath: "/var/lib/neo4j"},
		}
	case "redis":
		return []corev1.VolumeMount{
			{Name: "redis-data", MountPath: "/data"},
		}
	default:
		return nil
	}
}

// providerWritableVolumes returns emptyDir volumes corresponding to providerWritablePaths.
func providerWritableVolumes(provider string) []corev1.Volume {
	mounts := providerWritablePaths(provider)
	vols := make([]corev1.Volume, len(mounts))
	for i, m := range mounts {
		vols[i] = corev1.Volume{
			Name:         m.Name,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		}
	}
	return vols
}

func boolPtr(b bool) *bool {
	return &b
}
