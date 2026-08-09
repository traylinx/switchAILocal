package management

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteConfigCreatesAndMigratesPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, WriteConfig(path, []byte("port: 18080\n")))
	for _, oldMode := range []os.FileMode{0o644, 0o666} {
		require.NoError(t, os.Chmod(path, oldMode))
		require.NoError(t, WriteConfig(path, []byte("port: 18081\n")))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	anchored := filepath.Join(filepath.Dir(path), "anchored.yaml")
	require.NoError(t, os.Link(path, anchored))
	require.NoError(t, WriteConfig(anchored, []byte("port: 18082\n")))
	targetInfo, err := os.Stat(path)
	require.NoError(t, err)
	anchoredInfo, err := os.Stat(anchored)
	require.NoError(t, err)
	assert.True(t, os.SameFile(targetInfo, anchoredInfo))
}
