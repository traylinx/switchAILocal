package router

import (
	"context"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
)

func TestFallbackRouter_GetFallback_CapabilityMatching(t *testing.T) {
	tests := []struct {
		name         string
		requirements *RequestRequirements
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "no requirements - returns first available",
			requirements: nil,
			wantProvider: "geminicli",
			wantErr:      false,
		},
		{
			name: "requires streaming - matches streaming provider",
			requirements: &RequestRequirements{
				RequiresStream: true,
			},
			wantProvider: "geminicli",
			wantErr:      false,
		},
		{
			name: "requires CLI - matches CLI provider",
			requirements: &RequestRequirements{
				RequiresCLI: true,
			},
			wantProvider: "geminicli",
			wantErr:      false,
		},
		{
			name: "requires large context - matches provider with sufficient context",
			requirements: &RequestRequirements{
				MinContextSize: 500000,
			},
			wantProvider: "geminicli",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.FallbackConfig{
				Enabled:        true,
				Providers:      []string{"geminicli", "gemini", "ollama"},
				MinSuccessRate: 0.5,
			}
			router := NewFallbackRouter(cfg)

			decision, err := router.GetFallback(context.Background(), "claudecli", tt.requirements)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if decision.FallbackProvider != tt.wantProvider {
				t.Errorf("got provider %s, want %s", decision.FallbackProvider, tt.wantProvider)
			}
		})
	}
}

func TestFallbackRouter_GetFallback_SuccessRateSelection(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli", "gemini", "ollama"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	// Simulate failures for geminicli to lower its success rate below threshold
	for i := 0; i < 10; i++ {
		router.UpdateProviderStats("geminicli", false, 100*time.Millisecond)
	}

	// geminicli should now have 0% success rate, so gemini should be selected
	decision, err := router.GetFallback(context.Background(), "claudecli", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.FallbackProvider != "gemini" {
		t.Errorf("expected gemini (geminicli has low success rate), got %s", decision.FallbackProvider)
	}
}

func TestFallbackRouter_GetFallback_OnlyConfiguredProviders(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"gemini"}, // Only gemini is configured
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	decision, err := router.GetFallback(context.Background(), "claudecli", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only return gemini since it's the only configured provider
	if decision.FallbackProvider != "gemini" {
		t.Errorf("expected gemini (only configured provider), got %s", decision.FallbackProvider)
	}

	// Verify unconfigured providers are not selected
	if router.IsProviderConfigured("geminicli") {
		t.Error("geminicli should not be configured")
	}
	if router.IsProviderConfigured("ollama") {
		t.Error("ollama should not be configured")
	}
}

func TestFallbackRouter_GetFallback_NoSuitableProviders(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*FallbackRouter)
		errMsg string
	}{
		{
			name: "fallback disabled",
			setup: func(r *FallbackRouter) {
				// Router created with disabled config
			},
			errMsg: "no fallback provider available",
		},
		{
			name: "all providers unavailable",
			setup: func(r *FallbackRouter) {
				r.SetProviderAvailability("geminicli", false)
				r.SetProviderAvailability("gemini", false)
				r.SetProviderAvailability("ollama", false)
			},
			errMsg: "no fallback provider available",
		},
		{
			name: "all providers below success rate threshold",
			setup: func(r *FallbackRouter) {
				// Simulate many failures for all providers
				for i := 0; i < 10; i++ {
					r.UpdateProviderStats("geminicli", false, 100*time.Millisecond)
					r.UpdateProviderStats("gemini", false, 100*time.Millisecond)
					r.UpdateProviderStats("ollama", false, 100*time.Millisecond)
				}
			},
			errMsg: "no fallback provider available",
		},
		{
			name: "no provider meets capability requirements",
			setup: func(r *FallbackRouter) {
				// Requirements that no provider can meet
			},
			errMsg: "no fallback provider available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *config.FallbackConfig
			if tt.name == "fallback disabled" {
				cfg = &config.FallbackConfig{
					Enabled:        false,
					Providers:      []string{"geminicli", "gemini", "ollama"},
					MinSuccessRate: 0.5,
				}
			} else {
				cfg = &config.FallbackConfig{
					Enabled:        true,
					Providers:      []string{"geminicli", "gemini", "ollama"},
					MinSuccessRate: 0.5,
				}
			}
			router := NewFallbackRouter(cfg)

			if tt.setup != nil {
				tt.setup(router)
			}

			var requirements *RequestRequirements
			if tt.name == "no provider meets capability requirements" {
				requirements = &RequestRequirements{
					MinContextSize: 10000000, // 10M tokens - no provider has this
				}
			}

			_, err := router.GetFallback(context.Background(), "claudecli", requirements)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}

			if err != ErrNoFallbackAvailable {
				t.Errorf("expected ErrNoFallbackAvailable, got %v", err)
			}
		})
	}
}

func TestFallbackRouter_SkipsFailedProvider(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"claudecli", "geminicli", "gemini"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	// Request fallback for claudecli - should skip claudecli and return geminicli
	decision, err := router.GetFallback(context.Background(), "claudecli", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.FallbackProvider == "claudecli" {
		t.Error("fallback should not return the failed provider")
	}

	if decision.OriginalProvider != "claudecli" {
		t.Errorf("original provider should be claudecli, got %s", decision.OriginalProvider)
	}
}

func TestFallbackRouter_UpdateProviderStats(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	// Record some successes and failures
	router.UpdateProviderStats("geminicli", true, 100*time.Millisecond)
	router.UpdateProviderStats("geminicli", true, 150*time.Millisecond)
	router.UpdateProviderStats("geminicli", false, 200*time.Millisecond)

	stats := router.GetStatsTracker().GetStats("geminicli")
	if stats == nil {
		t.Fatal("expected stats for geminicli")
	}

	if stats.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", stats.TotalRequests)
	}

	if stats.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", stats.SuccessCount)
	}

	if stats.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", stats.FailureCount)
	}

	expectedRate := 2.0 / 3.0
	if stats.SuccessRate() != expectedRate {
		t.Errorf("expected success rate %f, got %f", expectedRate, stats.SuccessRate())
	}
}

func TestFallbackRouter_IsProviderAvailable(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	// Initially available
	if !router.IsProviderAvailable("geminicli") {
		t.Error("geminicli should be available initially")
	}

	// Set unavailable
	router.SetProviderAvailability("geminicli", false)
	if router.IsProviderAvailable("geminicli") {
		t.Error("geminicli should be unavailable after setting")
	}

	// Unknown provider
	if router.IsProviderAvailable("unknown") {
		t.Error("unknown provider should not be available")
	}
}

func TestFallbackRouter_IsProviderConfigured(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli", "gemini"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	if !router.IsProviderConfigured("geminicli") {
		t.Error("geminicli should be configured")
	}

	if !router.IsProviderConfigured("gemini") {
		t.Error("gemini should be configured")
	}

	if router.IsProviderConfigured("ollama") {
		t.Error("ollama should not be configured")
	}
}

func TestFallbackRouter_GetProviderCapabilities(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"geminicli"},
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	caps := router.GetProviderCapabilities()
	if len(caps) == 0 {
		t.Error("expected at least one capability")
	}

	// Verify default capabilities are present
	found := false
	for _, cap := range caps {
		if cap.Provider == "geminicli" {
			found = true
			if !cap.SupportsStream {
				t.Error("geminicli should support streaming")
			}
			if !cap.SupportsCLI {
				t.Error("geminicli should support CLI")
			}
		}
	}

	if !found {
		t.Error("geminicli capability not found")
	}
}

func TestFallbackRouter_CLIRequirement(t *testing.T) {
	cfg := &config.FallbackConfig{
		Enabled:        true,
		Providers:      []string{"gemini", "ollama"}, // Neither supports CLI
		MinSuccessRate: 0.5,
	}
	router := NewFallbackRouter(cfg)

	// Request with CLI requirement should fail since no configured provider supports CLI
	requirements := &RequestRequirements{
		RequiresCLI: true,
	}

	_, err := router.GetFallback(context.Background(), "claudecli", requirements)
	if err != ErrNoFallbackAvailable {
		t.Errorf("expected ErrNoFallbackAvailable for CLI requirement, got %v", err)
	}
}
