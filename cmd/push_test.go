package cmd

import (
	"regexp"
	"testing"
)

func TestGenerateBuildID(t *testing.T) {
	id := generateBuildID()

	// Should be 8-char hex string
	if len(id) != 8 {
		t.Errorf("generateBuildID() length = %d, want 8", len(id))
	}

	hexRE := regexp.MustCompile(`^[a-f0-9]{8}$`)
	if !hexRE.MatchString(id) {
		t.Errorf("generateBuildID() = %q, want 8-char hex string", id)
	}

	// Two calls should produce different IDs (probabilistic but effectively guaranteed)
	id2 := generateBuildID()
	if id == id2 {
		t.Errorf("generateBuildID() produced same ID twice: %q", id)
	}
}
