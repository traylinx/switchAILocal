package intelligence

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// Integration tests for Cortex Router with Intelligence Service

func TestCortexRouter_IntegrationWithIntelligenceService(t *testing.T) {
	// Create intelligence service
	cfg := &config.IntelligenceConfig{
		Enabled: true,
		SemanticTier: config.SemanticTierConfig{
			Enabled:             false, // Disable for this test
			ConfidenceThreshold: 0.7,
		},
		SemanticCache: config.SemanticCacheConfig{
			Enabled:             false, // Disable for this test
			SimilarityThreshold: 0.8,
			MaxSize:             1000,
		},
	}

	service := NewService(cfg)

	// Initialize service
	ctx := context.Background()
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize intelligence service: %v", err)
	}
	defer func() { _ = service.Shutdown(ctx) }()

	// Get Cortex Router from service
	router := service.GetCortexRouter()
	if router == nil {
		t.Fatal("Expected non-nil Cortex Router from intelligence service")
	}

	// Test basic routing functionality
	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "Hello world!",
	}

	decision, err := router.Route(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error during routing: %v", err)
	}

	if decision == nil {
		t.Fatal("Expected non-nil routing decision")
	}

	// Verify decision has required fields
	if decision.Tier == "" {
		t.Error("Expected non-empty tier")
	}

	if decision.SelectedModel == "" {
		t.Error("Expected non-empty selected model")
	}

	if decision.Confidence < 0.0 || decision.Confidence > 1.0 {
		t.Errorf("Expected confidence between 0.0 and 1.0, got %.2f", decision.Confidence)
	}

	// Should use reflex tier for simple greeting
	if decision.Tier != "reflex" {
		t.Errorf("Expected tier 'reflex' for simple greeting, got '%s'", decision.Tier)
	}

	if decision.Intent != "chat" {
		t.Errorf("Expected intent 'chat' for greeting, got '%s'", decision.Intent)
	}
}

func TestIntegration_MemoryAndRouting(t *testing.T) {
	// Setup temporary directory for memory
	tempDir, err := os.MkdirTemp("", "cortex-memory-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup memory manager
	memCfg := &memory.MemoryConfig{
		Enabled:       true,
		BaseDir:       tempDir,
		RetentionDays: 1,
		MaxLogSizeMB:  1,
	}
	memManager, err := memory.NewMemoryManager(memCfg)
	require.NoError(t, err)
	defer memManager.Close()

	// Setup router
	routerCfg := &config.IntelligenceConfig{Enabled: true}
	router := NewCortexRouter(routerCfg, memManager)

	apiKey := "integration-test-key"
	apiKeyHash := router.hashAPIKey(apiKey)

	// 1. Initial request - should use default fallback or patterns
	ctx := context.Background()
	req := &RoutingRequest{
		APIKey:  apiKey,
		Content: "Developing a robust software authentication module using golang and cryptography",
	}

	decision, err := router.Route(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "reflex", decision.Tier)
	assert.Equal(t, "coding", decision.Intent)
	assert.Equal(t, "claudecli:claude-sonnet-4", decision.SelectedModel)

	// 2. Simulate multiple successes for a different model to force a preference
	otherModel := "geminicli:gemini-2.5-pro"
	for i := 0; i < 5; i++ {
		// Use specific preferences setting to ensure it takes effect in the test
		prefs, _ := memManager.GetUserPreferences(apiKeyHash)
		if prefs == nil {
			prefs = &memory.UserPreferences{
				APIKeyHash: apiKeyHash,
			}
		}
		if prefs.ModelPreferences == nil {
			prefs.ModelPreferences = make(map[string]string)
		}
		prefs.ModelPreferences["coding"] = otherModel
		prefs.LastUpdated = time.Now()
		err = memManager.UpdateUserPreferences(prefs)
		require.NoError(t, err)
	}

	// 3. Request again - should now prefer otherModel (learned tier)
	decision2, err := router.Route(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "learned", decision2.Tier)
	assert.Equal(t, otherModel, decision2.SelectedModel)
}

func TestIntegration_SteeringAndExecution(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	// Test mapping (internal steering logic)
	assert.Equal(t, "claudecli:claude-sonnet-4", router.mapIntentToModel("coding"))
	assert.Equal(t, "geminicli:gemini-2.5-pro", router.mapIntentToModel("reasoning"))
	assert.Equal(t, "ollama:qwen:0.5b", router.mapIntentToModel("chat"))
}

func TestIntegration_HooksAndEvents(t *testing.T) {
	config := &config.IntelligenceConfig{Enabled: true}
	memoryManager := newMockMemoryManager(true)
	router := NewCortexRouter(config, memoryManager)

	// Verify that recordDecision records to memory (which would normally trigger events)
	decision := &RoutingDecision{
		Timestamp:     time.Now(),
		APIKeyHash:    "test-hash",
		SelectedModel: "test-model",
		Tier:          "reflex",
	}
	router.recordDecision(decision)

	// Verify it was recorded in memory
	assert.Equal(t, 1, len(memoryManager.decisions))
}

// Benchmark integration performance
func BenchmarkCortexRouter_Integration(b *testing.B) {
	// Create intelligence service
	cfg := &config.IntelligenceConfig{
		Enabled: true,
	}

	service := NewService(cfg)

	// Initialize service
	ctx := context.Background()
	err := service.Initialize(ctx)
	if err != nil {
		b.Fatalf("Failed to initialize intelligence service: %v", err)
	}
	defer func() { _ = service.Shutdown(ctx) }()

	// Get Cortex Router from service
	router := service.GetCortexRouter()
	if router == nil {
		b.Fatal("Expected non-nil Cortex Router from intelligence service")
	}

	request := &RoutingRequest{
		APIKey:  "test-key",
		Model:   "auto",
		Content: "Hello world!",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Route(ctx, request)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}
