package memory

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// TestProperty_RoutingDecisionRecording tests Property 1: Routing Decision Recording
// Feature: clawd-patterns-integration, Property 1: Routing Decision Recording
// Validates: Requirements FR-1.1
func TestProperty_RoutingDecisionRecording(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("all routing decisions are recorded and retrievable", prop.ForAll(
		func(decision *RoutingDecision) bool {
			// Create temporary directory for this test
			tmpDir, err := os.MkdirTemp("", "memory-property-test-*")
			if err != nil {
				t.Logf("Failed to create temp dir: %v", err)
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Initialize directory structure
			ds := NewDirectoryStructure(filepath.Join(tmpDir, "memory"))
			if err := ds.Initialize(); err != nil {
				t.Logf("Failed to initialize directory structure: %v", err)
				return false
			}

			// Create routing history store
			store, err := NewRoutingHistoryStore(ds.GetRoutingHistoryPath())
			if err != nil {
				t.Logf("Failed to create routing history store: %v", err)
				return false
			}
			defer store.Close()

			// Record the routing decision
			if err := store.RecordRouting(decision); err != nil {
				t.Logf("Failed to record routing decision: %v", err)
				return false
			}

			// Verify decision can be retrieved
			history, err := store.GetHistory(decision.APIKeyHash, 1)
			if err != nil {
				t.Logf("Failed to get history: %v", err)
				return false
			}

			// Should have exactly one decision
			if len(history) != 1 {
				t.Logf("Expected 1 decision, got %d", len(history))
				return false
			}

			// Verify the decision matches what we recorded
			retrieved := history[0]
			if retrieved.APIKeyHash != decision.APIKeyHash {
				t.Logf("API key hash mismatch: expected %s, got %s", decision.APIKeyHash, retrieved.APIKeyHash)
				return false
			}

			if retrieved.Request.Intent != decision.Request.Intent {
				t.Logf("Intent mismatch: expected %s, got %s", decision.Request.Intent, retrieved.Request.Intent)
				return false
			}

			if retrieved.Routing.SelectedModel != decision.Routing.SelectedModel {
				t.Logf("Selected model mismatch: expected %s, got %s", decision.Routing.SelectedModel, retrieved.Routing.SelectedModel)
				return false
			}

			return true
		},
		genRoutingDecision(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestProperty_DirectoryStructureInitialization tests that directory structure initialization is idempotent and complete
// Feature: clawd-patterns-integration, Property 1: Routing Decision Recording (directory structure component)
// Validates: Requirements FR-1.1
func TestProperty_DirectoryStructureInitialization(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("directory structure initialization is idempotent and complete", prop.ForAll(
		func(baseDir string) bool {
			// Sanitize base directory name to avoid invalid paths
			if baseDir == "" || baseDir == "." || baseDir == ".." {
				baseDir = "memory-test"
			}

			// Create temporary directory for this test
			tmpDir, err := os.MkdirTemp("", "memory-property-test-*")
			if err != nil {
				t.Logf("Failed to create temp dir: %v", err)
				return false
			}
			defer os.RemoveAll(tmpDir)

			fullBaseDir := filepath.Join(tmpDir, baseDir)
			ds := NewDirectoryStructure(fullBaseDir)

			// Initialize once
			if err := ds.Initialize(); err != nil {
				t.Logf("First initialization failed: %v", err)
				return false
			}

			// Validate structure
			if err := ds.Validate(); err != nil {
				t.Logf("First validation failed: %v", err)
				return false
			}

			// Initialize again (should be idempotent)
			if err := ds.Initialize(); err != nil {
				t.Logf("Second initialization failed: %v", err)
				return false
			}

			// Validate structure again
			if err := ds.Validate(); err != nil {
				t.Logf("Second validation failed: %v", err)
				return false
			}

			// Verify all required paths exist and are accessible
			paths := []string{
				ds.GetRoutingHistoryPath(),
				ds.GetProviderQuirksPath(),
				ds.GetUserPreferencesDir(),
				ds.GetDailyLogsDir(),
				ds.GetAnalyticsDir(),
			}

			for _, path := range paths {
				if _, err := os.Stat(path); err != nil {
					t.Logf("Required path does not exist: %s, error: %v", path, err)
					return false
				}
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) < 50 && s != "." && s != ".."
		}),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestProperty_PreferenceLearning tests Property 2: Preference Learning
// Feature: clawd-patterns-integration, Property 2: Preference Learning
// Validates: Requirements FR-1.2
func TestProperty_PreferenceLearning(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("successful routing decisions are learned as model preferences", prop.ForAll(
		func(intent string, model string, apiKeyHash string) bool {
			// Create temporary directory for this test
			tmpDir, err := os.MkdirTemp("", "memory-property-test-*")
			if err != nil {
				t.Logf("Failed to create temp dir: %v", err)
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Initialize directory structure
			ds := NewDirectoryStructure(filepath.Join(tmpDir, "memory"))
			if err := ds.Initialize(); err != nil {
				t.Logf("Failed to initialize directory structure: %v", err)
				return false
			}

			// Create preferences store
			store, err := NewPreferencesStore(ds.GetUserPreferencesDir())
			if err != nil {
				t.Logf("Failed to create preferences store: %v", err)
				return false
			}
			defer store.Close()

			// Create a successful routing decision
			decision := &RoutingDecision{
				Timestamp:  time.Now(),
				APIKeyHash: apiKeyHash,
				Request: RequestInfo{
					Intent: intent,
				},
				Routing: RoutingInfo{
					SelectedModel: model,
				},
				Outcome: OutcomeInfo{
					Success:      true,
					QualityScore: 0.9,
				},
			}

			// Learn from the decision
			if err := store.LearnFromOutcome(decision); err != nil {
				t.Logf("Failed to learn from outcome: %v", err)
				return false
			}

			// Get preferences
			prefs, err := store.GetUserPreferences(apiKeyHash)
			if err != nil {
				t.Logf("Failed to get preferences: %v", err)
				return false
			}

			// Verify that the successful model was learned for this intent
			if learnedModel, exists := prefs.ModelPreferences[intent]; exists {
				if learnedModel != model {
					t.Logf("Expected learned model %s for intent %s, got %s", model, intent, learnedModel)
					return false
				}
			} else {
				t.Logf("No model preference learned for intent %s", intent)
				return false
			}

			// Verify provider bias was adjusted positively
			provider := extractProviderFromModel(model)
			if provider != "" {
				if bias, exists := prefs.ProviderBias[provider]; exists {
					if bias <= 0 {
						t.Logf("Expected positive bias for successful provider %s, got %f", provider, bias)
						return false
					}
				}
			}

			return true
		},
		gen.OneConstOf("coding", "reasoning", "creative", "analysis"),
		gen.OneConstOf("claudecli:claude-sonnet-4", "geminicli:gemini-2.5-pro", "ollama:codellama"),
		genAPIKeyHash(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestProperty_QuirkStorageAndApplication tests Property 4: Quirk Storage and Application
// Feature: clawd-patterns-integration, Property 4: Quirk Storage and Application
// Validates: Requirements FR-1.4
func TestProperty_QuirkStorageAndApplication(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("all quirks are stored with required fields and retrievable", prop.ForAll(
		func(quirk *Quirk) bool {
			// Create temporary directory for this test
			tmpDir, err := os.MkdirTemp("", "memory-property-test-*")
			if err != nil {
				t.Logf("Failed to create temp dir: %v", err)
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Initialize directory structure
			ds := NewDirectoryStructure(filepath.Join(tmpDir, "memory"))
			if err := ds.Initialize(); err != nil {
				t.Logf("Failed to initialize directory structure: %v", err)
				return false
			}

			// Create quirks store
			store, err := NewQuirksStore(ds.GetProviderQuirksPath())
			if err != nil {
				t.Logf("Failed to create quirks store: %v", err)
				return false
			}
			defer store.Close()

			// Add the quirk
			if err := store.AddQuirk(quirk); err != nil {
				t.Logf("Failed to add quirk: %v", err)
				return false
			}

			// Verify quirk can be retrieved
			quirks, err := store.GetProviderQuirks(quirk.Provider)
			if err != nil {
				t.Logf("Failed to get provider quirks: %v", err)
				return false
			}

			// Should have at least one quirk
			if len(quirks) == 0 {
				t.Logf("Expected at least 1 quirk, got 0")
				return false
			}

			// Find our quirk in the results
			found := false
			for _, retrieved := range quirks {
				if retrieved.Provider == quirk.Provider &&
					retrieved.Issue == quirk.Issue &&
					retrieved.Workaround == quirk.Workaround &&
					retrieved.Frequency == quirk.Frequency &&
					retrieved.Severity == quirk.Severity {
					found = true
					break
				}
			}

			if !found {
				t.Logf("Quirk not found in retrieved results")
				return false
			}

			// Test automatic workaround application
			workaround, err := store.ApplyWorkaround(quirk.Provider, quirk.Issue)
			if err != nil {
				t.Logf("Failed to apply workaround: %v", err)
				return false
			}

			if workaround != quirk.Workaround {
				t.Logf("Workaround mismatch: expected %s, got %s", quirk.Workaround, workaround)
				return false
			}

			return true
		},
		genQuirk(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// genRoutingDecision generates random RoutingDecision instances for property testing
func genRoutingDecision() gopter.Gen {
	return gopter.CombineGens(
		genAPIKeyHash(),
		genRequestInfo(),
		genRoutingInfo(),
		genOutcomeInfo(),
	).Map(func(values []interface{}) *RoutingDecision {
		return &RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: values[0].(string),
			Request:    values[1].(RequestInfo),
			Routing:    values[2].(RoutingInfo),
			Outcome:    values[3].(OutcomeInfo),
		}
	})
}

// genAPIKeyHash generates random API key hashes
func genAPIKeyHash() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	}).Map(func(s string) string {
		hash := sha256.Sum256([]byte(s))
		return fmt.Sprintf("sha256:%x", hash)
	})
}

// genRequestInfo generates random RequestInfo instances
func genRequestInfo() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("auto", "gpt-4", "claude-3", "gemini-pro"),
		gen.OneConstOf("coding", "reasoning", "creative", "analysis"),
		genContentHash(),
		gen.IntRange(1, 10000),
	).Map(func(values []interface{}) RequestInfo {
		return RequestInfo{
			Model:         values[0].(string),
			Intent:        values[1].(string),
			ContentHash:   values[2].(string),
			ContentLength: values[3].(int),
		}
	})
}

// genRoutingInfo generates random RoutingInfo instances
func genRoutingInfo() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("reflex", "semantic", "cognitive", "learned"),
		gen.OneConstOf("claudecli:claude-sonnet-4", "geminicli:gemini-2.5-pro", "ollama:codellama", "openai:gpt-4"),
		gen.Float64Range(0.0, 1.0),
		gen.Int64Range(1, 1000),
	).Map(func(values []interface{}) RoutingInfo {
		return RoutingInfo{
			Tier:          values[0].(string),
			SelectedModel: values[1].(string),
			Confidence:    values[2].(float64),
			LatencyMs:     values[3].(int64),
		}
	})
}

// genOutcomeInfo generates random OutcomeInfo instances
func genOutcomeInfo() gopter.Gen {
	return gopter.CombineGens(
		gen.Bool(),
		gen.Int64Range(100, 10000),
		gen.OneConstOf("", "timeout", "rate_limit", "invalid_response"),
		gen.Float64Range(0.0, 1.0),
	).Map(func(values []interface{}) OutcomeInfo {
		success := values[0].(bool)
		errorMsg := values[2].(string)
		
		// If success is true, clear error message
		if success {
			errorMsg = ""
		}
		
		return OutcomeInfo{
			Success:        success,
			ResponseTimeMs: values[1].(int64),
			Error:          errorMsg,
			QualityScore:   values[3].(float64),
		}
	})
}

// genContentHash generates random content hashes
func genContentHash() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	}).Map(func(s string) string {
		hash := sha256.Sum256([]byte(s))
		return fmt.Sprintf("sha256:%x", hash)
	})
}

// extractProviderFromModel extracts the provider name from a model string.
// E.g., "claudecli:claude-sonnet-4" -> "claudecli"
func extractProviderFromModel(model string) string {
	parts := strings.SplitN(model, ":", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// genQuirk generates random Quirk instances for property testing
func genQuirk() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("ollama", "gemini", "claude", "openai", "lmstudio"),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.OneConstOf("always", "3/10 startups", "daily during peak", "rarely", "sometimes"),
		gen.OneConstOf("low", "medium", "high", "critical"),
	).Map(func(values []interface{}) *Quirk {
		return &Quirk{
			Provider:   values[0].(string),
			Issue:      values[1].(string),
			Workaround: values[2].(string),
			Discovered: time.Now(),
			Frequency:  values[3].(string),
			Severity:   values[4].(string),
		}
	})
}