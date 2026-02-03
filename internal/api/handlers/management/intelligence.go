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
	GetMemory() interface{}
	GetHeartbeat() interface{}
	GetSteering() interface{}
	GetHooks() interface{}
}

// GetMemoryStats returns statistics about the memory system.
// GET /v0/management/memory/stats
func (h *Handler) GetMemoryStats(c *gin.Context) {
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Memory system is not initialized"})
		return
	}
	coordinator := h.serviceCoordinator
	if coordinator == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Intelligence service does not support memory system"})
		return
	}
	memoryManagerRaw := coordinator.GetMemory()
	if memoryManagerRaw == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Memory system is disabled"})
		return
	}
	memoryManager, ok := memoryManagerRaw.(memory.MemoryManager)
	if !ok || memoryManager == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Memory system is disabled"})
		return
	}
	stats, err := memoryManager.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve memory statistics", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "stats": stats})
}

// GetHeartbeatStatus returns the current status of all monitored providers.
// GET /v0/management/heartbeat/status
func (h *Handler) GetHeartbeatStatus(c *gin.Context) {
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Heartbeat monitor is not initialized"})
		return
	}
	coordinator := h.serviceCoordinator
	if coordinator == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Intelligence service does not support heartbeat monitor"})
		return
	}
	hbMonitorRaw := coordinator.GetHeartbeat()
	if hbMonitorRaw == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Heartbeat monitor is disabled"})
		return
	}
	hbMonitor, ok := hbMonitorRaw.(heartbeat.HeartbeatMonitor)
	if !ok || hbMonitor == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Heartbeat monitor is disabled"})
		return
	}
	statuses := hbMonitor.GetAllStatuses()
	response := gin.H{"enabled": true, "providers": statuses}
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
func (h *Handler) GetSteeringRules(c *gin.Context) {
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
	rules := steeringEngine.GetRules()
	c.JSON(http.StatusOK, gin.H{"enabled": true, "rules_count": len(rules), "rules": rules})
}

// GetHooksStatus returns all loaded hooks and their configuration.
// GET /v0/management/hooks/status
func (h *Handler) GetHooksStatus(c *gin.Context) {
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
	allHooks := hooksMgr.GetHooks()
	hooksDir := hooksMgr.GetHooksDir()
	c.JSON(http.StatusOK, gin.H{"enabled": true, "hooks_count": len(allHooks), "hooks_dir": hooksDir, "hooks": allHooks})
}

// GetAnalytics returns computed analytics from the memory system.
// GET /v0/management/analytics
func (h *Handler) GetAnalytics(c *gin.Context) {
	if h.intelligenceService == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Memory system is not initialized"})
		return
	}
	coordinator := h.serviceCoordinator
	if coordinator == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Intelligence service does not support memory system"})
		return
	}
	memoryManagerRaw := coordinator.GetMemory()
	if memoryManagerRaw == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Memory system is disabled"})
		return
	}
	memoryManager, ok := memoryManagerRaw.(memory.MemoryManager)
	if !ok || memoryManager == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "Memory system is disabled"})
		return
	}
	analytics, err := memoryManager.GetAnalytics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve analytics", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "analytics": analytics})
}
