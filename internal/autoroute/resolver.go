package autoroute

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// ErrAutoRoutingDisabled is returned when auto-routing is not enabled in config.
var ErrAutoRoutingDisabled = errors.New("auto-routing is disabled")

// ErrNoAvailableProviders is returned when all candidate providers are unavailable.
var ErrNoAvailableProviders = errors.New("all providers are unavailable")

// ErrResolutionTimeout is returned when the resolution exceeds the configured budget.
var ErrResolutionTimeout = errors.New("auto-routing resolution exceeded timeout budget")

// RoutingRequest encapsulates everything the resolver needs to make a decision.
type RoutingRequest struct {
	// Content is the user's message text (used for complexity estimation)
	Content string

	// IntentHint is an explicit intent from the model name (e.g., "auto:coding" → "coding")
	IntentHint string

	// AvailableModels is the list of candidate model IDs currently registered
	AvailableModels []CandidateInput
}

// AutoResolver orchestrates the full routing decision pipeline.
type AutoResolver struct {
	config       Config
	scorer       *ProviderScorer
	candidates   []CandidateInput
	candidatesMu sync.RWMutex // Protects parallel updates from Discovery/Health phases
	monitor      *ProviderHealthMonitor
	lab          *Lab
}

// NewAutoResolver creates a new resolver with the given configuration.
// workspaceDir is used for persistent storage (journal TSV files).
func NewAutoResolver(cfg Config, workspaceDir string) *AutoResolver {
	r := &AutoResolver{
		config: cfg,
		scorer: NewProviderScorer(cfg),
	}
	r.monitor = NewProviderHealthMonitor(r, 60*time.Second)

	if cfg.Lab.Enabled {
		journal, err := NewExperimentJournal(workspaceDir, true)
		if err != nil {
			log.WithError(err).Warn("Failed to create experiment journal, Lab telemetry will be in-memory only")
			journal = &ExperimentJournal{
				enabled:   true,
				maxRecent: 100,
				recent:    make([]JournalEntry, 100),
			}
		}
		r.lab = NewLab(cfg, r.scorer, journal)
	}

	return r
}

// SeedCandidates allows the DiscoveryService to inject real-time provider data
// and registers them with the health monitor for initial state tracking.
func (r *AutoResolver) SeedCandidates(candidates []CandidateInput) {
	r.candidatesMu.Lock()
	r.candidates = candidates
	r.candidatesMu.Unlock()

	if r.monitor != nil {
		r.monitor.RegisterInitialCandidates(candidates)
	}
}

// updateCandidatesDirectly updates the candidate list without re-registering
// with the health monitor. Used by the monitor's syncToResolverLocked to
// avoid the circular call deadlock (C1 fix).
func (r *AutoResolver) updateCandidatesDirectly(candidates []CandidateInput) {
	r.candidatesMu.Lock()
	r.candidates = candidates
	r.candidatesMu.Unlock()
}

// GetCandidates returns the current live state of providers
func (r *AutoResolver) GetCandidates() []CandidateInput {
	r.candidatesMu.RLock()
	defer r.candidatesMu.RUnlock()
	return r.candidates
}

// StartMonitor begins background health checks
func (r *AutoResolver) StartMonitor(ctx context.Context) {
	if r.monitor != nil {
		r.monitor.Start(ctx)
	}
}

// RecordOutcome allows the proxy handler to passively report request statistics
// and triggers the Lab optimization cycle.
func (r *AutoResolver) RecordOutcome(reqID string, decision *RoutingDecision, provider string, latency time.Duration, success bool, httpCode int, headers http.Header) {
	if r.monitor != nil {
		r.monitor.RecordRequestOutcome(provider, latency, success, httpCode, headers)
	}

	if r.lab != nil && decision != nil {
		outcome := RequestOutcome{
			Timestamp:           time.Now(),
			Model:               decision.SelectedModel,
			Provider:            provider,
			Tier:                GetEffectiveTier(decision.SelectedModel, provider, r.config),
			Latency:             latency,
			Success:             success,
			StatusCode:          httpCode,
			EstimatedComplexity: decision.EstimatedComplexity,
		}
		r.lab.RecordOutcome(reqID, decision.Intent, decision.EstimatedComplexity, decision, outcome)
	}
}

// Resolve executes the full routing pipeline:
//  1. Validate config (enabled?)
//  2. Estimate complexity from content
//  3. Filter by intent (if applicable)
//  4. Score all candidates
//  5. Select winner + build fallback chain
//
// The entire operation is budgeted to cfg.MaxResolution (default 5ms).
func (r *AutoResolver) Resolve(ctx context.Context, req *RoutingRequest) (*RoutingDecision, error) {
	start := time.Now()

	// Feature flag check
	if !r.config.Enabled {
		return nil, ErrAutoRoutingDisabled
	}

	// Apply timeout budget
	deadline := start.Add(r.config.MaxResolution)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Step 1: Estimate complexity
	complexity := EstimateComplexity(req.Content)

	// Step 2: Classify intent (Phase G)
	// Uses explicit hint if provided, otherwise runs fast heuristic classification
	classification := ClassifyIntent(req.Content, req.IntentHint)
	resolvedIntent := classification.Intent

	// Step 3: Filter by intent (if intent matrix is configured)
	candidates := req.AvailableModels
	if resolvedIntent != "" && resolvedIntent != IntentGeneral && len(r.config.IntentMatrix) > 0 {
		candidates = filterByIntent(resolvedIntent, candidates, r.config.IntentMatrix)
	}

	// Step 3: Check for timeout before scoring
	select {
	case <-ctx.Done():
		return nil, ErrResolutionTimeout
	default:
	}

	// Step 4: Score all candidates
	scored := r.scorer.ScoreAll(candidates, complexity)

	if len(scored) == 0 {
		return nil, ErrNoAvailableProviders
	}

	// Step 5: Find first available candidate
	winner := -1
	for i, s := range scored {
		if s.Available {
			winner = i
			break
		}
	}

	if winner < 0 {
		return nil, ErrNoAvailableProviders
	}

	// Step 6: Build fallback chain from remaining available candidates
	var fallbacks []FallbackEntry
	for i := winner + 1; i < len(scored); i++ {
		if scored[i].Available {
			fallbacks = append(fallbacks, FallbackEntry{
				Provider: scored[i].Provider,
				Model:    scored[i].Model,
				Tier:     scored[i].EffectiveTier,
			})
		}
	}

	decision := &RoutingDecision{
		SelectedModel:       scored[winner].Model,
		FallbackChain:       fallbacks,
		Intent:              resolvedIntent,
		EstimatedComplexity: complexity,
		Candidates:          scored,
		OriginalInputs:      candidates,
		ResolutionLatency:   time.Since(start),
	}

	return decision, nil
}

// filterByIntent returns only candidates whose model IDs appear in the intent matrix.
// Falls back to the full candidate list if no matches are found.
func filterByIntent(intent string, candidates []CandidateInput, matrix map[string][]string) []CandidateInput {
	intentModels, ok := matrix[intent]
	if !ok || len(intentModels) == 0 {
		return candidates // Unknown intent → no filtering
	}

	// Build a fast lookup set
	modelSet := make(map[string]struct{}, len(intentModels))
	for _, m := range intentModels {
		modelSet[m] = struct{}{}
	}

	var filtered []CandidateInput
	for _, c := range candidates {
		if _, ok := modelSet[c.Model]; ok {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return candidates // No matches → fall back to all
	}

	return filtered
}

// ParseAutoModelHint extracts the intent hint from a model name like "auto:coding".
// Returns ("auto", "") for plain "auto", ("auto", "coding") for "auto:coding".
func ParseAutoModelHint(model string) (base string, hint string) {
	if model == "auto" || model == "" {
		return "auto", ""
	}

	const prefix = "auto:"
	if len(model) > len(prefix) && model[:len(prefix)] == prefix {
		return "auto", model[len(prefix):]
	}

	return model, ""
}

// String returns a human-readable summary of the routing decision.
func (d *RoutingDecision) String() string {
	fb := len(d.FallbackChain)
	return fmt.Sprintf("model=%s intent=%s complexity=%.2f fallbacks=%d latency=%v",
		d.SelectedModel, d.Intent, d.EstimatedComplexity, fb, d.ResolutionLatency)
}

// GetLabStatus returns the current live telemetry from the autonomous experiment engine.
func (r *AutoResolver) GetLabStatus() *LabStatus {
	if r.lab == nil {
		return nil
	}
	status := r.lab.GetStatus()
	return &status
}

// GetRecentJournal returns the most recent routing decisions logged by the lab.
func (r *AutoResolver) GetRecentJournal(n int) []JournalEntry {
	if r.lab == nil || r.lab.journal == nil {
		return nil
	}
	return r.lab.journal.GetRecent(n)
}

// Shutdown gracefully stops the monitor, lab, and closes the journal file.
func (r *AutoResolver) Shutdown() {
	if r.monitor != nil {
		r.monitor.Stop()
	}
	if r.lab != nil {
		r.lab.Stop()
	}
}
