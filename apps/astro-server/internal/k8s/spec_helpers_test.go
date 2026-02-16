package k8s

import (
	"testing"

	"github.com/postman/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildResourceRequirements_Empty(t *testing.T) {
	res := BuildResourceRequirements(spec.DeploymentResources{})
	if res != nil {
		t.Error("expected nil for empty resources")
	}
}

func TestBuildResourceRequirements_Standard(t *testing.T) {
	res := BuildResourceRequirements(spec.StandardResources)
	if res == nil {
		t.Fatal("expected non-nil for standard resources")
	}
	if res.Requests.Cpu().String() != "100m" {
		t.Errorf("cpu request: expected 100m, got %s", res.Requests.Cpu().String())
	}
	if res.Requests.Memory().String() != "256Mi" {
		t.Errorf("memory request: expected 256Mi, got %s", res.Requests.Memory().String())
	}
	if res.Limits.Cpu().String() != "1" {
		t.Errorf("cpu limit: expected 1, got %s", res.Limits.Cpu().String())
	}
	if res.Limits.Memory().String() != "1Gi" {
		t.Errorf("memory limit: expected 1Gi, got %s", res.Limits.Memory().String())
	}
}

func TestBuildResourceRequirements_GPU(t *testing.T) {
	res := BuildResourceRequirementsWithGPU(spec.GPUResources, &spec.DeploymentGPU{
		VRAM: "24Gi", Runtime: "cuda", Count: 2,
	})
	if res == nil {
		t.Fatal("expected non-nil for GPU resources")
	}
	gpuQty := res.Requests["nvidia.com/gpu"]
	if gpuQty.String() != "2" {
		t.Errorf("gpu count: expected 2, got %s", gpuQty.String())
	}
	// GPU should be in both requests and limits
	gpuLimit := res.Limits["nvidia.com/gpu"]
	if gpuLimit.String() != "2" {
		t.Errorf("gpu limit: expected 2, got %s", gpuLimit.String())
	}
}

func TestBuildResourceRequirements_GPUDefaultCount(t *testing.T) {
	res := BuildResourceRequirementsWithGPU(spec.GPUResources, &spec.DeploymentGPU{
		VRAM: "24Gi", Runtime: "cuda",
	})
	gpuQty := res.Requests["nvidia.com/gpu"]
	if gpuQty.String() != "1" {
		t.Errorf("default gpu count: expected 1, got %s", gpuQty.String())
	}
}

func TestBuildResourceRequirements_PartialFields(t *testing.T) {
	res := BuildResourceRequirements(spec.DeploymentResources{
		CPU: "200m", Memory: "512Mi",
	})
	if res == nil {
		t.Fatal("expected non-nil for partial resources")
	}
	if res.Requests.Cpu().String() != "200m" {
		t.Errorf("cpu: expected 200m, got %s", res.Requests.Cpu().String())
	}
	// Limits should be empty (not set)
	if !res.Limits.Cpu().IsZero() {
		t.Error("cpu limit should be zero when not set")
	}
}

func TestBuildDeploymentStrategy_Rolling(t *testing.T) {
	strategy := BuildDeploymentStrategy(spec.UpdateStrategy{
		Strategy: "rolling", MaxUnavailable: "1", MaxSurge: "2",
	})
	if strategy == nil {
		t.Fatal("expected non-nil for rolling strategy")
	}
	if strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
		t.Errorf("type: expected RollingUpdate, got %s", strategy.Type)
	}
	if strategy.RollingUpdate == nil {
		t.Fatal("expected RollingUpdate config")
	}
	if strategy.RollingUpdate.MaxUnavailable.String() != "1" {
		t.Errorf("maxUnavailable: expected 1, got %s", strategy.RollingUpdate.MaxUnavailable.String())
	}
	if strategy.RollingUpdate.MaxSurge.String() != "2" {
		t.Errorf("maxSurge: expected 2, got %s", strategy.RollingUpdate.MaxSurge.String())
	}
}

func TestBuildDeploymentStrategy_RollingPercentage(t *testing.T) {
	strategy := BuildDeploymentStrategy(spec.UpdateStrategy{
		Strategy: "rolling", MaxUnavailable: "25%", MaxSurge: "25%",
	})
	if strategy.RollingUpdate.MaxUnavailable.String() != "25%" {
		t.Errorf("maxUnavailable: expected 25%%, got %s", strategy.RollingUpdate.MaxUnavailable.String())
	}
}

func TestBuildDeploymentStrategy_Recreate(t *testing.T) {
	strategy := BuildDeploymentStrategy(spec.UpdateStrategy{Strategy: "recreate"})
	if strategy == nil {
		t.Fatal("expected non-nil for recreate strategy")
	}
	if strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Errorf("type: expected Recreate, got %s", strategy.Type)
	}
}

func TestBuildDeploymentStrategy_Empty(t *testing.T) {
	strategy := BuildDeploymentStrategy(spec.UpdateStrategy{})
	if strategy == nil {
		t.Fatal("expected non-nil for empty strategy (defaults to rolling)")
	}
	if strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
		t.Errorf("type: expected RollingUpdate, got %s", strategy.Type)
	}
}

func TestBuildStatefulSetUpdateStrategy_Recreate(t *testing.T) {
	strategy := BuildStatefulSetUpdateStrategy(spec.UpdateStrategy{Strategy: "recreate"})
	if strategy == nil {
		t.Fatal("expected non-nil")
	}
	if strategy.Type != appsv1.OnDeleteStatefulSetStrategyType {
		t.Errorf("type: expected OnDelete (recreate maps to OnDelete for StatefulSets), got %s", strategy.Type)
	}
}

func TestBuildStatefulSetUpdateStrategy_Rolling(t *testing.T) {
	strategy := BuildStatefulSetUpdateStrategy(spec.UpdateStrategy{Strategy: "rolling"})
	if strategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("type: expected RollingUpdate, got %s", strategy.Type)
	}
}

func TestBuildGPUNodeSelector_Cuda(t *testing.T) {
	sel := BuildGPUNodeSelector(&spec.DeploymentGPU{Runtime: "cuda"})
	if sel["accelerator"] != "nvidia-gpu" {
		t.Errorf("expected nvidia-gpu, got %s", sel["accelerator"])
	}
}

func TestBuildGPUNodeSelector_ROCM(t *testing.T) {
	sel := BuildGPUNodeSelector(&spec.DeploymentGPU{Runtime: "rocm"})
	if sel["accelerator"] != "amd-gpu" {
		t.Errorf("expected amd-gpu, got %s", sel["accelerator"])
	}
}

func TestBuildGPUNodeSelector_DefaultRuntime(t *testing.T) {
	sel := BuildGPUNodeSelector(&spec.DeploymentGPU{})
	if sel["accelerator"] != "nvidia-gpu" {
		t.Errorf("expected nvidia-gpu for default runtime, got %s", sel["accelerator"])
	}
}

func TestBuildGPUNodeSelector_Nil(t *testing.T) {
	sel := BuildGPUNodeSelector(nil)
	if sel != nil {
		t.Error("expected nil for nil GPU")
	}
}

func TestBuildDeployment_WithSpecDrivenFields(t *testing.T) {
	resources := BuildResourceRequirements(spec.DeploymentResources{
		CPU: "500m", Memory: "1Gi", CPULimit: "2", MemoryLimit: "4Gi",
	})
	strategy := BuildDeploymentStrategy(spec.UpdateStrategy{
		Strategy: "rolling", MaxUnavailable: "1", MaxSurge: "1",
	})

	cfg := DeploymentConfig{
		Name: "test-deploy", Namespace: "ns", AgentName: "agent",
		BuildID: "b1", Component: "model-llm",
		Container: spec.ContainerConfig{Image: "test:latest"},
		Port: 8080, Replicas: 3, Resources: resources,
		Strategy:     strategy,
		NodeSelector: map[string]string{"accelerator": "nvidia-gpu"},
	}

	depl := BuildDeployment(cfg)
	if *depl.Spec.Replicas != 3 {
		t.Errorf("replicas: expected 3, got %d", *depl.Spec.Replicas)
	}
	if depl.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
		t.Errorf("strategy: expected RollingUpdate, got %s", depl.Spec.Strategy.Type)
	}
	container := depl.Spec.Template.Spec.Containers[0]
	if container.Resources.Requests.Cpu().String() != "500m" {
		t.Errorf("cpu request: expected 500m, got %s", container.Resources.Requests.Cpu().String())
	}
	if depl.Spec.Template.Spec.NodeSelector["accelerator"] != "nvidia-gpu" {
		t.Error("expected nvidia-gpu node selector")
	}
}

func TestBuildStatefulSet_WithSpecDrivenFields(t *testing.T) {
	resources := BuildResourceRequirements(spec.DeploymentResources{
		CPU: "500m", Memory: "2Gi", CPULimit: "2", MemoryLimit: "8Gi",
	})

	cfg := StatefulSetConfig{
		Name: "test-ss", Namespace: "ns", AgentName: "agent",
		BuildID: "b1", Component: "knowledge-docs",
		Container: spec.ContainerConfig{Image: "qdrant:latest"},
		Port: 6333, Provider: "qdrant",
		Replicas:     2,
		Resources:    resources,
		StorageSize:  "50Gi",
		StorageClass: "gp3",
		AccessMode:   corev1.ReadWriteMany,
	}

	ss := BuildStatefulSet(cfg)
	if *ss.Spec.Replicas != 2 {
		t.Errorf("replicas: expected 2, got %d", *ss.Spec.Replicas)
	}
	container := ss.Spec.Template.Spec.Containers[0]
	if container.Resources.Requests.Cpu().String() != "500m" {
		t.Errorf("cpu request: expected 500m, got %s", container.Resources.Requests.Cpu().String())
	}

	// Check PVC
	pvc := ss.Spec.VolumeClaimTemplates[0]
	storageQty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageQty.String() != "50Gi" {
		t.Errorf("storage: expected 50Gi, got %s", storageQty.String())
	}
	if *pvc.Spec.StorageClassName != "gp3" {
		t.Errorf("storage class: expected gp3, got %s", *pvc.Spec.StorageClassName)
	}
	if pvc.Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Errorf("access mode: expected ReadWriteMany, got %s", pvc.Spec.AccessModes[0])
	}
}
