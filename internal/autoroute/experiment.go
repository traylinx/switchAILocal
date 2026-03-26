package autoroute

import (
	"context"
	"math/rand"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Lab orchestrates the autonomous self-optimization loop for routing weights.
// It directly implements the "fixed-budget, single-metric, keep-or-discard"
// loop inspired by AutoResearch.
type Lab struct {
	config       Config
	scorer       *ProviderScorer
	journal      *ExperimentJournal
	rng          *rand.Rand
	
	// The current hypothesis weights being evaluated in the shadow
	mu              sync.RWMutex
	shadowWeights   ScoringWeights
	activeHypothesis bool

	// Metric accumulation for the current observation window
	windowProdRQS   float64
	windowShadowRQS float64
	windowReqCount  int

	stopCh chan struct{}
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
	Enabled          bool           `json:"enabled"`
	ActiveWeights    ScoringWeights `json:"active_weights"`
	ShadowWeights    ScoringWeights `json:"shadow_weights"`
	ActiveHypothesis bool           `json:"active_hypothesis"`
	WindowReqCount   int            `json:"window_req_count"`
	AvgProdRQS       float64        `json:"avg_prod_rqs"`
	AvgShadowRQS     float64        `json:"avg_shadow_rqs"`
}

// GetStatus returns the current live telemetry from the lab.
func (l *Lab) GetStatus() LabStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()

	status := LabStatus{
		Enabled:          l.config.Lab.Enabled,
		ActiveWeights:    l.scorer.GetWeights(),
		ShadowWeights:    l.shadowWeights,
		ActiveHypothesis: l.activeHypothesis,
		WindowReqCount:   l.windowReqCount,
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
		Timestamp:   time.Now(),
		RequestID:   reqID,
		Intent:      intent,
		Complexity:  complexity,
		ProdModel:   prodOutcome.Model,
		ProdTier:    prodOutcome.Tier,
		ProdLatency: prodOutcome.Latency,
		ProdSuccess: prodOutcome.Success,
		ProdRQS:     prodRQS,
		WeightAvail:   l.scorer.weights.Availability,
		WeightQuota:   l.scorer.weights.Quota,
		WeightLatency: l.scorer.weights.Latency,
		WeightSuccess: l.scorer.weights.SuccessRate,
	}

	if len(shadowScored) > 0 && shadowScored[0].Available {
		entry.ShadowModel = shadowScored[0].Model
		entry.ShadowTier = shadowScored[0].EffectiveTier
		
		// If shadow chose the exact same model, assume the exact same RQS
		if entry.ShadowModel == entry.ProdModel {
			entry.ShadowExpectedRQS = prodRQS
			l.windowShadowRQS += prodRQS
		} else {
			// If shadow chose a different model, we can't know the exact latency/success,
			// so we estimate based on historical heuristics.
			// This is a naive estimation for the shadow.
			entry.ShadowExpectedRQS = 0.5 // Unknown predicted RQS
			l.windowShadowRQS += 0.5
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

	if l.windowReqCount < 10 {
		log.WithField("reqs", l.windowReqCount).
			Debug("Insufficient data in Lab window, extending observation")
		return // Not enough data to make a statistically sound decision
	}

	avgProdRQS := l.windowProdRQS / float64(l.windowReqCount)
	avgShadowRQS := l.windowShadowRQS / float64(l.windowReqCount)

	log.WithFields(log.Fields{
		"reqs":        l.windowReqCount,
		"prodRQS":     avgProdRQS,
		"shadowRQS":   avgShadowRQS,
	}).Info("Auto-routing lab evaluated experiment window")

	// Binary Keep/Discard decision (AutoResearch style)
	if avgShadowRQS > avgProdRQS * 1.05 { // Keep if 5% better
		log.WithField("improvement", avgShadowRQS - avgProdRQS).
			Info("Lab promoted shadow weights to production")
		
		// Promote via atomic swap (C2 fix: safe concurrent read/write)
		l.scorer.SetWeights(l.shadowWeights)
	} else {
		log.WithField("diff", avgShadowRQS - avgProdRQS).
			Debug("Lab discarded shadow weights (no significant improvement)")
	}

	// Reset window and formulate a new hypothesis
	l.windowProdRQS = 0
	l.windowShadowRQS = 0
	l.windowReqCount = 0
	
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
		Availability: curr.Availability + (l.rng.Float64() - 0.5) * drift,
		Quota:        curr.Quota + (l.rng.Float64() - 0.5) * drift,
		Latency:      curr.Latency + (l.rng.Float64() - 0.5) * drift,
		SuccessRate:  curr.SuccessRate + (l.rng.Float64() - 0.5) * drift,
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
