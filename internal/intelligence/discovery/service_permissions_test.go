package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/util"
)

func requirePrivateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm(), "%s mode", path)
}

func TestLegacyDiscoveryServiceMigratesDirectoryAndRegistryModes(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "discovery")
	require.NoError(t, os.Mkdir(cacheDir, 0o755))
	require.NoError(t, os.Chmod(cacheDir, 0o755))
	service, err := NewService(cacheDir, nil)
	require.NoError(t, err)
	requirePrivateMode(t, cacheDir, 0o700)

	registryPath := filepath.Join(cacheDir, "available_models.json")
	require.NoError(t, os.WriteFile(registryPath, []byte(`{"old":true}`), 0o644))
	for _, oldMode := range []os.FileMode{0o644, 0o666} {
		require.NoError(t, os.Chmod(registryPath, oldMode))
		require.NoError(t, service.WriteRegistry(""))
		requirePrivateMode(t, registryPath, 0o600)
	}

	data, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	var registry DiscoveryRegistry
	require.NoError(t, json.Unmarshal(data, &registry))
}

func TestStateBoxDiscoveryServiceMigratesRegistryMode(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SWITCHAI_STATE_DIR", root)
	t.Setenv("SWITCHAI_READONLY", "0")
	stateBox, err := util.NewStateBox()
	require.NoError(t, err)
	service, err := NewService("", stateBox)
	require.NoError(t, err)

	registryPath := filepath.Join(stateBox.DiscoveryDir(), "available_models.json")
	require.NoError(t, os.WriteFile(registryPath, []byte(`{"old":true}`), 0o644))
	require.NoError(t, os.Chmod(registryPath, 0o644))
	require.NoError(t, service.WriteRegistry(""))
	requirePrivateMode(t, registryPath, 0o600)
}

func TestWriteRegistryDoesNotChangeCustomParentMode(t *testing.T) {
	root := t.TempDir()
	service, err := NewService(filepath.Join(root, "service-cache"), nil)
	require.NoError(t, err)
	customParent := filepath.Join(root, "operator-owned")
	require.NoError(t, os.Mkdir(customParent, 0o755))
	require.NoError(t, os.Chmod(customParent, 0o755))
	registryPath := filepath.Join(customParent, "models.json")

	require.NoError(t, service.WriteRegistry(registryPath))
	requirePrivateMode(t, registryPath, 0o600)
	requirePrivateMode(t, customParent, 0o755)
}

func TestConcurrentRegistryWritesRemainCompleteAndPrivate(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "discovery")
	require.NoError(t, os.Mkdir(cacheDir, 0o700))
	registryPath := filepath.Join(cacheDir, "available_models.json")

	var wg sync.WaitGroup
	start := make(chan struct{})
	for writer := range 16 {
		models := make([]*DiscoveredModel, writer+1)
		for model := range models {
			models[model] = &DiscoveredModel{ID: fmt.Sprintf("writer-%02d-model-%02d", writer, model)}
		}
		service := &Service{
			models:    models,
			providers: []*ProviderStatus{{Provider: fmt.Sprintf("writer-%02d", writer), ModelCount: len(models)}},
			cacheDir:  cacheDir,
		}
		wg.Add(1)
		go func(service *Service) {
			defer wg.Done()
			<-start
			assert.NoError(t, service.WriteRegistry(""))
		}(service)
	}
	close(start)
	wg.Wait()

	data, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	var registry DiscoveryRegistry
	require.NoError(t, json.Unmarshal(data, &registry))
	require.NotEmpty(t, registry.Models)
	assert.Equal(t, len(registry.Models), registry.TotalModels)
	prefix := strings.SplitN(registry.Models[0].ID, "-model-", 2)[0]
	for model, entry := range registry.Models {
		assert.Equal(t, fmt.Sprintf("%s-model-%02d", prefix, model), entry.ID)
	}
	require.Len(t, registry.Providers, 1)
	assert.Equal(t, prefix, registry.Providers[0].Provider)
	assert.Equal(t, len(registry.Models), registry.Providers[0].ModelCount)
	requirePrivateMode(t, registryPath, 0o600)
}
