package privatefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCreatesAndMigratesPrivateFileWithoutChangingExistingParent(t *testing.T) {
	root := t.TempDir()
	freshParent := filepath.Join(root, "fresh")
	freshPath := filepath.Join(freshParent, "secret.json")
	require.NoError(t, Write(freshPath, []byte("fresh")))
	info, err := os.Stat(freshParent)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	existingParent := filepath.Join(root, "operator-owned")
	require.NoError(t, os.Mkdir(existingParent, 0o755))
	require.NoError(t, os.Chmod(existingParent, 0o755))
	path := filepath.Join(existingParent, "secret.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	for _, oldMode := range []os.FileMode{0o644, 0o666} {
		require.NoError(t, os.Chmod(path, oldMode))
		require.NoError(t, Write(path, []byte("private")))
		info, err = os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	info, err = os.Stat(existingParent)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "private", string(data))
	matches, err := filepath.Glob(filepath.Join(existingParent, ".secret.json.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestWriteAnchoredPreservesSymlinkHardLinkAndWritableFileInReadOnlyParent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))
	require.NoError(t, os.Chmod(target, 0o644))
	symlink := filepath.Join(root, "symlink.json")
	require.NoError(t, os.Symlink(target, symlink))

	require.NoError(t, WriteAnchored(symlink, []byte("symlink")))
	linkInfo, err := os.Lstat(symlink)
	require.NoError(t, err)
	assert.NotZero(t, linkInfo.Mode()&os.ModeSymlink)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "symlink", string(data))
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	hardLink := filepath.Join(root, "hardlink.json")
	require.NoError(t, os.Link(target, hardLink))
	require.NoError(t, WriteAnchored(hardLink, []byte("hardlink")))
	data, err = os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "hardlink", string(data))
	targetInfo, err := os.Stat(target)
	require.NoError(t, err)
	hardLinkInfo, err := os.Stat(hardLink)
	require.NoError(t, err)
	assert.True(t, os.SameFile(targetInfo, hardLinkInfo))

	lockedParent := filepath.Join(root, "locked-parent")
	require.NoError(t, os.Mkdir(lockedParent, 0o755))
	lockedPath := filepath.Join(lockedParent, "config.yaml")
	require.NoError(t, os.WriteFile(lockedPath, []byte("old"), 0o600))
	require.NoError(t, os.Chmod(lockedParent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(lockedParent, 0o755) })
	require.NoError(t, WriteAnchored(lockedPath, []byte("updated")))
	data, err = os.ReadFile(lockedPath)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(data))
}

func TestWriteFallsBackWhenParentCannotCreateAtomicTemp(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "locked-parent")
	require.NoError(t, os.Mkdir(parent, 0o755))
	path := filepath.Join(parent, "cache.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	require.NoError(t, os.Chmod(parent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	require.NoError(t, Write(path, []byte("fallback")))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "fallback", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWriteFallsBackWhenAtomicRenameIsRejected(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "mounted-config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))
	wantRenameErr := errors.New("rename rejected")

	err := writeWithOps(
		path,
		[]byte("fallback"),
		os.CreateTemp,
		func(string, string) error { return wantRenameErr },
	)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "fallback", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	matches, err := filepath.Glob(filepath.Join(parent, ".mounted-config.yaml.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}
