// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/intelligence"
)

// FeedbackRequest represents a request to submit feedback.
type FeedbackRequest struct {
	Query           string                 `json:"query" binding:"required"`
	Intent          string                 `json:"intent" binding:"required"`
	SelectedModel   string                 `json:"selected_model" binding:"required"`
	RoutingTier     string                 `json:"routing_tier" binding:"required"`
	Confidence      float64                `json:"confidence"`
	MatchedSkill    string                 `json:"matched_skill,omitempty"`
	CascadeOccurred bool                   `json:"cascade_occurred"`
	ResponseQuality float64                `json:"response_quality,omitempty"`
	LatencyMs       int64                  `json:"latency_ms" binding:"required"`
	Success         bool                   `json:"success"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// FeedbackStatsResponse represents aggregated feedback statistics.
type FeedbackStatsResponse struct {
	TotalRecords     int64              `json:"total_records"`
	SuccessRate      float64            `json:"success_rate"`
	TierDistribution map[string]int64   `json:"tier_distribution"`
	AvgLatencyMs     float64            `json:"avg_latency_ms"`
	CascadeRate      float64            `json:"cascade_rate"`
}

// FeedbackHandler handles the /v0/management/feedback endpoint.
// It requires an intelligence service to be provided.
type FeedbackHandler struct {
	intelligenceService *intelligence.Service
}

// NewFeedbackHandler creates a new feedback handler.
//
// Parameters:
//   - intelligenceService: The intelligence service instance
//
// Returns:
//   - *FeedbackHandler: A new handler instance
func NewFeedbackHandler(intelligenceService *intelligence.Service) *FeedbackHandler {
	return &FeedbackHandler{
		intelligenceService: intelligenceService,
	}
}

// SubmitFeedback handles POST /v0/management/feedback
// Accepts explicit feedback submission from clients.
//
// Request Body:
//   - FeedbackRequest: The feedback data
//
// Response:
//   - 201: Feedback recorded successfully
//   - 400: Invalid request body
//   - 503: Intelligence services not enabled
func (h *FeedbackHandler) SubmitFeedback(c *gin.Context) {
	// Check if intelligence service is enabled
	if h.intelligenceService == nil || !h.intelligenceService.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "intelligence services not enabled",
		})
		return
	}

	// Get feedback collector
	collector := h.intelligenceService.GetFeedbackCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback collector not available",
		})
		return
	}

	if !collector.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback collector not initialized",
		})
		return
	}

	// Parse request body
	var req FeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	// Convert to feedback record
	record := &intelligence.FeedbackRecord{
		Query:           req.Query,
		Intent:          req.Intent,
		SelectedModel:   req.SelectedModel,
		RoutingTier:     req.RoutingTier,
		Confidence:      req.Confidence,
		MatchedSkill:    req.MatchedSkill,
		CascadeOccurred: req.CascadeOccurred,
		ResponseQuality: req.ResponseQuality,
		LatencyMs:       req.LatencyMs,
		Success:         req.Success,
		ErrorMessage:    req.ErrorMessage,
		Metadata:        req.Metadata,
	}

	// Record feedback
	if err := collector.Record(c.Request.Context(), record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to record feedback: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "feedback recorded successfully",
	})
}

// GetFeedbackStats handles GET /v0/management/feedback/stats
// Returns aggregated statistics about feedback records.
//
// Response:
//   - 200: Success with statistics
//   - 503: Intelligence services not enabled
func (h *FeedbackHandler) GetFeedbackStats(c *gin.Context) {
	// Check if intelligence service is enabled
	if h.intelligenceService == nil || !h.intelligenceService.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "intelligence services not enabled",
		})
		return
	}

	// Get feedback collector
	collector := h.intelligenceService.GetFeedbackCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback collector not available",
		})
		return
	}

	if !collector.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback collector not initialized",
		})
		return
	}

	// Get stats
	stats, err := collector.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get stats: " + err.Error(),
		})
		return
	}

	// Convert to response format
	response := FeedbackStatsResponse{
		TotalRecords:     stats["total_records"].(int64),
		SuccessRate:      stats["success_rate"].(float64),
		TierDistribution: stats["tier_distribution"].(map[string]int64),
		AvgLatencyMs:     stats["avg_latency_ms"].(float64),
		CascadeRate:      stats["cascade_rate"].(float64),
	}

	c.JSON(http.StatusOK, response)
}

// GetRecentFeedback handles GET /v0/management/feedback/recent
// Returns the most recent feedback records.
//
// Query Parameters:
//   - limit: Maximum number of records to return (default: 100)
//
// Response:
//   - 200: Success with feedback records
//   - 503: Intelligence services not enabled
func (h *FeedbackHandler) GetRecentFeedback(c *gin.Context) {
	// Check if intelligence service is enabled
	if h.intelligenceService == nil || !h.intelligenceService.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "intelligence services not enabled",
		})
		return
	}

	// Get feedback collector
	collector := h.intelligenceService.GetFeedbackCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback collector not available",
		})
		return
	}

	if !collector.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback collector not initialized",
		})
		return
	}

	// Parse limit parameter
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get recent records
	recordsInterface, err := collector.GetRecent(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get recent feedback: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"records": recordsInterface,
	})
}
