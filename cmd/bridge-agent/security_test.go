// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSecurity_ArbitraryExecutionBlocked(t *testing.T) {
	// 1. Create a temp directory
	tempDir, err := os.MkdirTemp("", "bridge-agent-security-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 2. Create a mock executable named 'gemini' (whitelisted name)
	scriptName := "gemini"
	if runtime.GOOS == "windows" {
		scriptName += ".bat"
	}
	scriptPath := filepath.Join(tempDir, scriptName)

	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptContent = "@echo off\necho PWNED"
	} else {
		scriptContent = "#!/bin/sh\necho PWNED"
	}

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write mock script: %v", err)
	}

	// 3. Prepare request to execute this script
	req := ExecuteRequest{
		Binary: scriptPath,
		Args:   []string{},
	}
	body, _ := json.Marshal(req)

	// 4. Send request with cleared PATH to force LookPath failure
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)
	os.Setenv("PATH", "")

	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	// Setup auth
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")
	httpReq.Header.Set("Authorization", "Bearer test-secret")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handleRun(w, httpReq)

	// 5. Analyze response
	// Expect 400 Bad Request
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d", w.Code)
	}

	expectedError := "Forbidden: Binary not found in trusted locations"
	if !strings.Contains(w.Body.String(), expectedError) {
		t.Errorf("Expected error message containing %q, got %q", expectedError, w.Body.String())
	}

	// Double check stdout/stderr just in case (should be empty or error)
	// But since it's likely plain text error from http.Error, we can't parse JSON.
	// If it was JSON, we'd check stdout.
	// The http.Error writes plain text.
}
