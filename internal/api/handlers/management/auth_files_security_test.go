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

// MockStore implements coreauth.Store for testing
type MockStore struct{}

func (m *MockStore) List(ctx context.Context) ([]*coreauth.Auth, error) {
	return []*coreauth.Auth{}, nil
}

func (m *MockStore) Save(ctx context.Context, auth *coreauth.Auth) (string, error) {
	return "", nil
}

func (m *MockStore) Delete(ctx context.Context, id string) error {
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

func TestAuthFiles_PathTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup directory structure
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

	// Setup handler
	cfg := &config.Config{
		AuthDir: authDir,
	}

	mockStore := &MockStore{}
	authManager := coreauth.NewManager(mockStore, nil, nil)
	h := NewHandler(cfg, "", authManager)

	r := gin.New()
	r.POST("/upload", h.UploadAuthFile)
	r.DELETE("/delete", h.DeleteAuthFile)

	testCases := []struct {
		name      string
		endpoint  string
		method    string
		queryName string
		body      []byte // For upload
	}{
		{
			name:      "Upload Path Traversal ../",
			endpoint:  "/upload",
			method:    "POST",
			queryName: "../secrets/evil.json",
			body:      []byte(`{"key": "value"}`),
		},
		{
			name:      "Upload Path Traversal ..\\",
			endpoint:  "/upload",
			method:    "POST",
			queryName: "..\\secrets\\evil.json",
			body:      []byte(`{"key": "value"}`),
		},
		{
			name:      "Delete Path Traversal ../",
			endpoint:  "/delete",
			method:    "DELETE",
			queryName: "../secrets/secret.json",
		},
		{
			name:      "Delete Path Traversal ..\\",
			endpoint:  "/delete",
			method:    "DELETE",
			queryName: "..\\secrets\\secret.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := tc.endpoint + "?name=" + tc.queryName
			req, _ := http.NewRequest(tc.method, url, bytes.NewBuffer(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// We expect 400 Bad Request if the traversal is blocked.
			assert.Equal(t, http.StatusBadRequest, w.Code, "Expected 400 Bad Request for path traversal attempt")
		})
	}
}
