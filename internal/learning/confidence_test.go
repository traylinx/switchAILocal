package learning

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculatePreferenceConfidence_ZeroSamples(t *testing.T) {
	confidence := CalculatePreferenceConfidence(0, 0, 0.0)
	assert.Equal(t, 0.0, confidence, "Zero samples should return zero confidence")
}

func TestCalculatePreferenceConfidence_PerfectSmallSample(t *testing.T) {
	// 10/10 success with quality 1.0
	confidence := CalculatePreferenceConfidence(10, 10, 1.0)
	
	// Base score: (1.0 * 0.7) + (1.0 * 0.3) = 1.0
	// Sample penalty: log(11)/log(101) ≈ 0.518
	// Expected: 1.0 * 0.518 ≈ 0.518
	assert.Greater(t, confidence, 0.5)
	assert.Less(t, confidence, 0.6)
}

func TestCalculatePreferenceConfidence_PerfectLargeSample(t *testing.T) {
	// 100/100 success with quality 1.0
	confidence := CalculatePreferenceConfidence(100, 100, 1.0)
	
	// Base score: (1.0 * 0.7) + (1.0 * 0.3) = 1.0
	// Sample penalty: 1.0 (no penalty for 100+ samples)
	// Expected: 1.0
	assert.InDelta(t, 1.0, confidence, 0.01)
}

func TestCalculatePreferenceConfidence_MixedResults(t *testing.T) {
	// 70/100 success with quality 0.8
	confidence := CalculatePreferenceConfidence(70, 100, 0.8)
	
	// Base score: (0.7 * 0.7) + (0.8 * 0.3) = 0.49 + 0.24 = 0.73
	// Sample penalty: 1.0 (no penalty for 100+ samples)
	// Expected: 0.73
	assert.InDelta(t, 0.73, confidence, 0.02)
}

func TestCalculatePreferenceConfidence_NoQualityScore(t *testing.T) {
	// 80/100 success with no quality score (0.0)
	confidence := CalculatePreferenceConfidence(80, 100, 0.0)
	
	// Base score: 0.8 (success rate only when quality is 0)
	// Sample penalty: 1.0 (no penalty for 100+ samples)
	// Expected: 0.8
	assert.InDelta(t, 0.8, confidence, 0.01)
}

func TestCalculatePreferenceConfidence_SampleSizePenalty(t *testing.T) {
	// Test different sample sizes with same success rate
	testCases := []struct {
		samples  int
		expected float64 // Approximate expected penalty factor
	}{
		{1, 0.15},   // log(2)/log(101) ≈ 0.15
		{5, 0.39},   // log(6)/log(101) ≈ 0.39
		{10, 0.52},  // log(11)/log(101) ≈ 0.52
		{20, 0.66},  // log(21)/log(101) ≈ 0.66
		{50, 0.85},  // log(51)/log(101) ≈ 0.85
		{100, 1.0},  // log(101)/log(101) = 1.0
		{200, 1.0},  // No penalty for 100+
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("samples_%d", tc.samples), func(t *testing.T) {
			// Perfect success rate to isolate sample penalty
			confidence := CalculatePreferenceConfidence(tc.samples, tc.samples, 1.0)
			assert.InDelta(t, tc.expected, confidence, 0.05, 
				"Sample size %d should have penalty factor ~%.2f", tc.samples, tc.expected)
		})
	}
}

func TestAdjustRuntimeConfidence_BaselineOnly(t *testing.T) {
	// No adjustments - should return base confidence
	adjusted := AdjustRuntimeConfidence(0.7, false, 0.0, false)
	assert.Equal(t, 0.7, adjusted)
}

func TestAdjustRuntimeConfidence_PreferenceBoost(t *testing.T) {
	// With preference boost (+0.15)
	adjusted := AdjustRuntimeConfidence(0.7, true, 0.0, false)
	assert.InDelta(t, 0.85, adjusted, 0.01)
}

func TestAdjustRuntimeConfidence_ProviderBias(t *testing.T) {
	// Test positive bias
	adjusted := AdjustRuntimeConfidence(0.7, false, 0.5, false)
	// Provider bias: 0.5 * 0.1 = 0.05
	assert.InDelta(t, 0.75, adjusted, 0.01)
	
	// Test negative bias
	adjusted = AdjustRuntimeConfidence(0.7, false, -0.8, false)
	// Provider bias: -0.8 * 0.1 = -0.08
	assert.InDelta(t, 0.62, adjusted, 0.01)
}

func TestAdjustRuntimeConfidence_TimePatternBoost(t *testing.T) {
	// With time pattern boost (+0.1)
	adjusted := AdjustRuntimeConfidence(0.7, false, 0.0, true)
	assert.InDelta(t, 0.8, adjusted, 0.01)
}

func TestAdjustRuntimeConfidence_AllFactors(t *testing.T) {
	// All factors combined
	// Base: 0.6, Preference: +0.15, Bias: +0.05, Time: +0.1
	// Expected: 0.6 + 0.15 + 0.05 + 0.1 = 0.9
	adjusted := AdjustRuntimeConfidence(0.6, true, 0.5, true)
	assert.InDelta(t, 0.9, adjusted, 0.01)
}

func TestAdjustRuntimeConfidence_ClampingLow(t *testing.T) {
	// Test lower bound clamping
	adjusted := AdjustRuntimeConfidence(0.1, false, -1.0, false)
	// Base: 0.1, Bias: -1.0 * 0.1 = -0.1
	// Expected: max(0.0, 0.1 - 0.1) = 0.0
	assert.Equal(t, 0.0, adjusted)
}

func TestAdjustRuntimeConfidence_ClampingHigh(t *testing.T) {
	// Test upper bound clamping
	adjusted := AdjustRuntimeConfidence(0.9, true, 1.0, true)
	// Base: 0.9, Preference: +0.15, Bias: +0.1, Time: +0.1
	// Expected: min(1.0, 0.9 + 0.15 + 0.1 + 0.1) = 1.0
	assert.Equal(t, 1.0, adjusted)
}

func TestAdjustRuntimeConfidence_WeightingFormula(t *testing.T) {
	// Test the exact weighting from design document
	testCases := []struct {
		name         string
		base         float64
		hasPreference bool
		providerBias float64
		isTimeMatch  bool
		expected     float64
	}{
		{
			name:         "typical_good_case",
			base:         0.75,
			hasPreference: true,
			providerBias: 0.3,
			isTimeMatch:  true,
			expected:     0.75 + 0.15 + 0.03 + 0.1, // 1.03 -> clamped to 1.0
		},
		{
			name:         "typical_poor_case", 
			base:         0.4,
			hasPreference: false,
			providerBias: -0.6,
			isTimeMatch:  false,
			expected:     0.4 + 0.0 + (-0.06) + 0.0, // 0.34
		},
		{
			name:         "mixed_case",
			base:         0.6,
			hasPreference: true,
			providerBias: -0.2,
			isTimeMatch:  false,
			expected:     0.6 + 0.15 + (-0.02) + 0.0, // 0.73
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adjusted := AdjustRuntimeConfidence(tc.base, tc.hasPreference, tc.providerBias, tc.isTimeMatch)
			
			// Clamp expected value to [0.0, 1.0] for comparison
			expected := tc.expected
			if expected > 1.0 {
				expected = 1.0
			}
			if expected < 0.0 {
				expected = 0.0
			}
			
			assert.InDelta(t, expected, adjusted, 0.01, 
				"Case %s: base=%.2f, pref=%t, bias=%.2f, time=%t", 
				tc.name, tc.base, tc.hasPreference, tc.providerBias, tc.isTimeMatch)
		})
	}
}

func BenchmarkAdjustRuntimeConfidence(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AdjustRuntimeConfidence(0.75, true, 0.3, true)
	}
}

func BenchmarkAdjustRuntimeConfidence_Minimal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AdjustRuntimeConfidence(0.75, false, 0.0, false)
	}
}