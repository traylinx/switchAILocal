package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
)

func TestResetSecret_Security(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a temp config file with minimal content
	content := []byte(`
remote-management:
  secret-key: somehash
`)
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.Write(content)
	require.NoError(t, err)
	tmpFile.Close()

	// Initialize Handler
	cfg := &config.Config{
		RemoteManagement: config.RemoteManagement{
			SecretKey: "somehash",
		},
	}
	h := NewHandler(cfg, tmpFile.Name(), nil)

	tests := []struct {
		name           string
		remoteAddr     string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name:           "Direct Localhost",
			remoteAddr:     "127.0.0.1:1234",
			headers:        nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Remote IP",
			remoteAddr:     "192.168.1.50:1234",
			headers:        nil,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:       "Spoofed Localhost via X-Forwarded-For (Proxy Bypass)",
			remoteAddr: "127.0.0.1:1234", // App behind proxy
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4",
			},
			expectedStatus: http.StatusForbidden, // Should be Forbidden if we check headers
		},
		{
			name:       "Spoofed Localhost via X-Real-IP (Proxy Bypass)",
			remoteAddr: "127.0.0.1:1234",
			headers: map[string]string{
				"X-Real-IP": "1.2.3.4",
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:       "Spoofed Localhost via Forwarded (Proxy Bypass)",
			remoteAddr: "127.0.0.1:1234",
			headers: map[string]string{
				"Forwarded": "for=1.2.3.4",
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:       "Spoof Attempt: External User claiming Localhost",
			remoteAddr: "1.2.3.4:5678",
			headers: map[string]string{
				"X-Forwarded-For": "127.0.0.1",
			},
			expectedStatus: http.StatusForbidden, // Must NOT be fooled
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup Router
			r := gin.New()
			r.POST("/reset", h.ResetSecret)

			req, _ := http.NewRequest("POST", "/reset", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Logf("Response Body: %s", w.Body.String())
			}
			assert.Equal(t, tc.expectedStatus, w.Code)
		})
	}
}

// Resetting the secret must also drop allow-remote: an empty secret makes the
// middleware bypass auth, so keeping remote access enabled would fail-open the
// management API to any remote host (and the persisted config would then be
// rejected by the fail-closed startup guard).
func TestResetSecret_DisablesRemoteAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.WriteString("remote-management:\n  secret-key: somehash\n  allow-remote: true\n")
	require.NoError(t, err)
	tmpFile.Close()

	cfg := &config.Config{
		RemoteManagement: config.RemoteManagement{
			SecretKey:   "somehash",
			AllowRemote: true,
		},
	}
	h := NewHandler(cfg, tmpFile.Name(), nil)

	r := gin.New()
	r.POST("/reset", h.ResetSecret)
	req, _ := http.NewRequest("POST", "/reset", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, cfg.RemoteManagement.SecretKey, "secret must be cleared")
	assert.False(t, cfg.RemoteManagement.AllowRemote, "allow-remote must be dropped with the secret")
}
