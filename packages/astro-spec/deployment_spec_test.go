package spec

import (
	"testing"
)

func TestDefaultUpdateStrategy(t *testing.T) {
	s := DefaultUpdateStrategy()
	if s.Strategy != "rolling" {
		t.Errorf("expected rolling, got %s", s.Strategy)
	}
	if s.MaxUnavailable != "25%" {
		t.Errorf("expected 25%%, got %s", s.MaxUnavailable)
	}
	if s.MaxSurge != "25%" {
		t.Errorf("expected 25%%, got %s", s.MaxSurge)
	}
}

func TestDefaultStorageConfig(t *testing.T) {
	s := DefaultStorageConfig()
	if s.Size != "10Gi" {
		t.Errorf("expected 10Gi, got %s", s.Size)
	}
	if s.AccessMode != "ReadWriteOnce" {
		t.Errorf("expected ReadWriteOnce, got %s", s.AccessMode)
	}
	if s.Class != "" {
		t.Errorf("expected empty class, got %s", s.Class)
	}
}

func TestStandardResources(t *testing.T) {
	r := StandardResources
	if r.CPU != "100m" {
		t.Errorf("expected 100m, got %s", r.CPU)
	}
	if r.Memory != "256Mi" {
		t.Errorf("expected 256Mi, got %s", r.Memory)
	}
	if r.CPULimit != "1" {
		t.Errorf("expected 1, got %s", r.CPULimit)
	}
	if r.MemoryLimit != "1Gi" {
		t.Errorf("expected 1Gi, got %s", r.MemoryLimit)
	}
}

func TestGPUResources(t *testing.T) {
	r := GPUResources
	if r.CPU != "2" {
		t.Errorf("expected 2, got %s", r.CPU)
	}
	if r.Memory != "8Gi" {
		t.Errorf("expected 8Gi, got %s", r.Memory)
	}
}

func TestMessagingResources(t *testing.T) {
	r := MessagingResources
	if r.CPU != "100m" {
		t.Errorf("expected 100m, got %s", r.CPU)
	}
	if r.MemoryLimit != "512Mi" {
		t.Errorf("expected 512Mi, got %s", r.MemoryLimit)
	}
}

func TestCollectorResources(t *testing.T) {
	r := CollectorResources
	if r.CPU != "50m" {
		t.Errorf("expected 50m, got %s", r.CPU)
	}
	if r.MemoryLimit != "256Mi" {
		t.Errorf("expected 256Mi, got %s", r.MemoryLimit)
	}
}
