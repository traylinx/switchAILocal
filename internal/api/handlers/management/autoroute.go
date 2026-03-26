package management

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetAutoRouteStatus returns the active weights and metrics from the AutoRouting Lab.
func (h *Handler) GetAutoRouteStatus(c *gin.Context) {
	if h.autoResolver == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-routing not initialized"})
		return
	}

	status := h.autoResolver.GetLabStatus()
	if status == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetAutoRouteJournal returns the latest routing decisions from the ring buffer.
func (h *Handler) GetAutoRouteJournal(c *gin.Context) {
	if h.autoResolver == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auto-routing not initialized"})
		return
	}

	limit := 50
	limitStr := c.Query("limit")
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 && val <= 100 {
			limit = val
		}
	}

	journal := h.autoResolver.GetRecentJournal(limit)
	if journal == nil {
		c.JSON(http.StatusOK, gin.H{"entries": []interface{}{}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"entries": journal})
}
