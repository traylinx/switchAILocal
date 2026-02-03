package steering

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConditionEvaluator_Basic(t *testing.T) {
	evaluator := NewConditionEvaluator()

	ctx := &RoutingContext{
		Intent:        "coding",
		Provider:      "ollama",
		ContentLength: 500,
		Hour:          14,
	}

	// Test simple conditions
	result, err := evaluator.Evaluate("Intent == 'coding'", ctx)
	require.NoError(t, err)
	assert.True(t, result)

	result, err = evaluator.Evaluate("Intent == 'reasoning'", ctx)
	require.NoError(t, err)
	assert.False(t, result)

	// Test empty condition
	result, err = evaluator.Evaluate("", ctx)
	require.NoError(t, err)
	assert.True(t, result)

	// Test "true" condition
	result, err = evaluator.Evaluate("true", ctx)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestConditionEvaluator_ComplexConditions(t *testing.T) {
	evaluator := NewConditionEvaluator()

	ctx := &RoutingContext{
		Intent:        "coding",
		Provider:      "ollama",
		ContentLength: 500,
		Hour:          14,
	}

	// Test complex condition
	result, err := evaluator.Evaluate("Intent == 'coding' && ContentLength > 100", ctx)
	require.NoError(t, err)
	assert.True(t, result)

	result, err = evaluator.Evaluate("Intent == 'coding' && ContentLength < 100", ctx)
	require.NoError(t, err)
	assert.False(t, result)

	// Test OR condition
	result, err = evaluator.Evaluate("Intent == 'reasoning' || Provider == 'ollama'", ctx)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestConditionEvaluator_ErrorHandling(t *testing.T) {
	evaluator := NewConditionEvaluator()

	ctx := &RoutingContext{
		Intent: "coding",
	}

	// Test invalid syntax
	_, err := evaluator.Evaluate("Intent ==", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to compile condition")

	// Test non-boolean result
	_, err = evaluator.Evaluate("Intent", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "did not return a boolean")
}

func TestConditionEvaluator_TimeRules(t *testing.T) {
	evaluator := NewConditionEvaluator()

	// Test Monday 10 AM
	mondayMorning := time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC) // Feb 2, 2026 is Monday

	// Test work hours rule
	workRule := TimeBasedRule{
		Hours: "9-17",
		Days:  "Mon-Fri",
	}
	assert.True(t, evaluator.CheckTimeRule(workRule, mondayMorning))

	// Test weekend rule
	weekendRule := TimeBasedRule{
		Days: "Sat-Sun",
	}
	assert.False(t, evaluator.CheckTimeRule(weekendRule, mondayMorning))

	// Test evening hours
	mondayEvening := time.Date(2026, 2, 2, 20, 0, 0, 0, time.UTC)
	assert.False(t, evaluator.CheckTimeRule(workRule, mondayEvening))

	// Test empty rule (should match)
	emptyRule := TimeBasedRule{}
	assert.True(t, evaluator.CheckTimeRule(emptyRule, mondayMorning))
}

func TestConditionEvaluator_DayMatching(t *testing.T) {
	evaluator := NewConditionEvaluator()

	// Test Monday
	monday := time.Monday
	assert.True(t, evaluator.isInDayRange(monday, "Mon-Fri"))
	assert.False(t, evaluator.isInDayRange(monday, "Sat-Sun"))
	assert.True(t, evaluator.isInDayRange(monday, "Mon"))

	// Test Saturday
	saturday := time.Saturday
	assert.False(t, evaluator.isInDayRange(saturday, "Mon-Fri"))
}

func TestConditionEvaluator_HourMatching(t *testing.T) {
	evaluator := NewConditionEvaluator()

	// Test 10 AM
	tenAM := time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC)
	assert.True(t, evaluator.isInHourRange(tenAM.Hour(), "9-17"))
	assert.False(t, evaluator.isInHourRange(tenAM.Hour(), "18-22"))

	// Test 8 PM
	eightPM := time.Date(2026, 2, 2, 20, 0, 0, 0, time.UTC)
	assert.False(t, evaluator.isInHourRange(eightPM.Hour(), "9-17"))
	assert.True(t, evaluator.isInHourRange(eightPM.Hour(), "18-22"))

	// Test invalid format
	assert.False(t, evaluator.isInHourRange(tenAM.Hour(), "invalid"))
}

func TestConditionEvaluator_ProgramCaching(t *testing.T) {
	evaluator := NewConditionEvaluator()

	ctx := &RoutingContext{
		Intent: "coding",
	}

	// First evaluation should compile and cache
	result1, err := evaluator.Evaluate("Intent == 'coding'", ctx)
	require.NoError(t, err)
	assert.True(t, result1)

	// Second evaluation should use cached program
	result2, err := evaluator.Evaluate("Intent == 'coding'", ctx)
	require.NoError(t, err)
	assert.True(t, result2)

	// Verify program was cached
	assert.Equal(t, 1, len(evaluator.programs))
}

// Benchmark tests

// BenchmarkConditionEvaluator_Simple benchmarks simple condition evaluation
func BenchmarkConditionEvaluator_Simple(b *testing.B) {
	evaluator := NewConditionEvaluator()
	ctx := &RoutingContext{
		Intent:        "coding",
		Provider:      "ollama",
		ContentLength: 500,
		Hour:          14,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = evaluator.Evaluate("Intent == 'coding'", ctx)
	}
}

// BenchmarkConditionEvaluator_Complex benchmarks complex condition evaluation
func BenchmarkConditionEvaluator_Complex(b *testing.B) {
	evaluator := NewConditionEvaluator()
	ctx := &RoutingContext{
		Intent:        "coding",
		Provider:      "ollama",
		ContentLength: 500,
		Hour:          14,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = evaluator.Evaluate("Intent == 'coding' && ContentLength > 100 && Hour >= 9 && Hour <= 17", ctx)
	}
}

// BenchmarkConditionEvaluator_TimeRule benchmarks time rule checking
func BenchmarkConditionEvaluator_TimeRule(b *testing.B) {
	evaluator := NewConditionEvaluator()
	now := time.Now()
	rule := TimeBasedRule{
		Hours: "9-17",
		Days:  "Mon-Fri",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluator.CheckTimeRule(rule, now)
	}
}
