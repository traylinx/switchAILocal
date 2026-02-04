package management

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
)

func TestInitializeSecret_Security(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
			name:       "Spoofed Localhost via X-Forwarded-For",
			remoteAddr: "127.0.0.1:1234",
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4",
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temp config file for each test
			content := []byte(`
remote-management:
  secret-key: ""
`)
			tmpFile, err := os.CreateTemp("", "config-init-*.yaml")
			require.NoError(t, err)
			defer os.Remove(tmpFile.Name())
			_, err = tmpFile.Write(content)
			require.NoError(t, err)
			tmpFile.Close()

			// Initialize Handler with empty secret for EACH test
			cfg := &config.Config{
				RemoteManagement: config.RemoteManagement{
					SecretKey: "",
				},
			}
			h := NewHandler(cfg, tmpFile.Name(), nil)

			// Setup Router
			r := gin.New()
			r.POST("/initialize", h.InitializeSecret)

			// Prepare request body
			body := bytes.NewBufferString(`{"secret": "newpassword"}`)

			req, _ := http.NewRequest("POST", "/initialize", body)
			req.Header.Set("Content-Type", "application/json")
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
