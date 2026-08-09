package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigWritersMigratePrivateFileModes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial string
		write   func(string) error
	}{
		{
			name:    "preserve comments",
			initial: "# operator comment\nport: 18080\n",
			write: func(path string) error {
				return SaveConfigPreserveComments(path, &Config{Port: 18081})
			},
		},
		{
			name:    "nested scalar",
			initial: "remote-management:\n  secret-key: old\n",
			write: func(path string) error {
				return SaveConfigPreserveCommentsUpdateNestedScalar(path, []string{"remote-management", "secret-key"}, "private")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.initial), 0o644))
			for _, oldMode := range []os.FileMode{0o644, 0o666} {
				require.NoError(t, os.Chmod(path, oldMode))
				require.NoError(t, tc.write(path))
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
			anchored := filepath.Join(filepath.Dir(path), "anchored.yaml")
			require.NoError(t, os.Link(path, anchored))
			require.NoError(t, tc.write(anchored))
			targetInfo, err := os.Stat(path)
			require.NoError(t, err)
			anchoredInfo, err := os.Stat(anchored)
			require.NoError(t, err)
			assert.True(t, os.SameFile(targetInfo, anchoredInfo))
		})
	}
}
