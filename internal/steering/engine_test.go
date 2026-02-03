package steering

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSteeringEngine_Basic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ruleContent := `
name: "Test Rule"
activation:
  condition: "Intent == 'test'"
  priority: 10
preferences:
  primary_model: "test-model"
  context_injection: "injected context"
`
	err = os.WriteFile(filepath.Join(tmpDir, "test.yaml"), []byte(ruleContent), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	err = engine.LoadRules()
	require.NoError(t, err)

	assert.Equal(t, 1, len(engine.GetRules()))

	ctx := &RoutingContext{
		Intent:    "test",
		Timestamp: time.Now(),
	}

	matches, err := engine.FindMatchingRules(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(matches))
	assert.Equal(t, "Test Rule", matches[0].Name)

	model, messages, _ := engine.ApplySteering(ctx, nil, nil, matches)
	assert.Equal(t, "test-model", model)
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "injected context", messages[0]["content"])
}

func TestSteeringEngine_Priority(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-priority-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rule1 := `
name: "Low Priority"
activation:
  condition: "true"
  priority: 10
preferences:
  primary_model: "low-model"
`
	rule2 := `
name: "High Priority"
activation:
  condition: "true"
  priority: 100
preferences:
  primary_model: "high-model"
`
	err = os.WriteFile(filepath.Join(tmpDir, "low.yaml"), []byte(rule1), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "high.yaml"), []byte(rule2), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	err = engine.LoadRules()
	require.NoError(t, err)

	matches, _ := engine.FindMatchingRules(&RoutingContext{Timestamp: time.Now()})
	assert.Equal(t, 2, len(matches))

	// Rule 2 should be first because of priority 100
	assert.Equal(t, "High Priority", matches[0].Name)

	model, _, _ := engine.ApplySteering(&RoutingContext{Timestamp: time.Now()}, nil, nil, matches)
	assert.Equal(t, "high-model", model)
}

func TestSteeringEngine_TimeBased(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-time-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rule := `
name: "Time Rule"
activation:
  condition: "true"
preferences:
  primary_model: "default-model"
  time_based_rules:
    - hours: "9-17"
      days: "Mon-Fri"
      prefer_model: "work-model"
`
	err = os.WriteFile(filepath.Join(tmpDir, "time.yaml"), []byte(rule), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)
	err = engine.LoadRules()
	require.NoError(t, err)

	// 1. Match work time (Monday 10 AM)
	mondayWorkTime := time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC) // Feb 2, 2026 is Monday
	ctx1 := &RoutingContext{Timestamp: mondayWorkTime}
	matches1, _ := engine.FindMatchingRules(ctx1)
	model1, _, _ := engine.ApplySteering(ctx1, nil, nil, matches1)
	assert.Equal(t, "work-model", model1)

	// 2. Out of work time (Monday 8 PM)
	mondayNightTime := time.Date(2026, 2, 2, 20, 0, 0, 0, time.UTC)
	ctx2 := &RoutingContext{Timestamp: mondayNightTime}
	matches2, _ := engine.FindMatchingRules(ctx2)
	model2, _, _ := engine.ApplySteering(ctx2, nil, nil, matches2)
	assert.Equal(t, "default-model", model2)
}

func TestSteeringEngine_ContextInjection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-injection-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rule := `
name: "Context Injection Rule"
activation:
  condition: "true"
preferences:
  primary_model: "test-model"
  context_injection: "You are helping with {{intent}} tasks. Current hour: {{hour}}"
  provider_settings:
    temperature: 0.7
    max_tokens: 2048
`
	err = os.WriteFile(filepath.Join(tmpDir, "injection.yaml"), []byte(rule), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)
	err = engine.LoadRules()
	require.NoError(t, err)

	ctx := &RoutingContext{
		Intent:    "coding",
		Hour:      14,
		Timestamp: time.Now(),
	}

	matches, _ := engine.FindMatchingRules(ctx)
	require.Equal(t, 1, len(matches))

	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}
	metadata := make(map[string]interface{})

	model, newMessages, newMetadata := engine.ApplySteering(ctx, messages, metadata, matches)

	assert.Equal(t, "test-model", model)
	assert.Equal(t, 2, len(newMessages))
	assert.Equal(t, "system", newMessages[0]["role"])
	assert.Contains(t, newMessages[0]["content"], "coding")
	assert.Contains(t, newMessages[0]["content"], "14")
	assert.Equal(t, float64(0.7), newMetadata["steering_temperature"])
	assert.Equal(t, 2048, newMetadata["steering_max_tokens"])
}

func TestSteeringEngine_ErrorHandling(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-error-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Invalid YAML
	invalidRule := `
name: "Invalid Rule"
activation:
  condition: "Intent == 'test'"
  priority: invalid_number
`
	err = os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte(invalidRule), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	// Should not fail, but should skip invalid rule
	err = engine.LoadRules()
	require.NoError(t, err)
	assert.Equal(t, 0, len(engine.GetRules()))
}

func TestSteeringEngine_EmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-empty-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	err = engine.LoadRules()
	require.NoError(t, err)
	assert.Equal(t, 0, len(engine.GetRules()))
}

func TestSteeringEngine_DefaultDirectory(t *testing.T) {
	engine, err := NewSteeringEngine("")
	require.NoError(t, err)
	assert.Contains(t, engine.steeringDir, ".switchailocal/steering")
}

func TestSteeringEngine_OverrideRouter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-override-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rule1 := `
name: "Override Rule"
activation:
  condition: "true"
  priority: 100
preferences:
  primary_model: "override-model"
  override_router: true
`
	rule2 := `
name: "Lower Priority Rule"
activation:
  condition: "true"
  priority: 50
preferences:
  primary_model: "lower-model"
`
	err = os.WriteFile(filepath.Join(tmpDir, "override.yaml"), []byte(rule1), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "lower.yaml"), []byte(rule2), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)
	err = engine.LoadRules()
	require.NoError(t, err)

	ctx := &RoutingContext{Timestamp: time.Now()}
	matches, _ := engine.FindMatchingRules(ctx)
	model, _, _ := engine.ApplySteering(ctx, nil, nil, matches)

	// Should use override rule and stop processing
	assert.Equal(t, "override-model", model)
}

func TestSteeringEngine_Watcher(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-watcher-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	// Test starting watcher
	err = engine.StartWatcher()
	require.NoError(t, err)

	// Test stopping watcher
	engine.StopWatcher()

	// Test stopping when no watcher exists
	engine.StopWatcher() // Should not panic
}

func TestSteeringEngine_LoadRulesEdgeCases(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-edge-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a non-YAML file that should be ignored
	err = os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("This is not YAML"), 0644)
	require.NoError(t, err)

	// Create a valid YAML file
	validRule := `
name: "Valid Rule"
activation:
  condition: "true"
  priority: 10
preferences:
  primary_model: "test-model"
`
	err = os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(validRule), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)

	err = engine.LoadRules()
	require.NoError(t, err)
	assert.Equal(t, 1, len(engine.GetRules()))
}

func TestSteeringEngine_FindMatchingRulesError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "steering-error-match-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Rule with invalid condition
	invalidRule := `
name: "Invalid Condition Rule"
activation:
  condition: "Intent =="
  priority: 10
preferences:
  primary_model: "test-model"
`
	err = os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte(invalidRule), 0644)
	require.NoError(t, err)

	engine, err := NewSteeringEngine(tmpDir)
	require.NoError(t, err)
	err = engine.LoadRules()
	require.NoError(t, err)

	ctx := &RoutingContext{
		Intent:    "test",
		Timestamp: time.Now(),
	}

	// Should handle error gracefully and return empty matches
	matches, err := engine.FindMatchingRules(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, len(matches))
}
