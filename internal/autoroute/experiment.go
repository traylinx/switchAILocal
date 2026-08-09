package autoroute

import (
	"context"
	"math/rand"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// DefaultAltSuccessRQS is the optimistic RQS estimate assigned when shadow
// selections route away from a failing production provider. It represents a
// conservative-but-hopeful guess that the alternative model would have
// succeeded (similar to prod success RQS without the 0.40 success penalty).
const DefaultAltSuccessRQS = 0.85

// Lab orchestrates the autonomous self-optimization loop for routing weights.
// It directly implements the "fixed-budget, single-metric, keep-or-discard"
// loop inspired by AutoResearch.
type Lab struct {
	config  Config
	scorer  *ProviderScorer
	journal *ExperimentJournal
	rng     *rand.Rand

	// The current hypothesis weights being evaluated in the shadow
	mu               sync.RWMutex
	shadowWeights    ScoringWeights
	activeHypothesis bool

	// Metric accumulation for the current observation window
	windowProdRQS       float64
	windowShadowRQS     float64
	windowReqCount      int
	windowExploredCount int // times shadow picked a different model

	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewLab initializes the self-optimizing research loop.
func NewLab(cfg Config, scorer *ProviderScorer, journal *ExperimentJournal) *Lab {
	lab := &Lab{
		config:  cfg,
		scorer:  scorer,
		journal: journal,
		stopCh:  make(chan struct{}),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Initialize shadow weights to exactly match production initially
	lab.shadowWeights = cfg.Weights

	return lab
}

// Start begins the background adaptation loop.
func (l *Lab) Start(ctx context.Context) {
	if !l.config.Lab.Enabled {
		log.Info("Autoroute Lab (Self-Optimization) is disabled")
		return
	}
	log.WithField("interval", l.config.Lab.AdaptationInterval).
		Info("Starting Autoroute Lab (Self-Optimization Loop)")
	go l.loop(ctx)
}

// Stop halts the background loop and closes the journal. Safe to call multiple times.
func (l *Lab) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
		if l.journal != nil {
			if err := l.journal.Close(); err != nil {
				log.WithError(err).Warn("Failed to close experiment journal")
			}
		}
	})
}

// LabStatus provides a snapshot of the current optimization experiment state for the UI.
type LabStatus struct {
	Enabled             bool           `json:"enabled"`
	ActiveWeights       ScoringWeights `json:"active_weights"`
	ShadowWeights       ScoringWeights `json:"shadow_weights"`
	ActiveHypothesis    bool           `json:"active_hypothesis"`
	WindowReqCount      int            `json:"window_req_count"`
	WindowExploredCount int            `json:"window_explored_count"`
	AvgProdRQS          float64        `json:"avg_prod_rqs"`
	AvgShadowRQS        float64        `json:"avg_shadow_rqs"`
}

// GetStatus returns the current live telemetry from the lab.
func (l *Lab) GetStatus() LabStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()

	status := LabStatus{
		Enabled:             l.config.Lab.Enabled,
		ActiveWeights:       l.scorer.GetWeights(),
		ShadowWeights:       l.shadowWeights,
		ActiveHypothesis:    l.activeHypothesis,
		WindowReqCount:      l.windowReqCount,
		WindowExploredCount: l.windowExploredCount,
	}

	if l.windowReqCount > 0 {
		status.AvgProdRQS = l.windowProdRQS / float64(l.windowReqCount)
		status.AvgShadowRQS = l.windowShadowRQS / float64(l.windowReqCount)
	}

	return status
}

// RecordOutcome is called after every request to log the real-world performance
// and compare it against the shadow prediction.
func (l *Lab) RecordOutcome(reqID string, intent string, complexity float64, prodDecision *RoutingDecision, prodOutcome RequestOutcome) {
	if !l.config.Lab.Enabled {
		return
	}

	prodRQS := CalculateRQS(prodOutcome, DefaultRQSWeights)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.windowProdRQS += prodRQS
	l.windowReqCount++

	// 1. Shadow Scoring: What WOULD we have chosen with the experimental weights?
	shadowScorer := &ProviderScorer{
		weights:      l.shadowWeights,
		preferences:  l.config.Preferences,
		conservation: l.config.Conservation,
		config:       l.config,
	}

	// Re-score using the exact same CandidateInputs that were available at decision time (H1 fix).
	// Falls back to reconstructing from ScoredCandidates only if OriginalInputs is empty (old decisions).
	candidates := prodDecision.OriginalInputs
	if len(candidates) == 0 {
		candidates = make([]CandidateInput, 0, len(prodDecision.Candidates))
		for _, sc := range prodDecision.Candidates {
			candidates = append(candidates, CandidateInput{
				Model:       sc.Model,
				Provider:    sc.Provider,
				Available:   sc.Available,
				Latency:     sc.EstimatedLatency,
				QuotaHealth: 1.0,
				SuccessRate: 0.9,
			})
		}
	}

	shadowScored := shadowScorer.ScoreAll(candidates, complexity)

	// Log the journal entry
	entry := JournalEntry{
		Timestamp:     time.Now(),
		RequestID:     reqID,
		Intent:        intent,
		Complexity:    complexity,
		ProdModel:     prodOutcome.Model,
		ProdTier:      prodOutcome.Tier,
		ProdLatency:   prodOutcome.Latency,
		ProdSuccess:   prodOutcome.Success,
		ProdRQS:       prodRQS,
		WeightAvail:   l.scorer.weights.Availability,
		WeightQuota:   l.scorer.weights.Quota,
		WeightLatency: l.scorer.weights.Latency,
		WeightSuccess: l.scorer.weights.SuccessRate,
	}

	if len(shadowScored) > 0 && shadowScored[0].Available {
		entry.ShadowModel = shadowScored[0].Model
		entry.ShadowTier = shadowScored[0].EffectiveTier

		if entry.ShadowModel == entry.ProdModel {
			entry.ShadowExpectedRQS = prodRQS
			l.windowShadowRQS += prodRQS
		} else {
			l.windowExploredCount++
			if prodOutcome.Success {
				// Conservative estimate: assume the alternative would have been similar.
				entry.ShadowExpectedRQS = prodRQS
				l.windowShadowRQS += prodRQS
			} else {
				// Production failed; shadow's different pick could have succeeded.
				// Optimistic estimate: the alternative model would have performed well.
				entry.ShadowExpectedRQS = DefaultAltSuccessRQS
				l.windowShadowRQS += DefaultAltSuccessRQS
			}
		}
	}

	if l.journal != nil {
		l.journal.Record(entry)
	}
}

// loop runs the evaluate -> keep/discard -> generate hypothesis cycle.
func (l *Lab) loop(ctx context.Context) {
	ticker := time.NewTicker(l.config.Lab.AdaptationInterval)
	defer ticker.Stop()

	// Initial hypothesis
	l.generateHypothesis()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.evaluateAndAdapt()
		}
	}
}

// evaluateAndAdapt compares the window metrics and decides whether to keep or discard the shadow weights.
func (l *Lab) evaluateAndAdapt() {
	l.mu.Lock()
	defer l.mu.Unlock()

	minObs := l.config.Lab.MinObservationWindow
	if minObs <= 0 {
		minObs = 10
	}

	if l.windowReqCount < minObs {
		log.WithFields(log.Fields{
			"reqs":   l.windowReqCount,
			"needed": minObs,
		}).Debug("Insufficient data in Lab window, extending observation")
		return
	}

	avgProdRQS := l.windowProdRQS / float64(l.windowReqCount)
	avgShadowRQS := l.windowShadowRQS / float64(l.windowReqCount)

	log.WithFields(log.Fields{
		"reqs":      l.windowReqCount,
		"prodRQS":   avgProdRQS,
		"shadowRQS": avgShadowRQS,
		"explored":  l.windowExploredCount,
	}).Info("Auto-routing lab evaluated experiment window")

	promoted := false

	// Criterion 1: Shadow RQS is 5%+ better (e.g., avoided production failures).
	if avgShadowRQS > avgProdRQS*1.05 {
		log.WithField("improvement", avgShadowRQS-avgProdRQS).
			Info("Lab promoted shadow weights (RQS improvement)")
		promoted = true
	}

	// Criterion 2: Shadow explored different routes (picked different models
	// at least once) while maintaining RQS parity. This means the weights
	// discovered alternative routing paths without quality degradation.
	if !promoted && l.windowExploredCount > 0 && avgShadowRQS >= avgProdRQS {
		log.WithFields(log.Fields{
			"explored":  l.windowExploredCount,
			"prodRQS":   avgProdRQS,
			"shadowRQS": avgShadowRQS,
		}).Info("Lab promoted shadow weights (exploration without degradation)")
		promoted = true
	}

	if promoted {
		l.scorer.SetWeights(l.shadowWeights)
	} else {
		log.WithField("diff", avgShadowRQS-avgProdRQS).
			Debug("Lab discarded shadow weights (no significant improvement)")
	}

	// Reset window and formulate a new hypothesis
	l.windowProdRQS = 0
	l.windowShadowRQS = 0
	l.windowReqCount = 0
	l.windowExploredCount = 0

	l.generateHypothesisLocked()
}

// generateHypothesis perturbs the current production weights slightly to explore the gradient.
func (l *Lab) generateHypothesis() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.generateHypothesisLocked()
}

func (l *Lab) generateHypothesisLocked() {
	// Start from current production weights
	curr := l.scorer.GetWeights()
	drift := l.config.Lab.MaxWeightDrift
	if drift <= 0 {
		drift = 0.05
	}

	// Add random noise [-drift/2, +drift/2] to each weight
	w := ScoringWeights{
		Availability: curr.Availability + (l.rng.Float64()-0.5)*drift,
		Quota:        curr.Quota + (l.rng.Float64()-0.5)*drift,
		Latency:      curr.Latency + (l.rng.Float64()-0.5)*drift,
		SuccessRate:  curr.SuccessRate + (l.rng.Float64()-0.5)*drift,
	}

	// Clamp limits 0.05 min
	w.Availability = max(0.05, w.Availability)
	w.Quota = max(0.05, w.Quota)
	w.Latency = max(0.05, w.Latency)
	w.SuccessRate = max(0.05, w.SuccessRate)

	// Normalize to sum to 1.0
	sum := w.Availability + w.Quota + w.Latency + w.SuccessRate
	w.Availability /= sum
	w.Quota /= sum
	w.Latency /= sum
	w.SuccessRate /= sum

	l.shadowWeights = w
	l.activeHypothesis = true

	log.WithFields(log.Fields{
		"Avail":   w.Availability,
		"Quota":   w.Quota,
		"Latency": w.Latency,
		"Success": w.SuccessRate,
	}).Debug("Lab formulated new shadow hypothesis")
}
