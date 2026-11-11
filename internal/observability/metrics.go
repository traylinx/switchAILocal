// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package observability

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	// requestsTotal tracks the total number of AI API requests.
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "switchailocal_requests_total",
			Help: "Total number of HTTP requests processed by proxy",
		},
		[]string{"model", "provider", "status", "auto_routed"},
	)

	// requestLatency tracks the latency of proxy requests in milliseconds.
	requestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "switchailocal_request_duration_milliseconds",
			Help:    "Duration of HTTP requests in milliseconds",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 20000, 45000, 90000},
		},
		[]string{"model", "provider", "auto_routed"},
	)

	// llmTokensTotal tracks the total number of tokens consumed.
	llmTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "switchailocal_llm_tokens_total",
			Help: "Total number of LLM tokens consumed",
		},
		[]string{"model", "provider", "token_type"}, // token_type = "input" or "output"
	)

	// routingQualityScore tracks the computed RQS for auto-routed requests.
	routingQualityScore = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "switchailocal_routing_quality_score",
			Help:    "Routing Quality Score (RQS) of successful decisions",
			Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
		},
		[]string{"model", "provider"},
	)

	// fallbacksTotal tracks the total number of fallback attempts.
	fallbacksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "switchailocal_fallbacks_total",
			Help: "Total number of provider failovers/fallbacks executed",
		},
		[]string{"requested_model"},
	)

	// InFlightRequests tracks the number of requests currently being processed.
	InFlightRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "switchailocal_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
	)

	// ProviderHealthScore tracks real-time provider health scores.
	ProviderHealthScore = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "switchailocal_provider_health_score",
			Help: "Provider health score (0-1) based on success rate, latency, and quota",
		},
		[]string{"provider", "status"},
	)

	// RateLimitedTotal tracks the total number of rate-limited requests.
	RateLimitedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "switchailocal_rate_limited_total",
			Help: "Total number of requests rejected by rate limiter",
		},
		[]string{"scope"}, // "global" or "per_key"
	)

	// LoadSheddedTotal tracks the total number of load-shed requests.
	LoadSheddedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "switchailocal_load_shed_total",
			Help: "Total number of requests rejected by load shedding",
		},
	)
)

// MetricsMiddleware creates a Gin middleware that records Prometheus telemetry.
func MetricsMiddleware(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if !enabled {
			return
		}

		// Only track standard proxy routes
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/api/provider/") {
			return
		}

		duration := time.Since(start).Milliseconds()
		status := strconv.Itoa(c.Writer.Status())

		model := c.GetString("requested_model")
		if model == "" {
			model = "unknown"
		}

		provider := c.Writer.Header().Get("X-Provider")
		if provider == "" {
			provider = "unknown"
		}

		autoRouted := "false"
		if c.Writer.Header().Get("X-Auto-Routed") == "true" {
			autoRouted = "true"
		}

		// Update base metrics
		requestsTotal.WithLabelValues(model, provider, status, autoRouted).Inc()
		requestLatency.WithLabelValues(model, provider, autoRouted).Observe(float64(duration))

		// Update token metrics if available
		if inStr := c.Writer.Header().Get("X-Usage-Input"); inStr != "" {
			if tokens, err := strconv.ParseFloat(inStr, 64); err == nil {
				llmTokensTotal.WithLabelValues(model, provider, "input").Add(tokens)
			}
		}
		if outStr := c.Writer.Header().Get("X-Usage-Output"); outStr != "" {
			if tokens, err := strconv.ParseFloat(outStr, 64); err == nil {
				llmTokensTotal.WithLabelValues(model, provider, "output").Add(tokens)
			}
		}

		// Update intelligence routing metrics
		if autoRouted == "true" {
			if rqsStr := c.Writer.Header().Get("X-RQS"); rqsStr != "" {
				if rqs, err := strconv.ParseFloat(rqsStr, 64); err == nil {
					routingQualityScore.WithLabelValues(model, provider).Observe(rqs)
				}
			}
			if fallbacksStr := c.Writer.Header().Get("X-Fallbacks"); fallbacksStr != "" {
				if fallbacks, err := strconv.ParseFloat(fallbacksStr, 64); err == nil && fallbacks > 0 {
					fallbacksTotal.WithLabelValues(model).Add(fallbacks)
				}
			}
		}
	}
}

// RegisterMetricsRoute attaches the /metrics endpoint to the provided router if enabled in config.
func RegisterMetricsRoute(router *gin.Engine, enabled bool, path string) {
	if !enabled || path == "" {
		return
	}

	// Add prometheus handler to global router
	router.GET(path, gin.WrapH(promhttp.Handler()))
	log.Infof("Registered Prometheus metrics endpoint at %s", path)
}
