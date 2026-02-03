package memory

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestNewPreferencesStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	if store.baseDir != tmpDir {
		t.Errorf("Expected base dir %s, got %s", tmpDir, store.baseDir)
	}

	// Verify directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("Base directory was not created")
	}
}

func TestGetUserPreferences_NewUser(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	prefs, err := store.GetUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to get user preferences: %v", err)
	}

	// Should return default preferences for new user
	if prefs.APIKeyHash != apiKeyHash {
		t.Errorf("Expected API key hash %s, got %s", apiKeyHash, prefs.APIKeyHash)
	}

	if prefs.ModelPreferences == nil {
		t.Errorf("Model preferences should be initialized")
	}

	if prefs.ProviderBias == nil {
		t.Errorf("Provider bias should be initialized")
	}

	if prefs.CustomRules == nil {
		t.Errorf("Custom rules should be initialized")
	}
}

func TestGetUserPreferences_InvalidAPIKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	// Test empty API key
	_, err = store.GetUserPreferences("")
	if err == nil {
		t.Errorf("Expected error for empty API key")
	}

	// Test invalid format
	_, err = store.GetUserPreferences("invalid-format")
	if err == nil {
		t.Errorf("Expected error for invalid API key format")
	}
}

func TestUpdatePreferences(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	prefs := &UserPreferences{
		APIKeyHash:       apiKeyHash,
		LastUpdated:      time.Now(),
		ModelPreferences: map[string]string{"coding": "claudecli:claude-sonnet-4"},
		ProviderBias:     map[string]float64{"claudecli": 0.5},
		CustomRules: []PreferenceRule{
			{
				Condition: "intent == 'coding'",
				Model:     "claudecli:claude-sonnet-4",
				Priority:  100,
			},
		},
	}

	err = store.UpdatePreferences(apiKeyHash, prefs)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// Verify file was created
	filePath := store.getFilePath(apiKeyHash)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Preferences file was not created")
	}

	// Retrieve and verify
	retrieved, err := store.GetUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to retrieve preferences: %v", err)
	}

	if retrieved.ModelPreferences["coding"] != "claudecli:claude-sonnet-4" {
		t.Errorf("Model preference not saved correctly")
	}

	if retrieved.ProviderBias["claudecli"] != 0.5 {
		t.Errorf("Provider bias not saved correctly")
	}

	if len(retrieved.CustomRules) != 1 {
		t.Errorf("Expected 1 custom rule, got %d", len(retrieved.CustomRules))
	}
}

func TestUpdatePreferences_InvalidInput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	// Test nil preferences
	err = store.UpdatePreferences("sha256:abc123", nil)
	if err == nil {
		t.Errorf("Expected error for nil preferences")
	}

	// Test empty API key
	prefs := &UserPreferences{}
	err = store.UpdatePreferences("", prefs)
	if err == nil {
		t.Errorf("Expected error for empty API key")
	}

	// Test invalid bias values
	prefs = &UserPreferences{
		APIKeyHash:   "sha256:abc123",
		ProviderBias: map[string]float64{"test": 2.0}, // Out of range
	}
	err = store.UpdatePreferences("sha256:abc123", prefs)
	if err == nil {
		t.Errorf("Expected error for out-of-range bias")
	}
}

func TestLearnFromOutcome_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: apiKeyHash,
		Request: RequestInfo{
			Intent: "coding",
		},
		Routing: RoutingInfo{
			SelectedModel: "claudecli:claude-sonnet-4",
		},
		Outcome: OutcomeInfo{
			Success:      true,
			QualityScore: 0.9,
		},
	}

	err = store.LearnFromOutcome(decision)
	if err != nil {
		t.Fatalf("Failed to learn from outcome: %v", err)
	}

	// Verify preferences were updated
	prefs, err := store.GetUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Should have learned model preference
	if prefs.ModelPreferences["coding"] != "claudecli:claude-sonnet-4" {
		t.Errorf("Expected model preference for coding to be claudecli:claude-sonnet-4, got %s", prefs.ModelPreferences["coding"])
	}

	// Should have positive bias for claudecli
	if prefs.ProviderBias["claudecli"] <= 0 {
		t.Errorf("Expected positive bias for claudecli, got %f", prefs.ProviderBias["claudecli"])
	}
}

func TestLearnFromOutcome_Failure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: apiKeyHash,
		Request: RequestInfo{
			Intent: "coding",
		},
		Routing: RoutingInfo{
			SelectedModel: "claudecli:claude-sonnet-4",
		},
		Outcome: OutcomeInfo{
			Success: false,
			Error:   "timeout",
		},
	}

	err = store.LearnFromOutcome(decision)
	if err != nil {
		t.Fatalf("Failed to learn from outcome: %v", err)
	}

	// Verify preferences were updated
	prefs, err := store.GetUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Should NOT have learned model preference for failed request
	if prefs.ModelPreferences["coding"] != "" {
		t.Errorf("Should not learn model preference from failed request")
	}

	// Should have negative bias for claudecli
	if prefs.ProviderBias["claudecli"] >= 0 {
		t.Errorf("Expected negative bias for claudecli, got %f", prefs.ProviderBias["claudecli"])
	}
}

func TestMergePreferences(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	base := &UserPreferences{
		APIKeyHash:       "sha256:abc123",
		ModelPreferences: map[string]string{"coding": "claudecli:claude-sonnet-4"},
		ProviderBias:     map[string]float64{"claudecli": 0.3},
		CustomRules:      []PreferenceRule{{Condition: "base", Model: "base", Priority: 50}},
	}

	override := &UserPreferences{
		APIKeyHash:       "sha256:abc123",
		ModelPreferences: map[string]string{"reasoning": "geminicli:gemini-2.5-pro"},
		ProviderBias:     map[string]float64{"claudecli": 0.7, "geminicli": 0.5},
		CustomRules:      []PreferenceRule{{Condition: "override", Model: "override", Priority: 100}},
	}

	merged := store.MergePreferences(base, override)

	// Should have both model preferences
	if merged.ModelPreferences["coding"] != "claudecli:claude-sonnet-4" {
		t.Errorf("Base model preference not preserved")
	}
	if merged.ModelPreferences["reasoning"] != "geminicli:gemini-2.5-pro" {
		t.Errorf("Override model preference not applied")
	}

	// Provider bias should be averaged for conflicts
	expectedBias := (0.3 + 0.7) / 2.0
	if merged.ProviderBias["claudecli"] != expectedBias {
		t.Errorf("Expected averaged bias %f, got %f", expectedBias, merged.ProviderBias["claudecli"])
	}

	// Should have both custom rules, sorted by priority
	if len(merged.CustomRules) != 2 {
		t.Errorf("Expected 2 custom rules, got %d", len(merged.CustomRules))
	}
	if merged.CustomRules[0].Priority != 100 {
		t.Errorf("Rules not sorted by priority correctly")
	}
}

func TestGetPreferencesByIntent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	prefs := &UserPreferences{
		APIKeyHash:       apiKeyHash,
		ModelPreferences: map[string]string{"coding": "claudecli:claude-sonnet-4"},
	}

	err = store.UpdatePreferences(apiKeyHash, prefs)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// Test existing intent
	model, err := store.GetPreferencesByIntent(apiKeyHash, "coding")
	if err != nil {
		t.Fatalf("Failed to get preferences by intent: %v", err)
	}
	if model != "claudecli:claude-sonnet-4" {
		t.Errorf("Expected claudecli:claude-sonnet-4, got %s", model)
	}

	// Test non-existing intent
	model, err = store.GetPreferencesByIntent(apiKeyHash, "reasoning")
	if err != nil {
		t.Fatalf("Failed to get preferences by intent: %v", err)
	}
	if model != "" {
		t.Errorf("Expected empty string for non-existing intent, got %s", model)
	}
}

func TestGetProviderBias(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	prefs := &UserPreferences{
		APIKeyHash:   apiKeyHash,
		ProviderBias: map[string]float64{"claudecli": 0.5},
	}

	err = store.UpdatePreferences(apiKeyHash, prefs)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// Test existing provider
	bias, err := store.GetProviderBias(apiKeyHash, "claudecli")
	if err != nil {
		t.Fatalf("Failed to get provider bias: %v", err)
	}
	if bias != 0.5 {
		t.Errorf("Expected bias 0.5, got %f", bias)
	}

	// Test non-existing provider
	bias, err = store.GetProviderBias(apiKeyHash, "geminicli")
	if err != nil {
		t.Fatalf("Failed to get provider bias: %v", err)
	}
	if bias != 0.0 {
		t.Errorf("Expected bias 0.0 for non-existing provider, got %f", bias)
	}
}

func TestCacheExpiry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	// Set very short cache TTL for testing
	store.cacheTTL = 1 * time.Millisecond

	apiKeyHash := "sha256:abc123"
	prefs := &UserPreferences{
		APIKeyHash:       apiKeyHash,
		ModelPreferences: map[string]string{"coding": "claudecli:claude-sonnet-4"},
	}

	err = store.UpdatePreferences(apiKeyHash, prefs)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// First access should hit cache
	_, err = store.GetUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(2 * time.Millisecond)

	// Second access should reload from file (cache expired)
	_, err = store.GetUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to get preferences after cache expiry: %v", err)
	}
}

func TestClearCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash1 := "sha256:abc123"
	apiKeyHash2 := "sha256:def456"

	// Add some preferences to cache
	_, err = store.GetUserPreferences(apiKeyHash1)
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	_, err = store.GetUserPreferences(apiKeyHash2)
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Verify cache has entries
	store.mu.RLock()
	cacheSize := len(store.cache)
	store.mu.RUnlock()
	if cacheSize != 2 {
		t.Errorf("Expected 2 cache entries, got %d", cacheSize)
	}

	// Clear specific user cache
	store.ClearCache(apiKeyHash1)

	store.mu.RLock()
	cacheSize = len(store.cache)
	store.mu.RUnlock()
	if cacheSize != 1 {
		t.Errorf("Expected 1 cache entry after clearing one user, got %d", cacheSize)
	}

	// Clear all cache
	store.ClearCache("")

	store.mu.RLock()
	cacheSize = len(store.cache)
	store.mu.RUnlock()
	if cacheSize != 0 {
		t.Errorf("Expected 0 cache entries after clearing all, got %d", cacheSize)
	}
}

func TestCount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	// Initially should be 0
	count, err := store.Count()
	if err != nil {
		t.Fatalf("Failed to get count: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}

	// Add some preferences
	apiKeyHash1 := "sha256:abc123"
	apiKeyHash2 := "sha256:def456"

	prefs1 := &UserPreferences{APIKeyHash: apiKeyHash1}
	prefs2 := &UserPreferences{APIKeyHash: apiKeyHash2}

	err = store.UpdatePreferences(apiKeyHash1, prefs1)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}
	err = store.UpdatePreferences(apiKeyHash2, prefs2)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// Should now be 2
	count, err = store.Count()
	if err != nil {
		t.Fatalf("Failed to get count: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func TestListUsers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	// Add some preferences
	apiKeyHash1 := "sha256:abc123"
	apiKeyHash2 := "sha256:def456"

	prefs1 := &UserPreferences{APIKeyHash: apiKeyHash1}
	prefs2 := &UserPreferences{APIKeyHash: apiKeyHash2}

	err = store.UpdatePreferences(apiKeyHash1, prefs1)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}
	err = store.UpdatePreferences(apiKeyHash2, prefs2)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// List users
	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("Failed to list users: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}

	// Check that both users are in the list
	found1, found2 := false, false
	for _, user := range users {
		if user == apiKeyHash1 {
			found1 = true
		}
		if user == apiKeyHash2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("Not all users found in list: %v", users)
	}
}

func TestDeleteUserPreferences(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	prefs := &UserPreferences{APIKeyHash: apiKeyHash}

	// Create preferences
	err = store.UpdatePreferences(apiKeyHash, prefs)
	if err != nil {
		t.Fatalf("Failed to update preferences: %v", err)
	}

	// Verify file exists
	filePath := store.getFilePath(apiKeyHash)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Preferences file should exist")
	}

	// Delete preferences
	err = store.DeleteUserPreferences(apiKeyHash)
	if err != nil {
		t.Fatalf("Failed to delete preferences: %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("Preferences file should be deleted")
	}

	// Verify cache is cleared
	store.mu.RLock()
	_, exists := store.cache[apiKeyHash]
	store.mu.RUnlock()
	if exists {
		t.Errorf("Cache should be cleared for deleted user")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preferences-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewPreferencesStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create preferences store: %v", err)
	}
	defer store.Close()

	apiKeyHash := "sha256:abc123"
	
	// Run concurrent operations
	done := make(chan bool, 10)
	
	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 10; j++ {
				_, err := store.GetUserPreferences(apiKeyHash)
				if err != nil {
					t.Errorf("Concurrent read failed: %v", err)
				}
			}
		}()
	}
	
	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < 10; j++ {
				prefs := &UserPreferences{
					APIKeyHash:       apiKeyHash,
					ModelPreferences: map[string]string{fmt.Sprintf("intent%d", id): fmt.Sprintf("model%d", id)},
				}
				err := store.UpdatePreferences(apiKeyHash, prefs)
				if err != nil {
					t.Errorf("Concurrent write failed: %v", err)
				}
			}
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}