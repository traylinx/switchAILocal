package auth_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/auth/claude"
	"github.com/traylinx/switchAILocal/internal/auth/codex"
	"github.com/traylinx/switchAILocal/internal/auth/gemini"
	"github.com/traylinx/switchAILocal/internal/auth/iflow"
	"github.com/traylinx/switchAILocal/internal/auth/qwen"
	"github.com/traylinx/switchAILocal/internal/auth/vertex"
)

type credentialSaver interface {
	SaveTokenToFile(string) error
}

func TestLegacyCredentialWritersCreateAndMigratePrivateFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		saver  credentialSaver
		indent bool
	}{
		{name: "claude", saver: &claude.ClaudeTokenStorage{AccessToken: "private"}},
		{name: "codex", saver: &codex.CodexTokenStorage{AccessToken: "private"}},
		{name: "gemini", saver: &gemini.GeminiTokenStorage{Token: map[string]any{"access_token": "private"}}},
		{name: "iflow", saver: &iflow.IFlowTokenStorage{AccessToken: "private"}},
		{name: "qwen", saver: &qwen.QwenTokenStorage{AccessToken: "private"}},
		{name: "vertex", saver: &vertex.VertexCredentialStorage{ServiceAccount: map[string]any{"private_key": "private"}}, indent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", tc.name+".json")
			require.NoError(t, tc.saver.SaveTokenToFile(path))
			parentInfo, err := os.Stat(filepath.Dir(path))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o700), parentInfo.Mode().Perm())

			for _, oldMode := range []os.FileMode{0o644, 0o666} {
				require.NoError(t, os.Chmod(path, oldMode))
				require.NoError(t, tc.saver.SaveTokenToFile(path))
				info, statErr := os.Stat(path)
				require.NoError(t, statErr)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var expected bytes.Buffer
			encoder := json.NewEncoder(&expected)
			if tc.indent {
				encoder.SetIndent("", "  ")
			}
			require.NoError(t, encoder.Encode(tc.saver))
			assert.Equal(t, expected.Bytes(), data, "legacy encoder bytes changed")
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tc.name, decoded["type"])
		})
	}
}

func TestLegacyCredentialWriterPreservesAnchoredTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"old":true}`), 0o600))
	anchored := filepath.Join(filepath.Dir(target), "anchored.json")
	require.NoError(t, os.Link(target, anchored))
	saver := &claude.ClaudeTokenStorage{AccessToken: "private"}
	require.NoError(t, saver.SaveTokenToFile(anchored))
	targetInfo, err := os.Stat(target)
	require.NoError(t, err)
	anchoredInfo, err := os.Stat(anchored)
	require.NoError(t, err)
	assert.True(t, os.SameFile(targetInfo, anchoredInfo))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Contains(t, string(data), "private")
}
