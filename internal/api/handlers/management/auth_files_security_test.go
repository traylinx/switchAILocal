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
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// MockStore implements auth.Store for testing
type MockStore struct{}

func (m *MockStore) List(ctx context.Context) ([]*auth.Auth, error) { return nil, nil }
func (m *MockStore) Save(ctx context.Context, a *auth.Auth) (string, error) { return "", nil }
func (m *MockStore) Delete(ctx context.Context, id string) error { return nil }

func TestDownloadAuthFile_PathTraversal(t *testing.T) {
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
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Path Traversal with Backslash (Windows style)",
			queryName:      "..\\secrets\\secret.json",
			expectedStatus: http.StatusBadRequest,
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

	tmpRoot, err := os.MkdirTemp("", "test-root-upload-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpRoot)

	authDir := filepath.Join(tmpRoot, "auths")
	err = os.Mkdir(authDir, 0755)
	require.NoError(t, err)

	cfg := &config.Config{
		AuthDir: authDir,
	}

	// Initialize with a mock auth manager to bypass the nil check
	authManager := auth.NewManager(&MockStore{}, nil, nil)
	h := NewHandler(cfg, "", authManager)

	r := gin.New()
	r.POST("/upload", h.UploadAuthFile)

	t.Run("Raw Body Path Traversal", func(t *testing.T) {
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
				name:           "Path Traversal Forward Slash",
				queryName:      "../evil.json",
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Path Traversal Backslash",
				queryName:      "..\\evil.json",
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				body := []byte(`{"foo":"bar"}`)
				req, _ := http.NewRequest("POST", "/upload?name="+tc.queryName, bytes.NewBuffer(body))
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				assert.Equal(t, tc.expectedStatus, w.Code)
			})
		}
	})

	t.Run("Multipart Path Traversal", func(t *testing.T) {
		tests := []struct {
			name           string
			filename       string
			expectedStatus int
		}{
			{
				name:           "Valid File",
				filename:       "valid.json",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "Path Traversal Backslash",
				filename:       "..\\evil.json",
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				part, err := writer.CreateFormFile("file", tc.filename)
				require.NoError(t, err)
				_, err = part.Write([]byte(`{"foo":"bar"}`))
				require.NoError(t, err)
				err = writer.Close()
				require.NoError(t, err)

				req, _ := http.NewRequest("POST", "/upload", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				assert.Equal(t, tc.expectedStatus, w.Code, "Filename: %s", tc.filename)
			})
		}
	})
}

func TestDeleteAuthFile_PathTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpRoot, err := os.MkdirTemp("", "test-root-delete-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpRoot)

	authDir := filepath.Join(tmpRoot, "auths")
	err = os.Mkdir(authDir, 0755)
	require.NoError(t, err)

	// Create a dummy file to delete
	err = os.WriteFile(filepath.Join(authDir, "dummy.json"), []byte("{}"), 0644)
	require.NoError(t, err)

	cfg := &config.Config{
		AuthDir: authDir,
	}

	// Initialize with a mock auth manager
	authManager := auth.NewManager(&MockStore{}, nil, nil)
	h := NewHandler(cfg, "", authManager)

	r := gin.New()
	r.DELETE("/delete", h.DeleteAuthFile)

	tests := []struct {
		name           string
		queryName      string
		expectedStatus int
	}{
		{
			name:           "Valid File",
			queryName:      "dummy.json",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Path Traversal Forward Slash",
			queryName:      "../outside.json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Path Traversal Backslash",
			queryName:      "..\\outside.json",
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
