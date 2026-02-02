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

	// Create temp auth dir
	authDir, err := os.MkdirTemp("", "auth-files-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(authDir)

	// Create a valid auth file
	validFile := filepath.Join(authDir, "valid.json")
	err = os.WriteFile(validFile, []byte(`{"type":"test"}`), 0644)
	require.NoError(t, err)

	// Create a file OUTSIDE auth dir (sibling of authDir)
	parentDir := filepath.Dir(authDir)
	secretFile := filepath.Join(parentDir, "secret.json")
	err = os.WriteFile(secretFile, []byte(`{"secret":"pwned"}`), 0644)
	require.NoError(t, err)
	defer os.Remove(secretFile)

	// Initialize Handler
	cfg := &config.Config{
		AuthDir: authDir,
	}
	h := NewHandler(cfg, "config.yaml", nil)

	tests := []struct {
		name           string
		queryName      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Valid File",
			queryName:      "valid.json",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"type":"test"}`,
		},
		{
			name:           "Traversal with ..",
			queryName:      "../secret.json",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `invalid name`,
		},
		{
			name:           "Traversal with /",
			queryName:      "/etc/passwd.json", // Note: .json suffix required
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `invalid name`,
		},
		{
			name:           "Traversal with backslash",
			queryName:      "..\\secret.json",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `invalid name`,
		},
		{
			name:           "Nested file (should be rejected)",
			queryName:      "subdir/valid.json",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `invalid name`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/download", h.DownloadAuthFile)

			req, _ := http.NewRequest("GET", "/download?name="+tc.queryName, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
			if tc.expectedBody != "" {
				assert.Contains(t, w.Body.String(), tc.expectedBody)
			}
		})
	}
}
