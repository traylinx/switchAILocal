package management

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

type invalidAuthLeafNameCase struct {
	name         string
	fileName     string
	absolute     bool
	expectedBody string
}

type recordingManagementAuthStore struct {
	baseDir   string
	deleteIDs []string
}

func (*recordingManagementAuthStore) List(context.Context) ([]*coreauth.Auth, error) {
	return nil, nil
}

func (*recordingManagementAuthStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}

func (store *recordingManagementAuthStore) Delete(_ context.Context, id string) error {
	store.deleteIDs = append(store.deleteIDs, id)
	return nil
}

func (store *recordingManagementAuthStore) SetBaseDir(baseDir string) {
	store.baseDir = baseDir
}

func invalidAuthLeafNameCases() []invalidAuthLeafNameCase {
	return []invalidAuthLeafNameCase{
		{name: "forward slash", fileName: "sub/secret.json", expectedBody: `{"error":"invalid name"}`},
		{name: "backslash", fileName: `sub\secret.json`, expectedBody: `{"error":"invalid name"}`},
		{name: "parent traversal", fileName: "../secret.json", expectedBody: `{"error":"invalid name"}`},
		{name: "nested parent traversal", fileName: "sub/../../secret.json", expectedBody: `{"error":"invalid name"}`},
		{name: "absolute path", absolute: true, expectedBody: `{"error":"invalid name"}`},
		{name: "dot", fileName: ".", expectedBody: `{"error":"invalid name"}`},
		{name: "dot dot", fileName: "..", expectedBody: `{"error":"invalid name"}`},
		{name: "empty", fileName: "", expectedBody: `{"error":"invalid name"}`},
		{name: "non JSON suffix", fileName: "secret.txt", expectedBody: `{"error":"name must end with .json"}`},
	}
}

func (tc invalidAuthLeafNameCase) resolvedFileName(testRoot string) string {
	if tc.absolute {
		return filepath.Join(testRoot, "outside", "secret.json")
	}
	return tc.fileName
}

func snapshotAuthTestTree(t *testing.T, root string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if info.Mode().IsRegular() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			value += "|" + string(data)
		}
		snapshot[rel] = value
		return nil
	}))
	return snapshot
}

// A nil-backed manager is intentional: these tests exercise handler validation,
// and coreauth.Manager treats a nil store as in-memory-only persistence.
func newAuthFileTestManager() *coreauth.Manager {
	return coreauth.NewManager(nil, nil, nil)
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

func TestUploadAuthFile_RejectsInvalidLeafNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range invalidAuthLeafNameCases() {
		t.Run(tc.name, func(t *testing.T) {
			testRoot := t.TempDir()
			authDir := filepath.Join(testRoot, "auth")
			require.NoError(t, os.Mkdir(authDir, 0o700))
			h := NewHandler(&config.Config{AuthDir: authDir}, "", newAuthFileTestManager())
			r := gin.New()
			r.POST("/upload", h.UploadAuthFile)

			fileName := tc.resolvedFileName(testRoot)
			before := snapshotAuthTestTree(t, testRoot)
			req := httptest.NewRequest(http.MethodPost, "/upload?name="+url.QueryEscape(fileName), strings.NewReader(`{}`))
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.JSONEq(t, tc.expectedBody, w.Body.String())
			assert.Equal(t, before, snapshotAuthTestTree(t, testRoot), "rejected upload must not mutate the test tree")
		})
	}
}

func TestUploadAuthFile_AcceptsValidLeafName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	h := NewHandler(&config.Config{AuthDir: authDir}, "", newAuthFileTestManager())
	r := gin.New()
	r.POST("/upload", h.UploadAuthFile)

	req := httptest.NewRequest(http.MethodPost, "/upload?name=good.json", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data, err := os.ReadFile(filepath.Join(authDir, "good.json"))
	require.NoError(t, err, "valid leaf name must reach the write path; response was %s", w.Body.String())
	assert.JSONEq(t, `{}`, string(data))
	info, err := os.Stat(filepath.Join(authDir, "good.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestDeleteAuthFile_RejectsInvalidLeafNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range invalidAuthLeafNameCases() {
		t.Run(tc.name, func(t *testing.T) {
			testRoot := t.TempDir()
			authDir := filepath.Join(testRoot, "auth")
			require.NoError(t, os.Mkdir(authDir, 0o700))
			h := NewHandler(&config.Config{AuthDir: authDir}, "", newAuthFileTestManager())
			r := gin.New()
			r.DELETE("/delete", h.DeleteAuthFile)

			fileName := tc.resolvedFileName(testRoot)
			if strings.HasSuffix(strings.ToLower(fileName), ".json") {
				canaryPath := fileName
				if !filepath.IsAbs(canaryPath) {
					canaryPath = filepath.Join(authDir, canaryPath)
				}
				canaryPath = filepath.Clean(canaryPath)
				rel, err := filepath.Rel(testRoot, canaryPath)
				require.NoError(t, err)
				require.False(t, rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)), "canary escaped test root")
				require.NoError(t, os.MkdirAll(filepath.Dir(canaryPath), 0o700))
				require.NoError(t, os.WriteFile(canaryPath, []byte(`{"canary":true}`), 0o600))
			}

			before := snapshotAuthTestTree(t, testRoot)
			req := httptest.NewRequest(http.MethodDelete, "/delete?name="+url.QueryEscape(fileName), nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.JSONEq(t, tc.expectedBody, w.Body.String())
			assert.Equal(t, before, snapshotAuthTestTree(t, testRoot), "rejected delete must not mutate the test tree")
		})
	}
}

func TestDeleteAuthFile_AcceptsValidLeafName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	filePath := filepath.Join(authDir, "good.json")
	require.NoError(t, os.WriteFile(filePath, []byte(`{"ok":true}`), 0o600))
	h := NewHandler(&config.Config{AuthDir: authDir}, "", newAuthFileTestManager())
	r := gin.New()
	r.DELETE("/delete", h.DeleteAuthFile)

	req := httptest.NewRequest(http.MethodDelete, "/delete?name=good.json", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
	_, err := os.Stat(filePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDeleteTokenRecordUsesCanonicalAuthID(t *testing.T) {
	authDir := t.TempDir()
	store := &recordingManagementAuthStore{}
	h := &Handler{
		cfg:        &config.Config{AuthDir: authDir},
		tokenStore: store,
	}

	absolutePath := filepath.Join(authDir, "provider", "tenant", "token.json")
	if err := h.deleteTokenRecord(context.Background(), absolutePath); err != nil {
		t.Fatal(err)
	}
	if err := h.deleteTokenRecord(context.Background(), "provider/second.json"); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, authDir, store.baseDir)
	assert.Equal(t, []string{"provider/tenant/token.json", "provider/second.json"}, store.deleteIDs)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	relativeAuthDir, err := filepath.Rel(cwd, authDir)
	require.NoError(t, err)
	relativeStore := &recordingManagementAuthStore{}
	relativeHandler := &Handler{
		cfg:        &config.Config{AuthDir: relativeAuthDir},
		tokenStore: relativeStore,
	}
	if err = relativeHandler.deleteTokenRecord(context.Background(), absolutePath); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []string{"provider/tenant/token.json"}, relativeStore.deleteIDs)

	outsidePath := filepath.Join(filepath.Dir(authDir), "outside.json")
	err = h.deleteTokenRecord(context.Background(), outsidePath)
	require.ErrorContains(t, err, "auth path is empty")
	assert.Equal(t, []string{"provider/tenant/token.json", "provider/second.json"}, store.deleteIDs)
}
