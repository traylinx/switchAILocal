package switchailocal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/sdk/config"
)

func TestEnsureAuthDirCreatesAndMigratesPrivateDirectory(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name      string
		precreate bool
	}{
		{name: "fresh"},
		{name: "existing", precreate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authDir := filepath.Join(root, tc.name)
			if tc.precreate {
				require.NoError(t, os.Mkdir(authDir, 0o755))
				require.NoError(t, os.Chmod(authDir, 0o755))
			}
			service := &Service{cfg: &config.Config{AuthDir: authDir}}
			require.NoError(t, service.ensureAuthDir())
			info, err := os.Stat(authDir)
			require.NoError(t, err)
			assert.True(t, info.IsDir())
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		})
	}
}
