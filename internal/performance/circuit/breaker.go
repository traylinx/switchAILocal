package circuit

import (
	"errors"
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
)

var (
	// ErrCircuitOpen is returned by AllowRequest when the upstream provider
	// has tripped the failure threshold and is currently blocking requests.
	ErrCircuitOpen = errors.New("circuit breaker is open (provider overloaded)")
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Breaker implements the circuit breaker pattern for a single provider.
type Breaker struct {
	mu           sync.RWMutex
	state        State
	failures     int
	halfOpenReqs int
	lastChange   time.Time

	config *config.CircuitBreakerConfig
}

// NewBreaker creates a new circuit breaker with the given performance configuration.
func NewBreaker(cfg *config.CircuitBreakerConfig) *Breaker {
	return &Breaker{
		state:  StateClosed,
		config: cfg,
	}
}

// AllowRequest checks if a request should be allowed to proceed.
// If the circuit is open, it returns ErrCircuitOpen.
// In HalfOpen state, it allows up to config.HalfOpenMax requests through to probe the upstream.
func (cb *Breaker) AllowRequest() error {
	if !cb.config.Enabled {
		return nil
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if the reset timeout has elapsed
		if now.Sub(cb.lastChange) >= cb.config.ResetTimeout {
			cb.state = StateHalfOpen
			cb.lastChange = now
			cb.halfOpenReqs = 1 // Allow this probe request through
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		if cb.halfOpenReqs < cb.config.HalfOpenMax {
			cb.halfOpenReqs++
			return nil
		}
		// Deny if we already sent all probe requests
		return ErrCircuitOpen
	}

	return nil
}

// RecordSuccess records a successful request.
// It resets the failure counter if closed, or completes the transition to closed if half-open.
func (cb *Breaker) RecordSuccess() {
	if !cb.config.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		if cb.failures > 0 {
			cb.failures = 0
		}
	case StateHalfOpen:
		// A successful request in half-open transitions it immediately back to closed.
		cb.state = StateClosed
		cb.failures = 0
		cb.halfOpenReqs = 0
		cb.lastChange = time.Now()
	}
}

// RecordFailure records a failed request.
// It increments the failure counter if closed, or instantly re-opens the circuit if half-open.
func (cb *Breaker) RecordFailure() {
	if !cb.config.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = StateOpen
			cb.lastChange = time.Now()
		}
	case StateHalfOpen:
		// Any failure in half-open state re-opens the circuit immediately.
		cb.state = StateOpen
		cb.lastChange = time.Now()
		cb.halfOpenReqs = 0
	}
}

// State returns the current state of the circuit breaker.
func (cb *Breaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Registry manages a thread-safe map of Breakers per provider.
type Registry struct {
	mu       sync.RWMutex
	breakers map[string]*Breaker
	config   *config.CircuitBreakerConfig
}

// NewRegistry creates a new global provider Breaker registry.
func NewRegistry(cfg *config.CircuitBreakerConfig) *Registry {
	return &Registry{
		breakers: make(map[string]*Breaker),
		config:   cfg,
	}
}

// Get returns the circuit breaker for the given provider, creating it implicitly if missing.
func (r *Registry) Get(provider string) *Breaker {
	r.mu.RLock()
	if b, ok := r.breakers[provider]; ok {
		r.mu.RUnlock()
		return b
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double-checked locking
	if b, ok := r.breakers[provider]; ok {
		return b
	}

	b := NewBreaker(r.config)
	r.breakers[provider] = b
	return b
}
