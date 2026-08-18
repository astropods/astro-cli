package evaldataset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestItemIDIsStable(t *testing.T) {
	assert.Equal(t,
		"904fa7c743b2e6956aefd147591bd3c89ed1873260fc8711546416e2c00a4e9e",
		ItemID("eval-dep-1", "trace-1"),
	)
}

func TestItemIDSeparatesDatasetFromTrace(t *testing.T) {
	assert.NotEqual(t, ItemID("ab", "c"), ItemID("a", "bc"))
}

func TestItemIDDependsOnBothParts(t *testing.T) {
	base := ItemID("eval-dep-1", "trace-1")
	assert.NotEqual(t, base, ItemID("eval-dep-2", "trace-1"))
	assert.NotEqual(t, base, ItemID("eval-dep-1", "trace-2"))
}
