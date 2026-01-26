// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
)

// GetSuperbrainMetrics returns the current Superbrain metrics snapshot.
// This endpoint exposes observability data for monitoring autonomous healing operations.
//
// Response includes:
// - Healing attempts, successes, and failures
// - Breakdown by healing type and failure type
// - Latency statistics
// - Active monitoring contexts and queued actions
// - Success/failure rates
func (h *Handler) GetSuperbrainMetrics(c *gin.Context) {
	m := metrics.Global()
	snapshot := m.Snapshot()

	// Enrich the snapshot with calculated rates
	response := gin.H{
		"superbrain": gin.H{
			// Counters
			"healing_attempts":       snapshot.HealingAttempts,
			"successful_healings":    snapshot.SuccessfulHealings,
			"failed_healings":        snapshot.FailedHealings,
			"silence_detections":     snapshot.SilenceDetections,
			"diagnoses_performed":    snapshot.DiagnosesPerformed,
			"fallbacks_triggered":    snapshot.FallbacksTriggered,
			"stdin_injections_total": snapshot.StdinInjectionsTotal,
			"restarts_total":         snapshot.RestartsTotal,
			"context_optimizations":  snapshot.ContextOptimizations,

			// By-type breakdowns
			"healing_by_type": snapshot.HealingByType,
			"failure_by_type": snapshot.FailureByType,

			// Latency statistics
			"latency_stats": snapshot.LatencyStats,

			// Gauges
			"active_monitoring_contexts": snapshot.ActiveMonitoringContexts,
			"queued_healing_actions":     snapshot.QueuedHealingActions,

			// Calculated rates
			"success_rate_percent": snapshot.SuccessRate(),
			"failure_rate_percent": snapshot.FailureRate(),

			// Metadata
			"uptime_seconds": snapshot.UptimeSeconds,
			"timestamp":      snapshot.Timestamp,
		},
	}

	c.JSON(http.StatusOK, response)
}
