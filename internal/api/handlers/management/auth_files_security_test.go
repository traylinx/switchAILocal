package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
)

func TestDownloadAuthFile_PathTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup directory structure
	// /tmp/test-root/
	//   auths/ (AuthDir)
	//   secrets/
	//     secret.json

	tmpRoot, err := os.MkdirTemp("", "test-root-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpRoot)

	authDir := filepath.Join(tmpRoot, "auths")
	err = os.Mkdir(authDir, 0755)
	require.NoError(t, err)

	secretsDir := filepath.Join(tmpRoot, "secrets")
	err = os.Mkdir(secretsDir, 0755)
	require.NoError(t, err)

	secretFile := filepath.Join(secretsDir, "secret.json")
	err = os.WriteFile(secretFile, []byte(`{"secret": "very-sensitive"}`), 0644)
	require.NoError(t, err)

	// Valid file in auths
	validFile := filepath.Join(authDir, "valid.json")
	err = os.WriteFile(validFile, []byte(`{"ok": true}`), 0644)
	require.NoError(t, err)

	cfg := &config.Config{
		AuthDir: authDir,
	}
	h := NewHandler(cfg, "", nil)

	r := gin.New()
	r.GET("/download", h.DownloadAuthFile)

	tests := []struct {
		name           string
		queryName      string
		expectedStatus int
	}{
		{
			name:           "Valid File",
			queryName:      "valid.json",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Path Traversal Attempt",
			queryName:      "../secrets/secret.json",
			expectedStatus: http.StatusBadRequest, // Should be rejected
		},
		{
			name:           "Path Traversal with Backslash (Windows style)",
			queryName:      "..\\secrets\\secret.json",
			expectedStatus: http.StatusBadRequest, // Should be rejected
		},
		{
			name:           "Path Traversal Encoded",
			queryName:      "%2e%2e%2fsecrets%2fsecret.json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/download?name="+tc.queryName, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
		})
	}
}
