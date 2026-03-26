package autoroute

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// CandidateInput represents the raw health data for a single model candidate.
// These values are provided by external health monitors, registries, and stats trackers.
type CandidateInput struct {
	Model       string        // Full model ID (e.g., "geminicli:gemini-3.1-pro")
	Provider    string        // Provider name (e.g., "geminicli")
	Available   bool          // Is the provider reachable and has active credentials?
	QuotaHealth float64       // 0.0 (exhausted) to 1.0 (full), based on active/cooled credentials
	Latency     time.Duration // Average observed latency (0 = unknown)
	SuccessRate float64       // 0.0 to 1.0 historical success rate (-1 = unknown/cold start)
}

// ProviderScorer implements the composite scoring algorithm.
type ProviderScorer struct {
	mu           sync.RWMutex
	weights      ScoringWeights
	preferences  []ModelPreference
	conservation ConservationConfig
	config       Config
}

// NewProviderScorer creates a scorer with the given configuration.
func NewProviderScorer(cfg Config) *ProviderScorer {
	return &ProviderScorer{
		weights:      cfg.Weights,
		preferences:  cfg.Preferences,
		conservation: cfg.Conservation,
		config:       cfg,
	}
}

// GetWeights returns a snapshot of the current scoring weights (thread-safe).
func (s *ProviderScorer) GetWeights() ScoringWeights {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.weights
}

// SetWeights atomically replaces the scoring weights (thread-safe).
func (s *ProviderScorer) SetWeights(w ScoringWeights) {
	s.mu.Lock()
	s.weights = w
	s.mu.Unlock()
}

// ScoreAll evaluates and sorts all candidates, returning scored results.
// The complexity parameter (0.0-1.0) drives the conservation multiplier.
func (s *ProviderScorer) ScoreAll(candidates []CandidateInput, complexity float64) []ScoredCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// Snapshot weights under lock for consistent scoring across all candidates
	s.mu.RLock()
	weightsSnap := s.weights
	s.mu.RUnlock()

	scored := make([]ScoredCandidate, 0, len(candidates))

	for _, c := range candidates {
		sc := s.scoreOne(c, complexity, weightsSnap)
		scored = append(scored, sc)
	}

	// Sort by FinalScore descending, then apply tie-breaking
	const epsilon = 1e-9
	sort.SliceStable(scored, func(i, j int) bool {
		if math.Abs(scored[i].FinalScore-scored[j].FinalScore) > epsilon {
			return scored[i].FinalScore > scored[j].FinalScore
		}
		// Tie-break 1: lower latency wins
		if scored[i].EstimatedLatency != scored[j].EstimatedLatency {
			return scored[i].EstimatedLatency < scored[j].EstimatedLatency
		}
		// Tie-break 2: alphabetical for determinism
		return scored[i].Model < scored[j].Model
	})

	return scored
}

// scoreOne computes the full composite score for a single candidate.
// Uses the provided weightsSnap for consistent scoring across all candidates
// in a single ScoreAll pass.
//
// Pipeline:
//
//	BaseScore = W_a × Availability + W_q × QuotaHealth + W_l × Latency + W_s × SuccessRate
//	TierScore = BaseScore + TierBoost(provider)
//	PrefScore = TierScore + PreferenceBoost(model)
//	FinalScore = PrefScore × ConservationMultiplier(tier, complexity)
func (s *ProviderScorer) scoreOne(c CandidateInput, complexity float64, w ScoringWeights) ScoredCandidate {
	// 1. Availability (binary gate: 0.0 or 1.0)
	avail := 0.0
	if c.Available {
		avail = 1.0
	}

	// 2. Quota health (already 0.0-1.0)
	quota := c.QuotaHealth

	// 3. Latency score (inverse normalization: 0ms=1.0, 5000ms=0.0)
	latencyScore := latencyToScore(c.Latency)

	// 4. Success rate (cold start: 0.7 optimistic default)
	successRate := c.SuccessRate
	if successRate < 0 {
		successRate = 0.7 // Cold start: optimistic default
	}

	// 5. Base score
	base := w.Availability*avail +
		w.Quota*quota +
		w.Latency*latencyScore +
		w.SuccessRate*successRate

	// 6. Tier resolution and boost
	tier := GetEffectiveTier(c.Model, c.Provider, s.config)
	tierB := TierBoost(tier)

	// 7. Preference boost
	prefB := PreferenceBoost(c.Model, s.preferences)

	// 8. Conservation multiplier
	conservMult := ConservationMultiplier(tier, complexity, s.conservation)

	// 9. Final score
	final := (base + tierB + prefB) * conservMult

	return ScoredCandidate{
		Model:            c.Model,
		Provider:         c.Provider,
		FinalScore:       final,
		BaseScore:        base,
		TierBoost:        tierB,
		PreferenceBoost:  prefB,
		ConservationMult: conservMult,
		EffectiveTier:    tier,
		Available:        c.Available,
		EstimatedLatency: c.Latency,
	}
}

// latencyToScore converts a latency duration to a 0.0-1.0 score.
// 0ms → 1.0, 5000ms → 0.0, linear decay. Unknown (0) → 0.5 (neutral).
func latencyToScore(d time.Duration) float64 {
	if d == 0 {
		return 0.5 // Unknown latency → neutral
	}
	ms := d.Milliseconds()
	const maxAcceptable = 5000
	if ms >= maxAcceptable {
		return 0.0
	}
	return 1.0 - float64(ms)/float64(maxAcceptable)
}

// EstimateComplexity provides a fast heuristic complexity estimate from content length.
// Uses the len(content)/4 token approximation (~100ns, well within 5ms budget).
func EstimateComplexity(content string) float64 {
	tokens := len(content) / 4
	if tokens <= 0 {
		return 0.1 // Minimal input → trivial
	}

	// Scale: 0-50 tokens = 0.1, 50-200 = 0.3, 200-500 = 0.5, 500-2000 = 0.7, 2000+ = 0.9
	switch {
	case tokens < 50:
		return 0.1
	case tokens < 200:
		return 0.3
	case tokens < 500:
		return 0.5
	case tokens < 2000:
		return 0.7
	default:
		return 0.9
	}
}

// ExtractProvider extracts the provider portion from a "provider:model" string.
func ExtractProvider(modelID string) string {
	if idx := strings.Index(modelID, ":"); idx > 0 {
		return modelID[:idx]
	}
	return modelID
}
