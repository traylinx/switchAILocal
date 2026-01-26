package metadata

import (
	"sync"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestNewAggregator(t *testing.T) {
	requestID := "test-request-123"
	provider := "claudecli"

	agg := NewAggregator(requestID, provider)

	if agg.requestID != requestID {
		t.Errorf("Expected requestID %s, got %s", requestID, agg.requestID)
	}

	if agg.originalProvider != provider {
		t.Errorf("Expected originalProvider %s, got %s", provider, agg.originalProvider)
	}

	if agg.finalProvider != provider {
		t.Errorf("Expected finalProvider to initially be %s, got %s", provider, agg.finalProvider)
	}

	if agg.GetActionCount() != 0 {
		t.Errorf("Expected 0 actions initially, got %d", agg.GetActionCount())
	}
}

func TestRecordAction(t *testing.T) {
	agg := NewAggregator("req-1", "claudecli")

	// Record a successful action
	agg.RecordAction("stdin_injection", "Injected permission response", true, map[string]interface{}{
		"pattern": "file_permission",
		"response": "y\n",
	})

	if agg.GetActionCount() != 1 {
		t.Errorf("Expected 1 action, got %d", agg.GetActionCount())
	}

	if !agg.HasActions() {
		t.Error("Expected HasActions to return true")
	}

	metadata := agg.GetMetadata()
	if len(metadata.Actions) != 1 {
		t.Errorf("Expected 1 action in metadata, got %d", len(metadata.Actions))
	}

	action := metadata.Actions[0]
	if action.ActionType != "stdin_injection" {
		t.Errorf("Expected action type 'stdin_injection', got %s", action.ActionType)
	}

	if !action.Success {
		t.Error("Expected action to be successful")
	}

	if action.Details["pattern"] != "file_permission" {
		t.Errorf("Expected pattern 'file_permission', got %v", action.Details["pattern"])
	}
}

func TestRecordMultipleActions(t *testing.T) {
	agg := NewAggregator("req-2", "claudecli")

	// Record multiple actions
	agg.RecordAction("stdin_injection", "First injection", true, nil)
	agg.RecordAction("restart_with_flags", "Restarted with --skip-permissions", true, map[string]interface{}{
		"flags": []string{"--skip-permissions"},
	})
	agg.RecordAction("fallback_routing", "Routed to gemini", true, map[string]interface{}{
		"target_provider": "geminicli",
	})

	if agg.GetActionCount() != 3 {
		t.Errorf("Expected 3 actions, got %d", agg.GetActionCount())
	}

	metadata := agg.GetMetadata()
	if len(metadata.Actions) != 3 {
		t.Errorf("Expected 3 actions in metadata, got %d", len(metadata.Actions))
	}

	// Verify actions are in chronological order
	if metadata.Actions[0].ActionType != "stdin_injection" {
		t.Errorf("Expected first action to be stdin_injection, got %s", metadata.Actions[0].ActionType)
	}

	if metadata.Actions[1].ActionType != "restart_with_flags" {
		t.Errorf("Expected second action to be restart_with_flags, got %s", metadata.Actions[1].ActionType)
	}

	if metadata.Actions[2].ActionType != "fallback_routing" {
		t.Errorf("Expected third action to be fallback_routing, got %s", metadata.Actions[2].ActionType)
	}
}

func TestRecordDiagnosis(t *testing.T) {
	agg := NewAggregator("req-3", "claudecli")

	diagnosis := &types.Diagnosis{
		FailureType:  types.FailureTypePermissionPrompt,
		RootCause:    "Process waiting for file permission",
		Confidence:   0.95,
		Remediation:  types.RemediationStdinInject,
		RemediationArgs: map[string]string{
			"pattern": "file_permission",
		},
	}

	agg.RecordDiagnosis(diagnosis)

	metadata := agg.GetMetadata()
	if len(metadata.DiagnosisHistory) != 1 {
		t.Errorf("Expected 1 diagnosis, got %d", len(metadata.DiagnosisHistory))
	}

	recorded := metadata.DiagnosisHistory[0]
	if recorded.FailureType != types.FailureTypePermissionPrompt {
		t.Errorf("Expected failure type permission_prompt, got %s", recorded.FailureType)
	}

	if recorded.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", recorded.Confidence)
	}
}

func TestSetFinalProvider(t *testing.T) {
	agg := NewAggregator("req-4", "claudecli")

	// Initially, final provider should match original
	if agg.GetFinalProvider() != "claudecli" {
		t.Errorf("Expected initial final provider to be claudecli, got %s", agg.GetFinalProvider())
	}

	if agg.WasProviderChanged() {
		t.Error("Expected WasProviderChanged to be false initially")
	}

	// Change the final provider
	agg.SetFinalProvider("geminicli")

	if agg.GetFinalProvider() != "geminicli" {
		t.Errorf("Expected final provider to be geminicli, got %s", agg.GetFinalProvider())
	}

	if !agg.WasProviderChanged() {
		t.Error("Expected WasProviderChanged to be true after change")
	}

	metadata := agg.GetMetadata()
	if metadata.OriginalProvider != "claudecli" {
		t.Errorf("Expected original provider to remain claudecli, got %s", metadata.OriginalProvider)
	}

	if metadata.FinalProvider != "geminicli" {
		t.Errorf("Expected final provider to be geminicli, got %s", metadata.FinalProvider)
	}
}

func TestSetContextOptimization(t *testing.T) {
	agg := NewAggregator("req-5", "claudecli")

	highDensityMap := &types.HighDensityMap{
		TotalFiles:    100,
		IncludedFiles: 20,
		ExcludedFiles: 80,
		DirectoryTree: "src/\n  main.go\n  utils/",
		FileSummaries: map[string]string{
			"test.go": "Test file with unit tests",
		},
		TokensSaved: 50000,
	}

	agg.SetContextOptimization(highDensityMap)

	metadata := agg.GetMetadata()
	if !metadata.ContextOptimized {
		t.Error("Expected ContextOptimized to be true")
	}

	if metadata.HighDensityMap == nil {
		t.Fatal("Expected HighDensityMap to be set")
	}

	if metadata.HighDensityMap.TotalFiles != 100 {
		t.Errorf("Expected 100 total files, got %d", metadata.HighDensityMap.TotalFiles)
	}

	if metadata.HighDensityMap.TokensSaved != 50000 {
		t.Errorf("Expected 50000 tokens saved, got %d", metadata.HighDensityMap.TokensSaved)
	}
}

func TestGetMetadata_TotalHealingTime(t *testing.T) {
	agg := NewAggregator("req-6", "claudecli")

	// Wait a bit to ensure some time passes
	time.Sleep(10 * time.Millisecond)

	agg.RecordAction("test_action", "Test", true, nil)

	metadata := agg.GetMetadata()
	if metadata.TotalHealingTimeMs <= 0 {
		t.Errorf("Expected positive healing time, got %d", metadata.TotalHealingTimeMs)
	}

	// Should be at least 10ms
	if metadata.TotalHealingTimeMs < 10 {
		t.Errorf("Expected at least 10ms healing time, got %d", metadata.TotalHealingTimeMs)
	}
}

func TestGetMetadata_RequestID(t *testing.T) {
	requestID := "unique-request-789"
	agg := NewAggregator(requestID, "claudecli")

	metadata := agg.GetMetadata()
	if metadata.RequestID != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, metadata.RequestID)
	}
}

func TestConcurrentAccess(t *testing.T) {
	agg := NewAggregator("req-concurrent", "claudecli")

	var wg sync.WaitGroup
	numGoroutines := 10
	actionsPerGoroutine := 5

	// Spawn multiple goroutines that record actions concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < actionsPerGoroutine; j++ {
				agg.RecordAction("concurrent_action", "Test concurrent", true, map[string]interface{}{
					"goroutine": id,
					"iteration": j,
				})
			}
		}(i)
	}

	wg.Wait()

	expectedActions := numGoroutines * actionsPerGoroutine
	if agg.GetActionCount() != expectedActions {
		t.Errorf("Expected %d actions, got %d", expectedActions, agg.GetActionCount())
	}

	metadata := agg.GetMetadata()
	if len(metadata.Actions) != expectedActions {
		t.Errorf("Expected %d actions in metadata, got %d", expectedActions, len(metadata.Actions))
	}
}

func TestHasActions_Empty(t *testing.T) {
	agg := NewAggregator("req-empty", "claudecli")

	if agg.HasActions() {
		t.Error("Expected HasActions to return false for new aggregator")
	}

	if agg.GetActionCount() != 0 {
		t.Errorf("Expected 0 actions, got %d", agg.GetActionCount())
	}
}

func TestGetOriginalProvider(t *testing.T) {
	provider := "test-provider"
	agg := NewAggregator("req-7", provider)

	if agg.GetOriginalProvider() != provider {
		t.Errorf("Expected original provider %s, got %s", provider, agg.GetOriginalProvider())
	}

	// Original provider should not change even after setting final provider
	agg.SetFinalProvider("different-provider")

	if agg.GetOriginalProvider() != provider {
		t.Errorf("Expected original provider to remain %s, got %s", provider, agg.GetOriginalProvider())
	}
}
