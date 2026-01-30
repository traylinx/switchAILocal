package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/logging"
	"github.com/traylinx/switchAILocal/internal/util"
)

func TestLogRequest_SensitiveBody(t *testing.T) {
	// Setup
	logsDir := t.TempDir()
	logger := logging.NewFileRequestLogger(true, logsDir, "")

	// Sensitive data
	apiKey := "sk-secret1234567890"
	password := "mySecretPass123"
	escapedVal := `val\"ue`
	jsonBody := []byte(`{
		"api_key": "` + apiKey + `",
		"password": "` + password + `",
		"token": "tok_` + escapedVal + `",
		"other": "value"
	}`)

	// Log request
	err := logger.LogRequest(
		"http://example.com/api",
		"POST",
		map[string][]string{"Content-Type": {"application/json"}},
		jsonBody,
		200,
		nil,
		[]byte("response"),
		nil,
		nil,
		nil,
		"test-req-id",
	)
	if err != nil {
		t.Fatalf("LogRequest failed: %v", err)
	}

	// Verify log file content
	files, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("Failed to read logs dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("No log file created")
	}

	logContent, err := os.ReadFile(filepath.Join(logsDir, files[0].Name()))
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(logContent)

	// Check API Key masking (partial)
	if strings.Contains(contentStr, apiKey) {
		t.Errorf("Security check failed: Plain text API key found in log.")
	}
	expectedMaskedKey := util.HideAPIKey(apiKey)
	if !strings.Contains(contentStr, expectedMaskedKey) {
		t.Errorf("Expected log to contain masked API key %q, but got:\n%s", expectedMaskedKey, contentStr)
	}

	// Check Password redaction (full)
	if strings.Contains(contentStr, password) {
		t.Errorf("Security check failed: Plain text password found in log.")
	}
	if !strings.Contains(contentStr, `"password": "******"`) {
		t.Errorf("Expected log to contain redacted password, but got:\n%s", contentStr)
	}

	// Check Escaped Quote handling
	if strings.Contains(contentStr, "tok_"+escapedVal) {
		t.Errorf("Security check failed: Plain text token with escaped quote found in log.")
	}
}
