// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ReloadSteering reloads steering rules from disk without restarting the server.
// POST /v0/management/steering/reload
//
// This endpoint enables hot-reload of steering rules. When called, it:
// 1. Reloads all steering rule files from the configured rules directory
// 2. Validates the new rules
// 3. Atomically replaces the active rules
// 4. Returns the count of loaded rules
//
// In-flight requests continue using the old rules until they complete.
// New requests immediately use the reloaded rules.
//
// Response on success:
//
//	{
//	  "success": true,
//	  "message": "Steering rules reloaded successfully",
//	  "rules_count": 5
//	}
//
// Response when steering is disabled:
//
//	{
//	  "enabled": false,
//	  "message": "Steering engine is not initialized"
//	}
//
// Response on error:
//
//	{
//	  "success": false,
//	  "error": "Failed to reload steering rules",
//	  "message": "detailed error message"
//	}
//
// Validates: Requirements 6.5
func (h *Handler) ReloadSteering(c *gin.Context) {
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

	// Reload rules from disk
	err := steering.LoadRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to reload steering rules",
			"message": err.Error(),
		})
		return
	}

	// Get the count of loaded rules
	rules := steering.GetRules()

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Steering rules reloaded successfully",
		"rules_count": len(rules),
	})
}

// ReloadHooks reloads hooks from disk without restarting the server.
// POST /v0/management/hooks/reload
//
// This endpoint enables hot-reload of hooks. When called, it:
// 1. Reloads all hook files from the configured hooks directory
// 2. Validates the new hooks
// 3. Atomically replaces the active hooks
// 4. Re-subscribes to events with the new hooks
// 5. Returns the count of loaded hooks
//
// In-flight hook executions continue to completion.
// New events immediately trigger the reloaded hooks.
//
// Response on success:
//
//	{
//	  "success": true,
//	  "message": "Hooks reloaded successfully",
//	  "hooks_count": 3
//	}
//
// Response when hooks are disabled:
//
//	{
//	  "enabled": false,
//	  "message": "Hooks manager is not initialized"
//	}
//
// Response on error:
//
//	{
//	  "success": false,
//	  "error": "Failed to reload hooks",
//	  "message": "detailed error message"
//	}
//
// Validates: Requirements 6.6
func (h *Handler) ReloadHooks(c *gin.Context) {
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

	// Reload hooks from disk
	err := hooks.LoadHooks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to reload hooks",
			"message": err.Error(),
		})
		return
	}

	// Get the count of loaded hooks
	allHooks := hooks.GetHooks()

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Hooks reloaded successfully",
		"hooks_count": len(allHooks),
	})
}
