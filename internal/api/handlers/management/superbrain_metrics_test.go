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
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
)

func TestGetSuperbrainMetrics(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("returns metrics snapshot with all fields", func(t *testing.T) {
		// Reset and populate metrics
		m := metrics.Global()
		m.Reset()

		// Record some test data
		m.RecordHealingAttempt()
		m.RecordHealingSuccess(150)
		m.RecordHealingAttempt()
		m.RecordHealingFailure()
		m.RecordSilenceDetection()
		m.RecordDiagnosis()
		m.RecordFallback()
		m.RecordStdinInjection()
		m.RecordRestart()
		m.RecordContextOptimization()
		m.RecordHealingByType("stdin_injection")
		m.RecordHealingByType("restart_with_flags")
		m.RecordFailureByType("permission_prompt")
		m.IncrementActiveMonitoring()
		m.IncrementQueuedActions()

		// Create handler
		h := &Handler{}

		// Create test request
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)

		// Call handler
		h.GetSuperbrainMetrics(c)

		// Verify response
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain, ok := response["superbrain"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'superbrain' key in response")
		}

		// Verify counters
		if superbrain["healing_attempts"] != float64(2) {
			t.Errorf("expected healing_attempts=2, got %v", superbrain["healing_attempts"])
		}
		if superbrain["successful_healings"] != float64(1) {
			t.Errorf("expected successful_healings=1, got %v", superbrain["successful_healings"])
		}
		if superbrain["failed_healings"] != float64(1) {
			t.Errorf("expected failed_healings=1, got %v", superbrain["failed_healings"])
		}
		if superbrain["silence_detections"] != float64(1) {
			t.Errorf("expected silence_detections=1, got %v", superbrain["silence_detections"])
		}
		if superbrain["diagnoses_performed"] != float64(1) {
			t.Errorf("expected diagnoses_performed=1, got %v", superbrain["diagnoses_performed"])
		}
		if superbrain["fallbacks_triggered"] != float64(1) {
			t.Errorf("expected fallbacks_triggered=1, got %v", superbrain["fallbacks_triggered"])
		}
		if superbrain["stdin_injections_total"] != float64(1) {
			t.Errorf("expected stdin_injections_total=1, got %v", superbrain["stdin_injections_total"])
		}
		if superbrain["restarts_total"] != float64(1) {
			t.Errorf("expected restarts_total=1, got %v", superbrain["restarts_total"])
		}
		if superbrain["context_optimizations"] != float64(1) {
			t.Errorf("expected context_optimizations=1, got %v", superbrain["context_optimizations"])
		}

		// Verify gauges
		if superbrain["active_monitoring_contexts"] != float64(1) {
			t.Errorf("expected active_monitoring_contexts=1, got %v", superbrain["active_monitoring_contexts"])
		}
		if superbrain["queued_healing_actions"] != float64(1) {
			t.Errorf("expected queued_healing_actions=1, got %v", superbrain["queued_healing_actions"])
		}

		// Verify by-type breakdowns exist
		if _, ok := superbrain["healing_by_type"]; !ok {
			t.Error("expected 'healing_by_type' in response")
		}
		if _, ok := superbrain["failure_by_type"]; !ok {
			t.Error("expected 'failure_by_type' in response")
		}

		// Verify latency stats exist
		if _, ok := superbrain["latency_stats"]; !ok {
			t.Error("expected 'latency_stats' in response")
		}

		// Verify calculated rates
		successRate, ok := superbrain["success_rate_percent"].(float64)
		if !ok {
			t.Error("expected 'success_rate_percent' to be a number")
		}
		if successRate != 50.0 {
			t.Errorf("expected success_rate_percent=50.0, got %v", successRate)
		}

		failureRate, ok := superbrain["failure_rate_percent"].(float64)
		if !ok {
			t.Error("expected 'failure_rate_percent' to be a number")
		}
		if failureRate != 50.0 {
			t.Errorf("expected failure_rate_percent=50.0, got %v", failureRate)
		}

		// Verify metadata
		if _, ok := superbrain["uptime_seconds"]; !ok {
			t.Error("expected 'uptime_seconds' in response")
		}
		if _, ok := superbrain["timestamp"]; !ok {
			t.Error("expected 'timestamp' in response")
		}
	})

	t.Run("returns zero metrics when no data recorded", func(t *testing.T) {
		// Reset metrics
		m := metrics.Global()
		m.Reset()

		// Create handler
		h := &Handler{}

		// Create test request
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)

		// Call handler
		h.GetSuperbrainMetrics(c)

		// Verify response
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain, ok := response["superbrain"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'superbrain' key in response")
		}

		// Verify all counters are zero
		if superbrain["healing_attempts"] != float64(0) {
			t.Errorf("expected healing_attempts=0, got %v", superbrain["healing_attempts"])
		}
		if superbrain["successful_healings"] != float64(0) {
			t.Errorf("expected successful_healings=0, got %v", superbrain["successful_healings"])
		}

		// Verify rates are zero when no attempts
		if superbrain["success_rate_percent"] != float64(0) {
			t.Errorf("expected success_rate_percent=0, got %v", superbrain["success_rate_percent"])
		}
		if superbrain["failure_rate_percent"] != float64(0) {
			t.Errorf("expected failure_rate_percent=0, got %v", superbrain["failure_rate_percent"])
		}
	})

	t.Run("includes healing_by_type breakdown", func(t *testing.T) {
		// Reset and populate metrics
		m := metrics.Global()
		m.Reset()

		m.RecordHealingByType("stdin_injection")
		m.RecordHealingByType("stdin_injection")
		m.RecordHealingByType("restart_with_flags")
		m.RecordHealingByType("fallback_routing")

		// Create handler
		h := &Handler{}

		// Create test request
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)

		// Call handler
		h.GetSuperbrainMetrics(c)

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain := response["superbrain"].(map[string]interface{})
		healingByType := superbrain["healing_by_type"].(map[string]interface{})

		if healingByType["stdin_injection"] != float64(2) {
			t.Errorf("expected stdin_injection=2, got %v", healingByType["stdin_injection"])
		}
		if healingByType["restart_with_flags"] != float64(1) {
			t.Errorf("expected restart_with_flags=1, got %v", healingByType["restart_with_flags"])
		}
		if healingByType["fallback_routing"] != float64(1) {
			t.Errorf("expected fallback_routing=1, got %v", healingByType["fallback_routing"])
		}
	})

	t.Run("includes failure_by_type breakdown", func(t *testing.T) {
		// Reset and populate metrics
		m := metrics.Global()
		m.Reset()

		m.RecordFailureByType("permission_prompt")
		m.RecordFailureByType("permission_prompt")
		m.RecordFailureByType("auth_error")
		m.RecordFailureByType("context_exceeded")

		// Create handler
		h := &Handler{}

		// Create test request
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/metrics", nil)

		// Call handler
		h.GetSuperbrainMetrics(c)

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		superbrain := response["superbrain"].(map[string]interface{})
		failureByType := superbrain["failure_by_type"].(map[string]interface{})

		if failureByType["permission_prompt"] != float64(2) {
			t.Errorf("expected permission_prompt=2, got %v", failureByType["permission_prompt"])
		}
		if failureByType["auth_error"] != float64(1) {
			t.Errorf("expected auth_error=1, got %v", failureByType["auth_error"])
		}
		if failureByType["context_exceeded"] != float64(1) {
			t.Errorf("expected context_exceeded=1, got %v", failureByType["context_exceeded"])
		}
	})
}
