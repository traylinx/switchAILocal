// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/intelligence"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// TestReloadSteering_Success tests successful steering rules reload.
func TestReloadSteering_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a real steering engine with temp directory
	tmpDir := t.TempDir()
	steeringEngine, err := steering.NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	// Create mock coordinator
	mockCoordinator := &mockServiceCoordinator{
		steering: steeringEngine,
	}

	// Create handler
	handler := &Handler{
		intelligenceService: mockCoordinator,
		serviceCoordinator:  mockCoordinator,
	}

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/steering/reload", nil)

	// Execute handler
	handler.ReloadSteering(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response["success"].(bool))
	assert.Equal(t, "Steering rules reloaded successfully", response["message"])
	// rules_count will be 0 since we didn't create any rule files
	assert.Equal(t, float64(0), response["rules_count"].(float64))
}

// TestReloadSteering_NotInitialized tests steering reload when service is not initialized.
func TestReloadSteering_NotInitialized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create handler with nil intelligence service
	handler := &Handler{
		intelligenceService: nil,
		serviceCoordinator:  nil,
	}

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/steering/reload", nil)

	// Execute handler
	handler.ReloadSteering(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["enabled"].(bool))
	assert.Equal(t, "Steering engine is not initialized", response["message"])
}

// TestReloadSteering_SteeringNil tests steering reload when steering engine is nil.
func TestReloadSteering_SteeringNil(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create mock coordinator with nil steering
	mockCoordinator := &mockServiceCoordinator{
		steering: nil,
	}

	// Create handler
	handler := &Handler{
		intelligenceService: mockCoordinator,
		serviceCoordinator:  mockCoordinator,
	}

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/steering/reload", nil)

	// Execute handler
	handler.ReloadSteering(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["enabled"].(bool))
	assert.Equal(t, "Steering engine is not initialized", response["message"])
}

// TestReloadHooks_Success tests successful hooks reload.
func TestReloadHooks_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a real hooks manager with temp directory
	tmpDir := t.TempDir()
	eventBus := hooks.NewEventBus()
	hooksManager, err := hooks.NewHookManager(tmpDir, eventBus)
	require.NoError(t, err)

	// Create mock coordinator
	mockCoordinator := &mockServiceCoordinator{
		hooks: hooksManager,
	}

	// Create handler
	handler := &Handler{
		intelligenceService: mockCoordinator,
		serviceCoordinator:  mockCoordinator,
	}

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/hooks/reload", nil)

	// Execute handler
	handler.ReloadHooks(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response["success"].(bool))
	assert.Equal(t, "Hooks reloaded successfully", response["message"])
	// hooks_count will be 0 since we didn't create any hook files
	assert.Equal(t, float64(0), response["hooks_count"].(float64))
}

// TestReloadHooks_NotInitialized tests hooks reload when service is not initialized.
func TestReloadHooks_NotInitialized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create handler with nil intelligence service
	handler := &Handler{
		intelligenceService: nil,
		serviceCoordinator:  nil,
	}

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/hooks/reload", nil)

	// Execute handler
	handler.ReloadHooks(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["enabled"].(bool))
	assert.Equal(t, "Hooks manager is not initialized", response["message"])
}

// TestReloadHooks_HooksNil tests hooks reload when hooks manager is nil.
func TestReloadHooks_HooksNil(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create mock coordinator with nil hooks
	mockCoordinator := &mockServiceCoordinator{
		hooks: nil,
	}

	// Create handler
	handler := &Handler{
		intelligenceService: mockCoordinator,
		serviceCoordinator:  mockCoordinator,
	}

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/hooks/reload", nil)

	// Execute handler
	handler.ReloadHooks(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["enabled"].(bool))
	assert.Equal(t, "Hooks manager is not initialized", response["message"])
}

// invalidIntelligenceService is a mock that implements IntelligenceService but not ServiceCoordinatorInterface
type invalidIntelligenceService struct{}

func (s *invalidIntelligenceService) IsEnabled() bool { return true }
func (s *invalidIntelligenceService) GetSemanticCache() intelligence.SemanticCacheInterface {
	return nil
}

// TestReloadSteering_WithInvalidCoordinator tests behavior with non-coordinator interface.
func TestReloadSteering_WithInvalidCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		intelligenceService: &invalidIntelligenceService{},
		serviceCoordinator:  nil, // Does not implement ServiceCoordinatorInterface
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/steering/reload", nil)

	handler.ReloadSteering(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["enabled"].(bool))
	assert.Contains(t, response["message"], "does not support")
}

// TestReloadHooks_WithInvalidCoordinator tests behavior with non-coordinator interface.
func TestReloadHooks_WithInvalidCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		intelligenceService: &invalidIntelligenceService{},
		serviceCoordinator:  nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/hooks/reload", nil)

	handler.ReloadHooks(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["enabled"].(bool))
	assert.Contains(t, response["message"], "does not support")
}
