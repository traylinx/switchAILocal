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
	"strings"
	"testing"
)

func TestSecurity_ArbitraryBinaryExecutionBlocked(t *testing.T) {
	// 1. Create a malicious script disguised as a whitelisted binary
	tmpDir, err := os.MkdirTemp("", "exploit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	maliciousBinary := filepath.Join(tmpDir, "gemini") // "gemini" is whitelisted
	scriptContent := "#!/bin/sh\necho PWNED"
	if err := os.WriteFile(maliciousBinary, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Prepare the request pointing to the malicious binary using absolute path
	req := ExecuteRequest{
		Binary: maliciousBinary,
		Args:   []string{},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer test-secret")
	httpReq.Header.Set("Content-Type", "application/json")

	// 3. Setup environment
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	// Ensure "gemini" is NOT in PATH so LookPath fails and code *would* fall back to our path if vulnerable
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", "") // clear PATH to force fallback logic
	defer os.Setenv("PATH", originalPath)

	// 4. Run the handler
	w := httptest.NewRecorder()
	handleRun(w, httpReq)

	// 5. Assertions for SAFE behavior
	// The request should be rejected because "gemini" is not in PATH or common trusted paths.
	if w.Code != http.StatusBadRequest {
		t.Errorf("SECURITY FAILURE: Expected status 400 Bad Request, got %d. The malicious binary was likely executed.", w.Code)
	}

	// Check if execution output indicates success (which is bad)
	var result ExecuteResponse
	// If status is 200, we can decode response
	if w.Code == http.StatusOK {
		if err := json.NewDecoder(w.Body).Decode(&result); err == nil {
			if strings.TrimSpace(result.Stdout) == "PWNED" {
				t.Error("SECURITY CRITICAL: Arbitrary binary execution confirmed! The malicious script was executed.")
			}
		}
	} else {
		// Expected error message
		if !strings.Contains(w.Body.String(), "Binary not found") && !strings.Contains(w.Body.String(), "Forbidden") {
			// It might return "Binary not in whitelist" if my logic is wrong, but here base is "gemini" so whitelist passes.
			// It should fail on path resolution.
		}
	}
}
