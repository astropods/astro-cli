package k8s

import (
	"fmt"

	"github.com/postman/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// BuildResourceRequirements converts DeploymentResources to k8s ResourceRequirements.
// Returns nil if all fields are empty (caller should apply its own default).
func BuildResourceRequirements(res spec.DeploymentResources) *corev1.ResourceRequirements {
	if res.CPU == "" && res.Memory == "" && res.CPULimit == "" && res.MemoryLimit == "" {
		return nil
	}

	reqs := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	if res.CPU != "" {
		reqs.Requests[corev1.ResourceCPU] = resource.MustParse(res.CPU)
	}
	if res.Memory != "" {
		reqs.Requests[corev1.ResourceMemory] = resource.MustParse(res.Memory)
	}
	if res.CPULimit != "" {
		reqs.Limits[corev1.ResourceCPU] = resource.MustParse(res.CPULimit)
	}
	if res.MemoryLimit != "" {
		reqs.Limits[corev1.ResourceMemory] = resource.MustParse(res.MemoryLimit)
	}

	return reqs
}

// BuildResourceRequirementsWithGPU converts DeploymentResources + GPU config to k8s ResourceRequirements.
func BuildResourceRequirementsWithGPU(res spec.DeploymentResources, gpu *spec.DeploymentGPU) *corev1.ResourceRequirements {
	reqs := BuildResourceRequirements(res)
	if reqs == nil {
		return nil
	}

	if gpu != nil {
		count := gpu.Count
		if count == 0 {
			count = 1
		}
		gpuQty := resource.MustParse(fmt.Sprintf("%d", count))
		reqs.Requests["nvidia.com/gpu"] = gpuQty
		reqs.Limits["nvidia.com/gpu"] = gpuQty
	}

	return reqs
}

// BuildDeploymentStrategy converts an UpdateStrategy to a k8s DeploymentStrategy.
func BuildDeploymentStrategy(us spec.UpdateStrategy) *appsv1.DeploymentStrategy {
	switch us.Strategy {
	case "recreate":
		return &appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
	case "rolling", "":
		strategy := &appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
		}
		if us.MaxUnavailable != "" || us.MaxSurge != "" {
			rollingUpdate := &appsv1.RollingUpdateDeployment{}
			if us.MaxUnavailable != "" {
				val := intstr.Parse(us.MaxUnavailable)
				rollingUpdate.MaxUnavailable = &val
			}
			if us.MaxSurge != "" {
				val := intstr.Parse(us.MaxSurge)
				rollingUpdate.MaxSurge = &val
			}
			strategy.RollingUpdate = rollingUpdate
		}
		return strategy
	default:
		return nil
	}
}

// BuildStatefulSetUpdateStrategy converts an UpdateStrategy to a k8s StatefulSetUpdateStrategy.
func BuildStatefulSetUpdateStrategy(us spec.UpdateStrategy) *appsv1.StatefulSetUpdateStrategy {
	switch us.Strategy {
	case "recreate":
		// StatefulSets don't have "Recreate" — use OnDelete which requires manual pod deletion
		return &appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.OnDeleteStatefulSetStrategyType,
		}
	case "rolling", "":
		return &appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		}
	default:
		return nil
	}
}

// BuildGPUNodeSelector returns a node selector map for GPU workloads.
func BuildGPUNodeSelector(gpu *spec.DeploymentGPU) map[string]string {
	if gpu == nil {
		return nil
	}
	runtime := gpu.Runtime
	if runtime == "" || runtime == "cuda" {
		return map[string]string{"workload-type": "gpu"}
	}
	if runtime == "rocm" {
		return map[string]string{"workload-type": "gpu"}
	}
	return nil
}

// BuildGPUTolerations returns tolerations for GPU-tainted nodes.
func BuildGPUTolerations(gpu *spec.DeploymentGPU) []corev1.Toleration {
	if gpu == nil {
		return nil
	}
	return []corev1.Toleration{
		{
			Key:      "nvidia.com/gpu",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
}
