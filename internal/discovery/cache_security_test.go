package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm(), "%s mode", path)
}

func TestNewCacheCreatesAndMigratesOnlyTheCacheLeafToPrivateMode(t *testing.T) {
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o755))
	freshCacheDir := filepath.Join(parent, "fresh-discovery")
	_, err := NewCache(freshCacheDir)
	require.NoError(t, err)
	requireMode(t, freshCacheDir, 0o700)

	cacheDir := filepath.Join(parent, "discovery")
	require.NoError(t, os.Mkdir(cacheDir, 0o755))
	require.NoError(t, os.Chmod(cacheDir, 0o755))

	_, err = NewCache(cacheDir)
	require.NoError(t, err)
	requireMode(t, cacheDir, 0o700)
	requireMode(t, parent, 0o755)
}

func TestCacheWriteCreatesAndMigratesPrivateFiles(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "discovery")
	cache, err := NewCache(cacheDir)
	require.NoError(t, err)
	entry := &CacheEntry{
		ProviderID: "provider",
		FetchedAt:  time.Now(),
		TTLSeconds: 60,
		SourceURL:  "https://example.com/models?token=private",
		SourceType: "api",
	}
	path := filepath.Join(cacheDir, "provider.json")

	require.NoError(t, cache.Set(entry))
	requireMode(t, path, 0o600)

	for _, oldMode := range []os.FileMode{0o644, 0o666} {
		require.NoError(t, os.Chmod(path, oldMode))
		require.NoError(t, cache.Set(entry))
		requireMode(t, path, 0o600)
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var stored CacheEntry
	require.NoError(t, json.Unmarshal(data, &stored))
	assert.Equal(t, entry.SourceURL, stored.SourceURL)
}
