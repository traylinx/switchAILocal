// Package metadata provides functionality for aggregating and managing healing metadata
// throughout the request lifecycle. It tracks all autonomous actions taken by the Superbrain
// system and provides methods for recording and retrieving this information.
package metadata

import (
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// Aggregator manages the collection of healing actions and metadata for a request.
// It provides thread-safe methods for recording actions and building the final metadata.
type Aggregator struct {
	mu                 sync.RWMutex
	requestID          string
	originalProvider   string
	finalProvider      string
	startTime          time.Time
	actions            []types.HealingAction
	contextOptimized   bool
	highDensityMap     *types.HighDensityMap
	diagnosisHistory   []*types.Diagnosis
}

// NewAggregator creates a new metadata aggregator for a request.
func NewAggregator(requestID, originalProvider string) *Aggregator {
	return &Aggregator{
		requestID:        requestID,
		originalProvider: originalProvider,
		finalProvider:    originalProvider, // Initially same as original
		startTime:        time.Now(),
		actions:          make([]types.HealingAction, 0),
		diagnosisHistory: make([]*types.Diagnosis, 0),
	}
}

// RecordAction adds a healing action to the aggregator.
// This method is thread-safe and can be called from multiple goroutines.
func (a *Aggregator) RecordAction(actionType, description string, success bool, details map[string]interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	action := types.HealingAction{
		Timestamp:   time.Now(),
		ActionType:  actionType,
		Description: description,
		Success:     success,
		Details:     details,
	}

	a.actions = append(a.actions, action)
}

// RecordDiagnosis adds a diagnosis to the history.
// This method is thread-safe and can be called from multiple goroutines.
func (a *Aggregator) RecordDiagnosis(diagnosis *types.Diagnosis) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.diagnosisHistory = append(a.diagnosisHistory, diagnosis)
}

// SetFinalProvider updates the final provider that fulfilled the request.
// This is typically called after a successful fallback routing.
func (a *Aggregator) SetFinalProvider(provider string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.finalProvider = provider
}

// SetContextOptimization records that context optimization was performed.
// The high-density map provides details about what content was excluded.
func (a *Aggregator) SetContextOptimization(highDensityMap *types.HighDensityMap) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.contextOptimized = true
	a.highDensityMap = highDensityMap
}

// GetMetadata builds and returns the complete healing metadata.
// This method is thread-safe and can be called at any time.
func (a *Aggregator) GetMetadata() *types.HealingMetadata {
	a.mu.RLock()
	defer a.mu.RUnlock()

	totalHealingTime := time.Since(a.startTime).Milliseconds()

	// Create a copy of actions to avoid race conditions
	actionsCopy := make([]types.HealingAction, len(a.actions))
	copy(actionsCopy, a.actions)

	// Create a copy of diagnosis history
	diagnosisCopy := make([]*types.Diagnosis, len(a.diagnosisHistory))
	copy(diagnosisCopy, a.diagnosisHistory)

	return &types.HealingMetadata{
		RequestID:          a.requestID,
		OriginalProvider:   a.originalProvider,
		FinalProvider:      a.finalProvider,
		TotalHealingTimeMs: totalHealingTime,
		Actions:            actionsCopy,
		ContextOptimized:   a.contextOptimized,
		HighDensityMap:     a.highDensityMap,
		DiagnosisHistory:   diagnosisCopy,
	}
}

// HasActions returns true if any healing actions have been recorded.
func (a *Aggregator) HasActions() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return len(a.actions) > 0
}

// GetActionCount returns the number of healing actions recorded.
func (a *Aggregator) GetActionCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return len(a.actions)
}

// GetOriginalProvider returns the original provider for the request.
func (a *Aggregator) GetOriginalProvider() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.originalProvider
}

// GetFinalProvider returns the final provider that fulfilled the request.
func (a *Aggregator) GetFinalProvider() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.finalProvider
}

// WasProviderChanged returns true if the final provider differs from the original.
func (a *Aggregator) WasProviderChanged() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.originalProvider != a.finalProvider
}
