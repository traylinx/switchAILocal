// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// ServiceCoordinatorInterface defines the interface for accessing the service coordinator.
type ServiceCoordinatorInterface interface {
	GetMemory() memory.MemoryManager
	GetHeartbeat() heartbeat.HeartbeatMonitor
	GetSteering() *steering.SteeringEngine
	GetHooks() *hooks.HookManager
}

// GetMemoryStats returns statistics about the memory system.
// GET /v0/management/memory/stats
//
// Response:
//
//	{
//	  "enabled": true,
//	  "stats": {
//	    "total_decisions": 1234,
//	    "total_users": 56,
//	    "total_quirks": 12,
//	    "disk_usage_bytes": 1048576,
//	    "oldest_decision": "2024-01-01T00:00:00Z",
//	    "newest_decision": "2024-01-15T12:34:56Z",
//	    "last_cleanup": "2024-01-15T00:00:00Z",
//	    "retention_days": 30,
//	    "compression_enabled": true
//	  }
//	}
//
// Validates: Requirements 6.1
func (h *Handler) GetMemoryStats(c *gin.Context) {
	// Check if intelligence service is available
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Memory system is not initialized",
		})
		return
	}

	// Try to get the service coordinator interface
	coordinator, ok := h.intelligenceService.(ServiceCoordinatorInterface)
	if !ok {
		// Intelligence service doesn't support coordinator interface
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Intelligence service does not support memory system",
		})
		return
	}

	// Get memory manager
	memoryManager := coordinator.GetMemory()
	if memoryManager == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Memory system is disabled",
		})
		return
	}

	// Get memory statistics
	stats, err := memoryManager.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve memory statistics",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": true,
		"stats":   stats,
	})
}

// GetHeartbeatStatus returns the current status of all monitored providers.
// GET /v0/management/heartbeat/status
//
// Response:
//
//	{
//	  "enabled": true,
//	  "running": true,
//	  "stats": {
//	    "start_time": "2024-01-15T00:00:00Z",
//	    "total_cycles": 100,
//	    "total_checks": 500,
//	    "successful_checks": 480,
//	    "failed_checks": 20,
//	    "providers_monitored": 5,
//	    "healthy_providers": 4,
//	    "degraded_providers": 1,
//	    "unavailable_providers": 0
//	  },
//	  "providers": {
//	    "openai": {
//	      "provider": "openai",
//	      "status": "healthy",
//	      "last_check": "2024-01-15T12:34:56Z",
//	      "response_time": 123,
//	      "quota_used": 1000,
//	      "quota_limit": 10000
//	    }
//	  }
//	}
//
// Validates: Requirements 6.2
func (h *Handler) GetHeartbeatStatus(c *gin.Context) {
	// Check if intelligence service is available
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Heartbeat monitor is not initialized",
		})
		return
	}

	// Try to get the service coordinator interface
	coordinator, ok := h.intelligenceService.(ServiceCoordinatorInterface)
	if !ok {
		// Intelligence service doesn't support coordinator interface
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Intelligence service does not support heartbeat monitor",
		})
		return
	}

	// Get heartbeat monitor
	hbMonitor := coordinator.GetHeartbeat()
	if hbMonitor == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Heartbeat monitor is disabled",
		})
		return
	}

	// Get all provider statuses
	statuses := hbMonitor.GetAllStatuses()

	// Build response with available information
	response := gin.H{
		"enabled":   true,
		"providers": statuses,
	}

	// Try to get additional stats if available (using type assertion to concrete type)
	if monitor, ok := hbMonitor.(interface {
		GetStats() *heartbeat.HeartbeatStats
		IsRunning() bool
	}); ok {
		response["stats"] = monitor.GetStats()
		response["running"] = monitor.IsRunning()
	}

	c.JSON(http.StatusOK, response)
}

// GetSteeringRules returns all loaded steering rules.
// GET /v0/management/steering/rules
//
// Response:
//
//	{
//	  "enabled": true,
//	  "rules_count": 5,
//	  "rules": [
//	    {
//	      "name": "Prefer Claude for coding",
//	      "description": "Route coding requests to Claude",
//	      "activation": {
//	        "condition": "contains(Request.Messages[0].Content, 'code')",
//	        "priority": 100
//	      },
//	      "preferences": {
//	        "primary_model": "claude-3-5-sonnet-20241022",
//	        "fallback_models": ["gpt-4o"],
//	        "override_router": true
//	      }
//	    }
//	  ]
//	}
//
// Validates: Requirements 6.3
func (h *Handler) GetSteeringRules(c *gin.Context) {
	// Check if intelligence service is available
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Steering engine is not initialized",
		})
		return
	}

	// Try to get the service coordinator interface
	coordinator, ok := h.intelligenceService.(ServiceCoordinatorInterface)
	if !ok {
		// Intelligence service doesn't support coordinator interface
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Intelligence service does not support steering engine",
		})
		return
	}

	// Get steering engine
	steering := coordinator.GetSteering()
	if steering == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Steering engine is not initialized",
		})
		return
	}

	// Get all rules
	rules := steering.GetRules()

	c.JSON(http.StatusOK, gin.H{
		"enabled":     true,
		"rules_count": len(rules),
		"rules":       rules,
	})
}

// GetHooksStatus returns all loaded hooks and their configuration.
// GET /v0/management/hooks/status
//
// Response:
//
//	{
//	  "enabled": true,
//	  "hooks_count": 3,
//	  "hooks_dir": "/home/user/.switchailocal/hooks",
//	  "hooks": [
//	    {
//	      "id": "quota-alert",
//	      "name": "Quota Alert",
//	      "description": "Send alert when quota is exceeded",
//	      "event": "quota_exceeded",
//	      "condition": "Data.quota_ratio > 0.9",
//	      "action": "log",
//	      "enabled": true
//	    }
//	  ]
//	}
//
// Validates: Requirements 6.4
func (h *Handler) GetHooksStatus(c *gin.Context) {
	// Check if intelligence service is available
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Hooks manager is not initialized",
		})
		return
	}

	// Try to get the service coordinator interface
	coordinator, ok := h.intelligenceService.(ServiceCoordinatorInterface)
	if !ok {
		// Intelligence service doesn't support coordinator interface
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Intelligence service does not support hooks manager",
		})
		return
	}

	// Get hooks manager
	hooks := coordinator.GetHooks()
	if hooks == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Hooks manager is not initialized",
		})
		return
	}

	// Get all hooks
	allHooks := hooks.GetHooks()
	hooksDir := hooks.GetHooksDir()

	c.JSON(http.StatusOK, gin.H{
		"enabled":     true,
		"hooks_count": len(allHooks),
		"hooks_dir":   hooksDir,
		"hooks":       allHooks,
	})
}

// GetAnalytics returns computed analytics from the memory system.
// GET /v0/management/analytics
//
// Response:
//
//	{
//	  "enabled": true,
//	  "analytics": {
//	    "generated_at": "2024-01-15T12:34:56Z",
//	    "provider_stats": {
//	      "openai": {
//	        "total_requests": 1000,
//	        "successful_requests": 980,
//	        "failed_requests": 20,
//	        "average_latency": 123.45,
//	        "success_rate": 0.98
//	      }
//	    },
//	    "model_performance": {
//	      "gpt-4o": {
//	        "total_requests": 500,
//	        "average_latency": 150.0,
//	        "success_rate": 0.99
//	      }
//	    }
//	  }
//	}
//
// Validates: Requirements 6.7
func (h *Handler) GetAnalytics(c *gin.Context) {
	// Check if intelligence service is available
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Memory system is not initialized",
		})
		return
	}

	// Try to get the service coordinator interface
	coordinator, ok := h.intelligenceService.(ServiceCoordinatorInterface)
	if !ok {
		// Intelligence service doesn't support coordinator interface
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Intelligence service does not support memory system",
		})
		return
	}

	// Get memory manager
	memoryManager := coordinator.GetMemory()
	if memoryManager == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"message": "Memory system is disabled",
		})
		return
	}

	// Get analytics (returns cached analytics if available)
	analytics, err := memoryManager.GetAnalytics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve analytics",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":   true,
		"analytics": analytics,
	})
}
