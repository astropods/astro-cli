package deviceid

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withSource(t *testing.T, fn func() (string, error)) {
	t.Helper()
	prev := Source
	Source = fn
	t.Cleanup(func() { Source = prev })
}

func TestID_IsAStableHashThatNeverContainsTheRawMachineID(t *testing.T) {
	withSource(t, func() (string, error) { return "  4C4C4544-0042-1010-8051-B4C04F4E4E32\n", nil })
	fallback := filepath.Join(t.TempDir(), "device-id")

	first, err := ID(fallback)
	require.NoError(t, err)
	second, err := ID(fallback)
	require.NoError(t, err)
	assert.Equal(t, first, second, "the same machine must always report the same device")
	assert.Len(t, first, 32)
	assert.NotContains(t, first, "4C4C4544", "the OS machine id must not leave the machine")
	_, statErr := os.Stat(fallback)
	assert.True(t, os.IsNotExist(statErr), "with an OS machine id, nothing is written to disk")
}

func TestID_DiffersFromAPlainHashOfTheMachineID(t *testing.T) {
	withSource(t, func() (string, error) { return "machine-1", nil })
	got, err := ID(filepath.Join(t.TempDir(), "device-id"))
	require.NoError(t, err)
	assert.NotEqual(t, hashWithoutDomain("machine-1"), got, "the domain prefix keeps this id unlinkable to other tools' hashes")
}

func TestID_FallsBackToARandomIDKeptOnDisk(t *testing.T) {
	withSource(t, func() (string, error) { return "", errors.New("no machine id") })
	fallback := filepath.Join(t.TempDir(), "nested", "device-id")

	first, err := ID(fallback)
	require.NoError(t, err)
	second, err := ID(fallback)
	require.NoError(t, err)
	assert.Equal(t, first, second, "the fallback must be created once and reused")
	info, err := os.Stat(fallback)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestOSMachineID_ReturnsSomethingOnThisMachine(t *testing.T) {
	id, err := osMachineID()
	if err != nil {
		t.Skipf("this OS exposes no machine id here: %v", err)
	}
	assert.NotEmpty(t, id)
}

func hashWithoutDomain(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}
