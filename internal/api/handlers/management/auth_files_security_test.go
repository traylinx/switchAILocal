package management

import (
	"bytes"
	"context"
	"mime/multipart"
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

// mockStore implements coreauth.Store for testing
type mockStore struct{}

func (m *mockStore) List(ctx context.Context) ([]*coreauth.Auth, error) {
	return []*coreauth.Auth{}, nil
}

func (m *mockStore) Save(ctx context.Context, auth *coreauth.Auth) (string, error) {
	return auth.ID, nil
}

func (m *mockStore) Delete(ctx context.Context, id string) error {
	return nil
}

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
	// No manager needed for download
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

	// Upload requires authManager
	store := &mockStore{}
	manager := coreauth.NewManager(store, nil, nil)
	h := NewHandler(cfg, "", manager)

	r := gin.New()
	r.POST("/upload", h.UploadAuthFile)

	tests := []struct {
		name           string
		queryName      string
		body           string
		expectedStatus int
	}{
		{
			name:           "Valid File Upload via Body",
			queryName:      "valid.json",
			body:           `{"type":"test"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Path Traversal Upload",
			queryName:      "../secrets/evil.json",
			body:           `{"type":"evil"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Path Traversal Upload Backslash",
			queryName:      "..\\secrets\\evil.json",
			body:           `{"type":"evil"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/upload?name="+tc.queryName, bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
		})
	}
}

func TestUploadAuthFile_Multipart_PathTraversal(t *testing.T) {
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

	// Upload requires authManager
	store := &mockStore{}
	manager := coreauth.NewManager(store, nil, nil)
	h := NewHandler(cfg, "", manager)

	r := gin.New()
	r.POST("/upload", h.UploadAuthFile)

	// Multipart upload handling uses filepath.Base internally, so traversal should be stripped
	// resulting in a file named "evil.json" in authDir.
	// We verify that it works safely and creates the file in the correct place.

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "../secrets/evil.json")
	require.NoError(t, err)
	_, err = part.Write([]byte(`{"type":"evil"}`))
	require.NoError(t, err)
	writer.Close()

	req, _ := http.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// It should succeed but strip path components
	assert.Equal(t, http.StatusOK, w.Code)
	// Verify file exists in authDir
	_, err = os.Stat(filepath.Join(authDir, "evil.json"))
	assert.NoError(t, err)
}

func TestDeleteAuthFile_PathTraversal(t *testing.T) {
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

	// Delete requires authManager
	store := &mockStore{}
	manager := coreauth.NewManager(store, nil, nil)
	h := NewHandler(cfg, "", manager)

	r := gin.New()
	r.DELETE("/delete", h.DeleteAuthFile)

	tests := []struct {
		name           string
		queryName      string
		expectedStatus int
	}{
		{
			name:           "Path Traversal Delete",
			queryName:      "../secrets/secret.json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Path Traversal Delete Backslash",
			queryName:      "..\\secrets\\secret.json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("DELETE", "/delete?name="+tc.queryName, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
		})
	}
}
