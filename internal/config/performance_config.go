// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import "time"

// PerformanceConfig holds all production performance tuning settings.
type PerformanceConfig struct {
	// ProfilingEnabled toggles pprof HTTP server on PprofPort.
	ProfilingEnabled bool `yaml:"profiling-enabled" json:"profiling-enabled"`

	// PprofPort is the port for the pprof HTTP server. Default: 6060.
	PprofPort int `yaml:"pprof-port" json:"pprof-port"`

	// RateLimiter configures global and per-key rate limiting.
	RateLimiter RateLimiterConfig `yaml:"rate-limiter" json:"rate-limiter"`

	// CircuitBreaker configures per-provider circuit breaker behavior.
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit-breaker" json:"circuit-breaker"`

	// LoadShedding configures graceful degradation under load.
	LoadShedding LoadSheddingConfig `yaml:"load-shedding" json:"load-shedding"`
}

// RateLimiterConfig configures the token-bucket rate limiter.
type RateLimiterConfig struct {
	// Enabled toggles the rate limiter.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// GlobalRequestsPerSecond is the maximum sustained request rate across all keys.
	GlobalRequestsPerSecond float64 `yaml:"global-requests-per-second" json:"global-requests-per-second"`

	// GlobalBurst is the maximum burst size for the global limiter.
	GlobalBurst int `yaml:"global-burst" json:"global-burst"`

	// PerKeyRequestsPerSecond is the maximum sustained request rate per API key.
	PerKeyRequestsPerSecond float64 `yaml:"per-key-requests-per-second" json:"per-key-requests-per-second"`

	// PerKeyBurst is the maximum burst size per API key.
	PerKeyBurst int `yaml:"per-key-burst" json:"per-key-burst"`
}

// CircuitBreakerConfig configures per-provider circuit breakers.
type CircuitBreakerConfig struct {
	// Enabled toggles circuit breaker functionality.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// FailureThreshold is the number of consecutive failures before the circuit opens.
	FailureThreshold int `yaml:"failure-threshold" json:"failure-threshold"`

	// ResetTimeout is the duration to wait before transitioning from Open to Half-Open.
	ResetTimeout time.Duration `yaml:"reset-timeout" json:"reset-timeout"`

	// HalfOpenMax is the maximum number of probe requests allowed in the Half-Open state.
	HalfOpenMax int `yaml:"half-open-max" json:"half-open-max"`
}

// LoadSheddingConfig configures graceful load shedding.
type LoadSheddingConfig struct {
	// Enabled toggles load shedding.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// MaxInFlight is the maximum number of in-flight requests before shedding begins.
	MaxInFlight int `yaml:"max-in-flight" json:"max-in-flight"`

	// RetryAfterSeconds is the Retry-After header value returned with 503 responses.
	RetryAfterSeconds int `yaml:"retry-after-seconds" json:"retry-after-seconds"`
}

// SanitizePerformance applies safe defaults to performance configuration.
func (c *PerformanceConfig) SanitizePerformance() {
	if c.PprofPort == 0 {
		c.PprofPort = 6060
	}
	if c.RateLimiter.GlobalRequestsPerSecond == 0 {
		c.RateLimiter.GlobalRequestsPerSecond = 1000
	}
	if c.RateLimiter.GlobalBurst == 0 {
		c.RateLimiter.GlobalBurst = 500
	}
	if c.RateLimiter.PerKeyRequestsPerSecond == 0 {
		c.RateLimiter.PerKeyRequestsPerSecond = 60
	}
	if c.RateLimiter.PerKeyBurst == 0 {
		c.RateLimiter.PerKeyBurst = 30
	}
	if c.CircuitBreaker.FailureThreshold == 0 {
		c.CircuitBreaker.FailureThreshold = 5
	}
	if c.CircuitBreaker.ResetTimeout == 0 {
		c.CircuitBreaker.ResetTimeout = 30 * time.Second
	}
	if c.CircuitBreaker.HalfOpenMax == 0 {
		c.CircuitBreaker.HalfOpenMax = 3
	}
	if c.LoadShedding.MaxInFlight == 0 {
		c.LoadShedding.MaxInFlight = 500
	}
	if c.LoadShedding.RetryAfterSeconds == 0 {
		c.LoadShedding.RetryAfterSeconds = 5
	}
}
