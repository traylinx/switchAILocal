// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/integration"
	"github.com/traylinx/switchAILocal/internal/intelligence"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// mockMemoryManager implements memory.MemoryManager for testing
type mockMemoryManager struct {
	stats     *memory.MemoryStats
	analytics *memory.AnalyticsSummary
	statsErr  error
	analyticsErr error
}

func (m *mockMemoryManager) RecordRouting(decision *memory.RoutingDecision) error { return nil }
func (m *mockMemoryManager) GetUserPreferences(apiKeyHash string) (*memory.UserPreferences, error) { return nil, nil }
func (m *mockMemoryManager) UpdateUserPreferences(prefs *memory.UserPreferences) error { return nil }
func (m *mockMemoryManager) DeleteUserPreferences(apiKeyHash string) error { return nil }
func (m *mockMemoryManager) AddQuirk(quirk *memory.Quirk) error { return nil }
func (m *mockMemoryManager) GetProviderQuirks(provider string) ([]*memory.Quirk, error) { return nil, nil }
func (m *mockMemoryManager) GetHistory(apiKeyHash string, limit int) ([]*memory.RoutingDecision, error) { return nil, nil }
func (m *mockMemoryManager) GetAllHistory(limit int) ([]*memory.RoutingDecision, error) { return nil, nil }
func (m *mockMemoryManager) LearnFromOutcome(decision *memory.RoutingDecision) error { return nil }
func (m *mockMemoryManager) GetStats() (*memory.MemoryStats, error) { return m.stats, m.statsErr }
func (m *mockMemoryManager) GetAnalytics() (*memory.AnalyticsSummary, error) { return m.analytics, m.analyticsErr }
func (m *mockMemoryManager) ComputeAnalytics() (*memory.AnalyticsSummary, error) { return m.analytics, m.analyticsErr }
func (m *mockMemoryManager) Cleanup() error { return nil }
func (m *mockMemoryManager) Close() error { return nil }

// mockHeartbeatMonitor implements heartbeat.HeartbeatMonitor for testing
type mockHeartbeatMonitor struct {
	stats    *heartbeat.HeartbeatStats
	statuses map[string]*heartbeat.HealthStatus
	running  bool
}

func (m *mockHeartbeatMonitor) Start(ctx context.Context) error { return nil }
func (m *mockHeartbeatMonitor) Stop() error { return nil }
func (m *mockHeartbeatMonitor) CheckAll(ctx context.Context) error { return nil }
func (m *mockHeartbeatMonitor) CheckProvider(ctx context.Context, provider string) (*heartbeat.HealthStatus, error) { return nil, nil }
func (m *mockHeartbeatMonitor) GetStatus(provider string) (*heartbeat.HealthStatus, error) { return m.statuses[provider], nil }
func (m *mockHeartbeatMonitor) GetAllStatuses() map[string]*heartbeat.HealthStatus { return m.statuses }
func (m *mockHeartbeatMonitor) RegisterChecker(checker heartbeat.ProviderHealthChecker) error { return nil }
func (m *mockHeartbeatMonitor) UnregisterChecker(provider string) error { return nil }
func (m *mockHeartbeatMonitor) SetInterval(interval time.Duration) {}
func (m *mockHeartbeatMonitor) GetInterval() time.Duration { return time.Minute }
func (m *mockHeartbeatMonitor) AddEventHandler(handler heartbeat.HeartbeatEventHandler) {}
func (m *mockHeartbeatMonitor) RemoveEventHandler(handler heartbeat.HeartbeatEventHandler) {}
func (m *mockHeartbeatMonitor) GetStats() *heartbeat.HeartbeatStats { return m.stats }
func (m *mockHeartbeatMonitor) IsRunning() bool { return m.running }
func (m *mockHeartbeatMonitor) GetConfig() *heartbeat.HeartbeatConfig { return nil }

// mockServiceCoordinator implements a minimal service coordinator for testing
type mockServiceCoordinator struct {
	memory    memory.MemoryManager
	heartbeat heartbeat.HeartbeatMonitor
	steering  *steering.SteeringEngine
	hooks     *hooks.HookManager
}

func (m *mockServiceCoordinator) IsEnabled() bool { return true }
func (m *mockServiceCoordinator) GetSemanticCache() intelligence.SemanticCacheInterface { return nil }
func (m *mockServiceCoordinator) GetMemory() memory.MemoryManager { return m.memory }
func (m *mockServiceCoordinator) GetHeartbeat() heartbeat.HeartbeatMonitor { return m.heartbeat }
func (m *mockServiceCoordinator) GetSteering() *steering.SteeringEngine { return m.steering }
func (m *mockServiceCoordinator) GetHooks() *hooks.HookManager { return m.hooks }

func TestGetMemoryStats(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("memory system disabled", func(t *testing.T) {
		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				memory: nil, // Memory disabled
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/memory/stats", nil)

		handler.GetMemoryStats(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.False(t, response["enabled"].(bool))
		assert.Contains(t, response["message"], "disabled")
	})

	t.Run("memory system enabled with stats", func(t *testing.T) {
		mockStats := &memory.MemoryStats{
			TotalDecisions:     1234,
			TotalUsers:         56,
			TotalQuirks:        12,
			DiskUsageBytes:     1048576,
			RetentionDays:      30,
			CompressionEnabled: true,
		}

		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				memory: &mockMemoryManager{
					stats: mockStats,
				},
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/memory/stats", nil)

		handler.GetMemoryStats(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.True(t, response["enabled"].(bool))
		assert.NotNil(t, response["stats"])
		
		stats := response["stats"].(map[string]interface{})
		assert.Equal(t, float64(1234), stats["total_decisions"])
		assert.Equal(t, float64(56), stats["total_users"])
		assert.Equal(t, float64(12), stats["total_quirks"])
	})

	t.Run("intelligence service not initialized", func(t *testing.T) {
		handler := &Handler{
			intelligenceService: nil,
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/memory/stats", nil)

		handler.GetMemoryStats(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.False(t, response["enabled"].(bool))
	})
}

func TestGetHeartbeatStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("heartbeat disabled", func(t *testing.T) {
		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				heartbeat: nil, // Heartbeat disabled
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/heartbeat/status", nil)

		handler.GetHeartbeatStatus(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.False(t, response["enabled"].(bool))
		assert.Contains(t, response["message"], "disabled")
	})

	t.Run("heartbeat enabled with status", func(t *testing.T) {
		mockStats := &heartbeat.HeartbeatStats{
			StartTime:              time.Now(),
			TotalCycles:            100,
			TotalChecks:            500,
			SuccessfulChecks:       480,
			FailedChecks:           20,
			ProvidersMonitored:     5,
			HealthyProviders:       4,
			DegradedProviders:      1,
			UnavailableProviders:   0,
		}

		mockStatuses := map[string]*heartbeat.HealthStatus{
			"openai": {
				Provider:     "openai",
				Status:       heartbeat.StatusHealthy,
				LastCheck:    time.Now(),
				ResponseTime: 123,
			},
		}

		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				heartbeat: &mockHeartbeatMonitor{
					stats:    mockStats,
					statuses: mockStatuses,
					running:  true,
				},
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/heartbeat/status", nil)

		handler.GetHeartbeatStatus(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.True(t, response["enabled"].(bool))
		assert.True(t, response["running"].(bool))
		assert.NotNil(t, response["stats"])
		assert.NotNil(t, response["providers"])
	})
}

func TestGetSteeringRules(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("steering engine not initialized", func(t *testing.T) {
		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				steering: nil,
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/steering/rules", nil)

		handler.GetSteeringRules(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.False(t, response["enabled"].(bool))
	})

	t.Run("steering engine with rules", func(t *testing.T) {
		// Create a temporary directory for steering rules
		tmpDir := t.TempDir()
		
		steeringEngine, err := steering.NewSteeringEngine(tmpDir)
		require.NoError(t, err)

		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				steering: steeringEngine,
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/steering/rules", nil)

		handler.GetSteeringRules(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.True(t, response["enabled"].(bool))
		assert.NotNil(t, response["rules"])
		assert.Equal(t, float64(0), response["rules_count"]) // No rules loaded
	})
}

func TestGetHooksStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("hooks manager not initialized", func(t *testing.T) {
		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				hooks: nil,
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/hooks/status", nil)

		handler.GetHooksStatus(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.False(t, response["enabled"].(bool))
	})

	t.Run("hooks manager with hooks", func(t *testing.T) {
		// Create a temporary directory for hooks
		tmpDir := t.TempDir()
		
		eventBus := hooks.NewEventBus()
		hooksManager, err := hooks.NewHookManager(tmpDir, eventBus)
		require.NoError(t, err)

		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				hooks: hooksManager,
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/hooks/status", nil)

		handler.GetHooksStatus(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.True(t, response["enabled"].(bool))
		assert.NotNil(t, response["hooks"])
		assert.Equal(t, float64(0), response["hooks_count"]) // No hooks loaded
		assert.NotEmpty(t, response["hooks_dir"])
	})
}

func TestGetAnalytics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("memory system disabled", func(t *testing.T) {
		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				memory: nil,
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/analytics", nil)

		handler.GetAnalytics(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.False(t, response["enabled"].(bool))
	})

	t.Run("analytics available", func(t *testing.T) {
		mockAnalytics := &memory.AnalyticsSummary{
			GeneratedAt:       time.Now(),
			ProviderStats:     make(map[string]*memory.ProviderStats),
			ModelPerformance:  make(map[string]*memory.ModelPerformance),
			TierEffectiveness: &memory.TierEffectiveness{},
			CostAnalysis:      &memory.CostAnalysis{},
			TrendAnalysis:     &memory.TrendAnalysis{},
		}

		handler := &Handler{
			intelligenceService: &mockServiceCoordinator{
				memory: &mockMemoryManager{
					analytics: mockAnalytics,
				},
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/analytics", nil)

		handler.GetAnalytics(c)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.True(t, response["enabled"].(bool))
		assert.NotNil(t, response["analytics"])
	})
}

func TestIntelligenceHandlersWithRealCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a real service coordinator with all systems disabled
	cfg := &integration.IntegrationConfig{
		Memory: &memory.MemoryConfig{
			Enabled:       false,
			BaseDir:       t.TempDir(),
			RetentionDays: 30,
			MaxLogSizeMB:  100,
			Compression:   false,
		},
		Heartbeat: &heartbeat.HeartbeatConfig{
			Enabled:  false,
			Interval: time.Minute,
		},
		Steering: &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  t.TempDir(),
			HotReload: false,
		},
		Hooks: &config.HooksConfig{
			Enabled:   false,
			HooksDir:  t.TempDir(),
			HotReload: false,
		},
	}

	coordinator, err := integration.NewServiceCoordinator(cfg)
	require.NoError(t, err)

	// Wrap the coordinator in a mock that implements IntelligenceService
	mockWrapper := &mockServiceCoordinator{
		memory:    coordinator.GetMemory(),
		heartbeat: coordinator.GetHeartbeat(),
		steering:  coordinator.GetSteering(),
		hooks:     coordinator.GetHooks(),
	}

	handler := &Handler{
		intelligenceService: mockWrapper,
	}

	t.Run("all systems disabled", func(t *testing.T) {
		// Test memory stats
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/memory/stats", nil)
		handler.GetMemoryStats(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// Test heartbeat status
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/heartbeat/status", nil)
		handler.GetHeartbeatStatus(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// Test steering rules
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/steering/rules", nil)
		handler.GetSteeringRules(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// Test hooks status
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/hooks/status", nil)
		handler.GetHooksStatus(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// Test analytics
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v0/management/analytics", nil)
		handler.GetAnalytics(c)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
