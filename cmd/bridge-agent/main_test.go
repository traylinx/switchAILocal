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
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestDefaultLocalhostBinding(t *testing.T) {
	// Save original environment
	originalPort := os.Getenv("BRIDGE_PORT")
	defer func() {
		if originalPort != "" {
			os.Setenv("BRIDGE_PORT", originalPort)
		} else {
			os.Unsetenv("BRIDGE_PORT")
		}
	}()

	// Test default port
	os.Unsetenv("BRIDGE_PORT")
	// We can't easily test the actual binding without starting a server,
	// but we can verify the logic by checking the address construction
	// This would be tested in integration tests
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	secret := "test-secret"
	handler := authMiddleware(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/run", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	if !strings.Contains(w.Body.String(), "Unauthorized") {
		t.Errorf("Expected 'Unauthorized' in response body, got: %s", w.Body.String())
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	secret := "test-secret"
	handler := authMiddleware(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/run", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	if !strings.Contains(w.Body.String(), "Forbidden") {
		t.Errorf("Expected 'Forbidden' in response body, got: %s", w.Body.String())
	}
}

func TestAuthMiddleware_InvalidBearerFormat(t *testing.T) {
	secret := "test-secret"
	handler := authMiddleware(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/run", nil)
	req.Header.Set("Authorization", "InvalidFormat token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret"
	handler := authMiddleware(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/run", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHandleRun_NonWhitelistedBinary(t *testing.T) {
	// Set up environment
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	req := ExecuteRequest{
		Binary: "malicious-binary",
		Args:   []string{"arg1"},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer test-secret")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handleRun(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if !strings.Contains(w.Body.String(), "Binary not in whitelist") {
		t.Errorf("Expected 'Binary not in whitelist' in response, got: %s", w.Body.String())
	}
}

func TestHandleRun_WhitelistedBinary(t *testing.T) {
	// Set up environment
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	req := ExecuteRequest{
		Binary: "gemini",
		Args:   []string{"--help"},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer test-secret")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handleRun(w, httpReq)

	// Note: This will likely fail because 'gemini' binary doesn't exist,
	// but it should pass the whitelist check and fail later in execution
	// The important thing is it doesn't return 400 for whitelist violation
	if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "Binary not in whitelist") {
		t.Errorf("Whitelisted binary 'gemini' was rejected by whitelist")
	}
}

func TestHandleRun_DangerousArguments(t *testing.T) {
	// Set up environment
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	dangerousArgs := [][]string{
		{"arg1", "arg2;rm -rf /"},
		{"arg1", "arg2|cat /etc/passwd"},
		{"arg1", "arg2&background-command"},
		{"arg1", "arg2`whoami`"},
		{"arg1", "arg2$HOME"},
		{"arg1", "arg2<input.txt"},
		{"arg1", "arg2>output.txt"},
	}

	for _, args := range dangerousArgs {
		req := ExecuteRequest{
			Binary: "gemini",
			Args:   args,
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
		httpReq.Header.Set("Authorization", "Bearer test-secret")
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handleRun(w, httpReq)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for dangerous args %v, got %d", http.StatusBadRequest, args, w.Code)
		}

		if !strings.Contains(w.Body.String(), "Dangerous characters") {
			t.Errorf("Expected 'Dangerous characters' in response for args %v, got: %s", args, w.Body.String())
		}
	}
}

func TestHandleRun_SafeArguments(t *testing.T) {
	// Set up environment
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	req := ExecuteRequest{
		Binary: "gemini",
		Args:   []string{"--help", "--version", "normal-arg"},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer test-secret")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handleRun(w, httpReq)

	// Should not be rejected for dangerous characters
	if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "Dangerous characters") {
		t.Errorf("Safe arguments were rejected as dangerous: %s", w.Body.String())
	}
}

func TestHandleRun_MethodNotAllowed(t *testing.T) {
	httpReq := httptest.NewRequest("GET", "/run", nil)
	w := httptest.NewRecorder()

	handleRun(w, httpReq)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleRun_InvalidJSON(t *testing.T) {
	httpReq := httptest.NewRequest("POST", "/run", strings.NewReader("invalid json"))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleRun(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// Property 1: Authorization header enforcement
// **Validates: Requirements 1.3**
func TestProperty_AuthorizationHeaderEnforcement(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("requests without Authorization header return 401", prop.ForAll(
		func(requestBody []byte) bool {
			secret := "test-secret"
			handler := authMiddleware(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("POST", "/run", bytes.NewReader(requestBody))
			// Deliberately not setting Authorization header
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			return w.Code == http.StatusUnauthorized
		},
		gen.SliceOf(gen.UInt8()),
	))

	properties.TestingRun(t)
}

// Property 2: Invalid token rejection
// **Validates: Requirements 1.4**
func TestProperty_InvalidTokenRejection(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("requests with invalid tokens return 403", prop.ForAll(
		func(invalidToken string) bool {
			secret := "correct-secret"
			// Ensure the generated token is different from the correct secret
			if invalidToken == secret {
				return true // Skip this test case
			}

			handler := authMiddleware(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("POST", "/run", nil)
			req.Header.Set("Authorization", "Bearer "+invalidToken)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			return w.Code == http.StatusForbidden
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// Property 3: Binary whitelist enforcement
// **Validates: Requirements 1.7**
func TestProperty_BinaryWhitelistEnforcement(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Set up environment for tests
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	allowedBinaries := map[string]bool{
		"gemini": true,
		"claude": true,
		"vibe":   true,
		"codex":  true,
	}

	properties.Property("non-whitelisted binaries return 400", prop.ForAll(
		func(binaryName string) bool {
			// Skip if the generated binary name is in the whitelist
			if allowedBinaries[binaryName] {
				return true // Skip this test case
			}

			req := ExecuteRequest{
				Binary: binaryName,
				Args:   []string{"arg1"},
			}

			body, _ := json.Marshal(req)
			httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
			httpReq.Header.Set("Authorization", "Bearer test-secret")
			httpReq.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handleRun(w, httpReq)

			return w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "Binary not in whitelist")
		},
		gen.AlphaString().SuchThat(func(s string) bool {
			// Only test with non-whitelisted binaries
			return s != "" && !allowedBinaries[s]
		}),
	))

	properties.TestingRun(t)
}

// Property 4: Argument sanitization
// **Validates: Requirements 1.8, 1.9**
func TestProperty_ArgumentSanitization(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Set up environment for tests
	os.Setenv("BRIDGE_AGENT_SECRET", "test-secret")
	defer os.Unsetenv("BRIDGE_AGENT_SECRET")

	dangerousChars := []string{";", "|", "&", "`", "$", "<", ">"}

	properties.Property("arguments with dangerous characters return 400", prop.ForAll(
		func(baseArg string, dangerousChar string) bool {
			// Create an argument that contains a dangerous character
			dangerousArg := baseArg + dangerousChar + "malicious"

			req := ExecuteRequest{
				Binary: "gemini", // Use whitelisted binary
				Args:   []string{dangerousArg},
			}

			body, _ := json.Marshal(req)
			httpReq := httptest.NewRequest("POST", "/run", bytes.NewReader(body))
			httpReq.Header.Set("Authorization", "Bearer test-secret")
			httpReq.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handleRun(w, httpReq)

			return w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "Dangerous characters")
		},
		gen.AlphaString(),
		gen.OneConstOf(dangerousChars[0], dangerousChars[1], dangerousChars[2], dangerousChars[3], dangerousChars[4], dangerousChars[5], dangerousChars[6]),
	))

	properties.TestingRun(t)
}
