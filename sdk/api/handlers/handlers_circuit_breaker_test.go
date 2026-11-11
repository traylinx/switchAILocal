package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/performance/circuit"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

func TestCircuitBreaker_PipelineIntegration(t *testing.T) {
	sdkCfg := &sdkconfig.SDKConfig{}

	cfg := &config.CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMax:      1,
	}
	breaker := circuit.NewRegistry(cfg)

	manager := coreauth.NewManager(nil, nil, nil)

	h := NewBaseAPIHandlers(sdkCfg, manager, nil, nil, nil, breaker)

	providers := []string{"fail_provider1", "fail_provider2"}

	// 1. All providers initially healthy
	allowed, err := h.enforceCircuitBreaker(providers)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(allowed) != 2 {
		t.Fatalf("expected 2 providers allowed, got %v", len(allowed))
	}

	// 2. Trip one provider
	breaker.Get("fail_provider1").RecordFailure()
	breaker.Get("fail_provider1").RecordFailure() // Trips

	allowed, err = h.enforceCircuitBreaker(providers)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(allowed) != 1 || allowed[0] != "fail_provider2" {
		t.Fatalf("expected fail_provider2, got %v", allowed)
	}

	// 3. Trip second provider
	breaker.Get("fail_provider2").RecordFailure()
	breaker.Get("fail_provider2").RecordFailure() // Trips

	allowed, err = h.enforceCircuitBreaker(providers)
	if err == nil {
		t.Fatalf("expected error when all circuits are open")
	}
	if err.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %v", err.StatusCode)
	}
	if len(allowed) != 0 {
		t.Fatalf("expected 0 allowed providers, got %v", len(allowed))
	}

	// 4. Wait for reset timeout and test probes
	time.Sleep(150 * time.Millisecond)
	allowed, err = h.enforceCircuitBreaker(providers)
	if err != nil {
		t.Fatalf("expected nil err after reset timeout, got %v", err)
	}
	if len(allowed) != 2 {
		t.Fatalf("expected 2 probes allowed, got %v", len(allowed))
	}
}
