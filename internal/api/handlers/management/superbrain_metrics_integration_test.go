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
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
)

// TestSuperbrainMetricsEndpointIntegration verifies the metrics endpoint
// works correctly when integrated with the management middleware.
func TestSuperbrainMetricsEndpointIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("endpoint returns metrics without authentication for localhost", func(t *testing.T) {
		// Reset metrics
		m := metrics.Global()
		m.Reset()

		// Record some test data
		m.RecordHealingAttempt()
		m.RecordHealingSuccess(100)
		m.RecordDiagnosis()

		// Create handler with minimal config
		cfg := &config.Config{}
		h := NewHandler(cfg, "", nil)

		// Create router
		router := gin.New()
		mgmt := router.Group("/v0/management")
		mgmt.Use(h.Middleware())
		{
			mgmt.GET("/metrics", h.GetSuperbrainMetrics)
		}

		// Create test request from localhost
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)
		req.RemoteAddr = "127.0.0.1:12345"

		router.ServeHTTP(w, req)

		// Verify response
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain, ok := response["superbrain"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'superbrain' key in response")
		}

		// Verify we got the metrics we recorded
		if superbrain["healing_attempts"] != float64(1) {
			t.Errorf("expected healing_attempts=1, got %v", superbrain["healing_attempts"])
		}
		if superbrain["diagnoses_performed"] != float64(1) {
			t.Errorf("expected diagnoses_performed=1, got %v", superbrain["diagnoses_performed"])
		}
	})

	t.Run("endpoint structure matches expected format", func(t *testing.T) {
		// Reset metrics
		m := metrics.Global()
		m.Reset()

		// Create handler
		cfg := &config.Config{}
		h := NewHandler(cfg, "", nil)

		// Create router
		router := gin.New()
		mgmt := router.Group("/v0/management")
		mgmt.Use(h.Middleware())
		{
			mgmt.GET("/metrics", h.GetSuperbrainMetrics)
		}

		// Create test request
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)
		req.RemoteAddr = "127.0.0.1:12345"

		router.ServeHTTP(w, req)

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain, ok := response["superbrain"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'superbrain' key in response")
		}

		// Verify all expected fields are present
		expectedFields := []string{
			"healing_attempts",
			"successful_healings",
			"failed_healings",
			"silence_detections",
			"diagnoses_performed",
			"fallbacks_triggered",
			"stdin_injections_total",
			"restarts_total",
			"context_optimizations",
			"healing_by_type",
			"failure_by_type",
			"latency_stats",
			"active_monitoring_contexts",
			"queued_healing_actions",
			"success_rate_percent",
			"failure_rate_percent",
			"uptime_seconds",
			"timestamp",
		}

		for _, field := range expectedFields {
			if _, ok := superbrain[field]; !ok {
				t.Errorf("expected field '%s' in superbrain metrics", field)
			}
		}
	})

	t.Run("latency_stats contains expected fields", func(t *testing.T) {
		// Reset and populate metrics with latency data
		m := metrics.Global()
		m.Reset()

		m.RecordHealingSuccess(100)
		m.RecordHealingSuccess(200)
		m.RecordHealingSuccess(150)

		// Create handler
		cfg := &config.Config{}
		h := NewHandler(cfg, "", nil)

		// Create router
		router := gin.New()
		mgmt := router.Group("/v0/management")
		mgmt.Use(h.Middleware())
		{
			mgmt.GET("/metrics", h.GetSuperbrainMetrics)
		}

		// Create test request
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)
		req.RemoteAddr = "127.0.0.1:12345"

		router.ServeHTTP(w, req)

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain := response["superbrain"].(map[string]interface{})
		latencyStats, ok := superbrain["latency_stats"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'latency_stats' to be an object")
		}

		// Verify latency stats fields
		expectedLatencyFields := []string{"average_ms", "min_ms", "max_ms", "samples"}
		for _, field := range expectedLatencyFields {
			if _, ok := latencyStats[field]; !ok {
				t.Errorf("expected field '%s' in latency_stats", field)
			}
		}

		// Verify latency calculations
		if latencyStats["samples"] != float64(3) {
			t.Errorf("expected samples=3, got %v", latencyStats["samples"])
		}
		if latencyStats["min_ms"] != float64(100) {
			t.Errorf("expected min_ms=100, got %v", latencyStats["min_ms"])
		}
		if latencyStats["max_ms"] != float64(200) {
			t.Errorf("expected max_ms=200, got %v", latencyStats["max_ms"])
		}
		if latencyStats["average_ms"] != float64(150) {
			t.Errorf("expected average_ms=150, got %v", latencyStats["average_ms"])
		}
	})
}
