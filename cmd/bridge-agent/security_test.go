package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVulnerability_PathExecution(t *testing.T) {
	// 1. Setup a "malicious" binary in a temp directory
	tmpDir, err := os.MkdirTemp("", "bridge_exploit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// We use 'codex' as it is whitelisted but likely not installed in system paths
	maliciousBin := filepath.Join(tmpDir, "codex")
	content := []byte("#!/bin/sh\necho pwned")
	if err := os.WriteFile(maliciousBin, content, 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Setup request pointing to the malicious binary
	req := ExecuteRequest{
		Binary: maliciousBin,
		Args:   []string{},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	// Mock auth
	os.Setenv("BRIDGE_AGENT_SECRET", "secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")
	httpReq.Header.Set("Authorization", "Bearer secret")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// 3. Execute handler
	handleRun(w, httpReq)

	// 4. Check if it executed
	respBody := w.Body.String()

	// Expect failure (404 Not Found)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	if !strings.Contains(respBody, "Binary not found in trusted locations") {
		t.Errorf("Expected error message about missing binary, got: %s", respBody)
	}

	if strings.Contains(respBody, "pwned") {
		t.Error("Vulnerability STILL EXIST: Arbitrary binary executed via absolute path")
	}
}

func TestValidExecution_InPath(t *testing.T) {
	// 1. Setup a valid binary in a temp directory
	tmpDir, err := os.MkdirTemp("", "bridge_valid")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy 'gemini' binary that is safe
	validBin := filepath.Join(tmpDir, "gemini")
	// Use a simple script that outputs valid JSON to mimic a real tool response if needed,
	// or just something simple. The handler captures stdout.
	content := []byte("#!/bin/sh\necho '{\"response\":\"valid\"}'")
	if err := os.WriteFile(validBin, content, 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Add tmpDir to PATH so exec.LookPath finds it
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)
	defer os.Setenv("PATH", originalPath)

	// 3. Request just by name (or even by path, but it should resolve via LookPath)
	req := ExecuteRequest{
		Binary: "gemini",
		Args:   []string{},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	os.Setenv("BRIDGE_AGENT_SECRET", "secret")
	// defer unsetting is handled in previous test or we just overwrite it
	httpReq.Header.Set("Authorization", "Bearer secret")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// 4. Execute handler
	handleRun(w, httpReq)

	// 5. Check success
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	respBody := w.Body.String()
	// The handler wraps stdout in JSON: {"stdout": "...", ...}
	// Our script outputs '{"response":"valid"}' followed by newline.
	// So we expect stdout to contain that.
	if !strings.Contains(respBody, "valid") {
		t.Errorf("Expected execution output 'valid', got: %s", respBody)
	}
}
