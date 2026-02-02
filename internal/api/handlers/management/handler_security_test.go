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
