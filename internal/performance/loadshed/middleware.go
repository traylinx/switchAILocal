// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package loadshed provides graceful load shedding middleware for the switchAILocal proxy.
// When in-flight requests exceed a configurable threshold, new requests are rejected
// with HTTP 503 and a Retry-After header, preventing system degradation under burst load.
package loadshed

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/observability"
)

// Middleware returns a Gin middleware that tracks in-flight requests
// and sheds load when the threshold is exceeded.
//
// The in-flight counter uses atomic operations for zero-contention tracking.
// This middleware should be positioned early in the chain, after basic
// recovery/logging but before auth and routing.
func Middleware(cfg config.LoadSheddingConfig) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	var inFlight atomic.Int64
	maxInFlight := int64(cfg.MaxInFlight)
	retryAfter := fmt.Sprintf("%d", cfg.RetryAfterSeconds)

	log.WithFields(log.Fields{
		"max_in_flight":    cfg.MaxInFlight,
		"retry_after_secs": cfg.RetryAfterSeconds,
	}).Info("Load shedding enabled")

	return func(c *gin.Context) {
		current := inFlight.Add(1)
		observability.InFlightRequests.Set(float64(current))
		defer func() {
			updated := inFlight.Add(-1)
			observability.InFlightRequests.Set(float64(updated))
		}()

		if current > maxInFlight {
			observability.LoadSheddedTotal.Inc()
			c.Header("Retry-After", retryAfter)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"message": "Service temporarily overloaded. Please retry after the Retry-After period.",
					"type":    "server_error",
					"code":    "service_unavailable",
				},
			})
			return
		}

		c.Next()
	}
}
