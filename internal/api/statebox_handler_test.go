// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/util"
)

func TestStateBoxStatusHandler_Success(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Set environment variables
	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	os.Setenv("SWITCHAI_READONLY", "0")
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	defer os.Unsetenv("SWITCHAI_READONLY")
	
	// Create StateBox
	sb, err := util.NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}
	
	// Create discovery and intelligence directories
	discoveryDir := filepath.Join(tempDir, "discovery")
	intelligenceDir := filepath.Join(tempDir, "intelligence")
	if err := os.MkdirAll(discoveryDir, 0700); err != nil {
		t.Fatalf("Failed to create discovery directory: %v", err)
	}
	if err := os.MkdirAll(intelligenceDir, 0700); err != nil {
		t.Fatalf("Failed to create intelligence directory: %v", err)
	}
	
	// Create test files
	registryPath := filepath.Join(discoveryDir, "registry.json")
	if err := os.WriteFile(registryPath, []byte(`{"test": "data"}`), 0600); err != nil {
		t.Fatalf("Failed to create registry file: %v", err)
	}
	
	dbPath := filepath.Join(intelligenceDir, "feedback.db")
	if err := os.WriteFile(dbPath, []byte("test db"), 0600); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}
	
	// Setup Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/state-box/status", StateBoxStatusHandler(sb))
	
	// Create request
	req, err := http.NewRequest("GET", "/api/state-box/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Record response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	// Parse response
	var status StateBoxStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	// Verify response
	if status.RootPath != tempDir {
		t.Errorf("Expected root path %s, got %s", tempDir, status.RootPath)
	}
	
	if status.ReadOnly {
		t.Error("Expected read-only to be false")
	}
	
	if !status.Initialized {
		t.Error("Expected initialized to be true")
	}
	
	if status.DiscoveryRegistry == nil {
		t.Fatal("Expected discovery registry status to be present")
	}
	
	if !status.DiscoveryRegistry.Exists {
		t.Error("Expected discovery registry to exist")
	}
	
	if status.FeedbackDatabase == nil {
		t.Fatal("Expected feedback database status to be present")
	}
	
	if !status.FeedbackDatabase.Exists {
		t.Error("Expected feedback database to exist")
	}
	
	if status.PermissionStatus != "ok" {
		t.Errorf("Expected permission status 'ok', got '%s'", status.PermissionStatus)
	}
}

func TestStateBoxStatusHandler_ReadOnlyMode(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Set environment variables for read-only mode
	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	os.Setenv("SWITCHAI_READONLY", "1")
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	defer os.Unsetenv("SWITCHAI_READONLY")
	
	// Create StateBox
	sb, err := util.NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}
	
	// Setup Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/state-box/status", StateBoxStatusHandler(sb))
	
	// Create request
	req, err := http.NewRequest("GET", "/api/state-box/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Record response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	// Parse response
	var status StateBoxStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	// Verify read-only mode
	if !status.ReadOnly {
		t.Error("Expected read-only to be true")
	}
}

func TestStateBoxStatusHandler_NilStateBox(t *testing.T) {
	// Setup Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/state-box/status", StateBoxStatusHandler(nil))
	
	// Create request
	req, err := http.NewRequest("GET", "/api/state-box/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Record response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	
	// Check status code
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}
