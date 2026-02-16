package management

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

type MockStore struct{}

func (m *MockStore) List(ctx context.Context) ([]*coreauth.Auth, error)     { return nil, nil }
func (m *MockStore) Save(ctx context.Context, auth *coreauth.Auth) (string, error) { return "", nil }
func (m *MockStore) Delete(ctx context.Context, id string) error            { return nil }

func TestUploadAuthFile_PathTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpRoot, err := os.MkdirTemp("", "test-root-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpRoot)

	authDir := filepath.Join(tmpRoot, "auths")
	err = os.Mkdir(authDir, 0755)
	require.NoError(t, err)

	cfg := &config.Config{
		AuthDir: authDir,
	}
	h := NewHandler(cfg, "", nil)

	// Setup Mock AuthManager
	mockStore := &MockStore{}
	authManager := coreauth.NewManager(mockStore, nil, nil)
	h.SetAuthManager(authManager)

	r := gin.New()
	r.POST("/upload", h.UploadAuthFile)

	tests := []struct {
		name           string
		queryName      string
		body           string
		expectedStatus int
	}{
		{
			name:           "Valid Upload",
			queryName:      "valid.json",
			body:           `{"valid": true}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Path Traversal Attempt",
			queryName:      "../secrets/secret.json",
			body:           `{"hacked": true}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Path Traversal with Backslash",
			queryName:      "..\\secrets\\secret.json",
			body:           `{"hacked": true}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/upload?name="+tc.queryName, bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "Status code mismatch for %s", tc.name)
		})
	}
}

func TestDeleteAuthFile_PathTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpRoot, err := os.MkdirTemp("", "test-root-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpRoot)

	authDir := filepath.Join(tmpRoot, "auths")
	err = os.Mkdir(authDir, 0755)
	require.NoError(t, err)

	// Create file to delete
	err = os.WriteFile(filepath.Join(authDir, "to_delete.json"), []byte("{}"), 0644)
	require.NoError(t, err)

	cfg := &config.Config{
		AuthDir: authDir,
	}
	h := NewHandler(cfg, "", nil)

	// Setup Mock AuthManager
	mockStore := &MockStore{}
	authManager := coreauth.NewManager(mockStore, nil, nil)
	h.SetAuthManager(authManager)

	r := gin.New()
	r.DELETE("/delete", h.DeleteAuthFile)

	tests := []struct {
		name           string
		queryName      string
		expectedStatus int
	}{
		{
			name:           "Path Traversal Attempt",
			queryName:      "../secrets/secret.json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Path Traversal with Backslash",
			queryName:      "..\\secrets\\secret.json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("DELETE", "/delete?name="+tc.queryName, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "Status code mismatch for %s", tc.name)
		})
	}
}
