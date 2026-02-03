// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// ReloadSteering reloads steering rules from disk without restarting the server.
// POST /v0/management/steering/reload
func (h *Handler) ReloadSteering(c *gin.Context) {
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Steering engine is not initialized"})
		return
	}
	coordinator := h.serviceCoordinator
	if coordinator == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Intelligence service does not support steering engine"})
		return
	}
	steeringRaw := coordinator.GetSteering()
	if steeringRaw == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Steering engine is not initialized"})
		return
	}
	steeringEngine, ok := steeringRaw.(*steering.SteeringEngine)
	if !ok || steeringEngine == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Steering engine is not initialized"})
		return
	}
	err := steeringEngine.LoadRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to reload steering rules", "message": err.Error()})
		return
	}
	rules := steeringEngine.GetRules()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Steering rules reloaded successfully", "rules_count": len(rules)})
}

// ReloadHooks reloads hooks from disk without restarting the server.
// POST /v0/management/hooks/reload
func (h *Handler) ReloadHooks(c *gin.Context) {
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Hooks manager is not initialized"})
		return
	}
	coordinator := h.serviceCoordinator
	if coordinator == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Intelligence service does not support hooks manager"})
		return
	}
	hooksRaw := coordinator.GetHooks()
	if hooksRaw == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Hooks manager is not initialized"})
		return
	}
	hooksMgr, ok := hooksRaw.(*hooks.HookManager)
	if !ok || hooksMgr == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Hooks manager is not initialized"})
		return
	}
	err := hooksMgr.LoadHooks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to reload hooks", "message": err.Error()})
		return
	}
	allHooks := hooksMgr.GetHooks()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Hooks reloaded successfully", "hooks_count": len(allHooks)})
}
