package circuit_test

import (
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/performance/circuit"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cfg := &config.CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMax:      1,
	}

	cb := circuit.NewBreaker(cfg)

	// verify closed state initially
	if cb.State() != circuit.StateClosed {
		t.Fatalf("expected initial state closed, got %v", cb.State())
	}
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("expected request allowed in closed state")
	}

	// record failures to trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != circuit.StateClosed {
		t.Fatalf("expected state to remain closed until threshold, got %v", cb.State())
	}

	cb.RecordFailure() // 3rd failure trips the breaker
	if cb.State() != circuit.StateOpen {
		t.Fatalf("expected state to be open, got %v", cb.State())
	}

	if err := cb.AllowRequest(); err != circuit.ErrCircuitOpen {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// the next request should be allowed (half-open)
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("expected probe request to be allowed in half-open state, got %v", err)
	}
	if cb.State() != circuit.StateHalfOpen {
		t.Fatalf("expected state to be half-open, got %v", cb.State())
	}

	// second probe should be rejected because HalfOpenMax = 1
	if err := cb.AllowRequest(); err != circuit.ErrCircuitOpen {
		t.Fatalf("expected second probe to be rejected via ErrCircuitOpen, got %v", err)
	}

	// simulate successful probe returning
	cb.RecordSuccess()
	if cb.State() != circuit.StateClosed {
		t.Fatalf("expected state to return to closed after successful probe, got %v", cb.State())
	}

	// test failure from half-open goes back to open instantly
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()                 // trip again
	time.Sleep(150 * time.Millisecond) // wait
	_ = cb.AllowRequest()              // transition to half-open
	cb.RecordFailure()                 // fail the probe
	if cb.State() != circuit.StateOpen {
		t.Fatalf("expected state to return to open after failed probe, got %v", cb.State())
	}
}

func TestCircuitBreaker_Disabled(t *testing.T) {
	cfg := &config.CircuitBreakerConfig{
		Enabled:          false,
		FailureThreshold: 1,
	}

	cb := circuit.NewBreaker(cfg)
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != circuit.StateClosed {
		t.Fatalf("expected state to remain closed when disabled")
	}
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("expected request allowed when disabled")
	}
}
