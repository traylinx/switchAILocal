// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// DashboardResponse is the JSON structure returned by the observability dashboard endpoint.
type DashboardResponse struct {
	Timestamp string      `json:"timestamp"`
	System    SystemStats `json:"system"`
	Server    ServerStats `json:"server"`
}

// SystemStats reports Go runtime metrics.
type SystemStats struct {
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heap_alloc_mb"`
	HeapSysMB   float64 `json:"heap_sys_mb"`
	NumGC       uint32  `json:"num_gc"`
	GoVersion   string  `json:"go_version"`
	NumCPU      int     `json:"num_cpu"`
}

// ServerStats reports proxy server metadata.
type ServerStats struct {
	Uptime string `json:"uptime"`
}

// serverStartTime is set when the handler is created.
var serverStartTime = time.Now()

// HandleDashboard returns a Gin handler that serves the observability dashboard JSON.
// This endpoint aggregates runtime metrics without introducing new data collection —
// it reads from existing Go runtime stats and the Prometheus collectors already registered.
//
// The dashboard is designed for consumption by monitoring UIs and health check scripts.
func (h *Handler) HandleDashboard(c *gin.Context) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	resp := DashboardResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System: SystemStats{
			Goroutines:  runtime.NumGoroutine(),
			HeapAllocMB: float64(mem.HeapAlloc) / (1024 * 1024),
			HeapSysMB:   float64(mem.HeapSys) / (1024 * 1024),
			NumGC:       mem.NumGC,
			GoVersion:   runtime.Version(),
			NumCPU:      runtime.NumCPU(),
		},
		Server: ServerStats{
			Uptime: time.Since(serverStartTime).Truncate(time.Second).String(),
		},
	}

	c.JSON(http.StatusOK, resp)
}
