// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package verification

import (
	"sync"
)

// Verifier handles the consensus check between different routing tiers.
type Verifier struct {
	mu sync.RWMutex

	// Metrics
	totalVerifications int
	agreementCount     int
	mismatchCount      int
}

// NewVerifier creates a new Verifier instance.
func NewVerifier() *Verifier {
	return &Verifier{}
}

// Verify compares two intents and records the outcome.
// Returns true if there is a consensus (intents match).
func (v *Verifier) Verify(tier1Intent, tier2Intent string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.totalVerifications++

	match := tier1Intent == tier2Intent
	if match {
		v.agreementCount++
	} else {
		v.mismatchCount++
	}

	return match
}

// GetMetrics returns verification statistics.
func (v *Verifier) GetMetrics() map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()

	agreementRate := 0.0
	if v.totalVerifications > 0 {
		agreementRate = float64(v.agreementCount) / float64(v.totalVerifications)
	}

	return map[string]interface{}{
		"total_verifications": v.totalVerifications,
		"agreement_count":     v.agreementCount,
		"mismatch_count":      v.mismatchCount,
		"agreement_rate":      agreementRate,
	}
}
