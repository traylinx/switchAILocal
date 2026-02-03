package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PreferencesStore manages persistent storage of user preferences in JSON format.
// Each API key gets its own JSON file for preferences storage.
type PreferencesStore struct {
	baseDir     string
	mu          sync.RWMutex
	cache       map[string]*UserPreferences // apiKeyHash -> preferences
	cacheTTL    time.Duration
	cacheExpiry map[string]time.Time // apiKeyHash -> expiry time
}

// NewPreferencesStore creates a new user preferences store.
// It uses the provided directory to store per-API-key JSON files.
func NewPreferencesStore(baseDir string) (*PreferencesStore, error) {
	store := &PreferencesStore{
		baseDir:     baseDir,
		cache:       make(map[string]*UserPreferences),
		cacheTTL:    10 * time.Minute, // Cache for 10 minutes as per design
		cacheExpiry: make(map[string]time.Time),
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, dirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create preferences directory: %w", err)
	}

	return store, nil
}

// GetUserPreferences retrieves learned preferences for a specific API key.
// Returns default preferences if no preferences file exists for the API key.
func (ps *PreferencesStore) GetUserPreferences(apiKeyHash string) (*UserPreferences, error) {
	if apiKeyHash == "" {
		return nil, fmt.Errorf("API key hash cannot be empty")
	}

	// Validate API key hash format (should be sha256:...)
	if !strings.HasPrefix(apiKeyHash, "sha256:") {
		return nil, fmt.Errorf("invalid API key hash format: must start with 'sha256:'")
	}

	ps.mu.RLock()
	// Check cache first
	if prefs, ok := ps.cache[apiKeyHash]; ok {
		if expiry, exists := ps.cacheExpiry[apiKeyHash]; exists && time.Now().Before(expiry) {
			ps.mu.RUnlock()
			// Return a copy to prevent external modification
			return ps.copyPreferences(prefs), nil
		}
	}
	ps.mu.RUnlock()

	// Load from file
	prefs, err := ps.loadFromFile(apiKeyHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load preferences: %w", err)
	}

	// Update cache
	ps.mu.Lock()
	ps.cache[apiKeyHash] = prefs
	ps.cacheExpiry[apiKeyHash] = time.Now().Add(ps.cacheTTL)
	ps.mu.Unlock()

	return ps.copyPreferences(prefs), nil
}

// UpdatePreferences updates preferences for a specific API key.
// This method handles both creating new preferences and updating existing ones.
func (ps *PreferencesStore) UpdatePreferences(apiKeyHash string, prefs *UserPreferences) error {
	if apiKeyHash == "" {
		return fmt.Errorf("API key hash cannot be empty")
	}
	if prefs == nil {
		return fmt.Errorf("preferences cannot be nil")
	}

	// Validate API key hash format
	if !strings.HasPrefix(apiKeyHash, "sha256:") {
		return fmt.Errorf("invalid API key hash format: must start with 'sha256:'")
	}

	// Ensure the preferences have the correct API key hash
	prefs.APIKeyHash = apiKeyHash
	prefs.LastUpdated = time.Now()

	// Comprehensive validation
	if err := ValidateUserPreferences(prefs); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check disk space before writing (estimate 5KB per preferences file)
	prefPath := filepath.Join(ps.baseDir, apiKeyHash+".json")
	if err := CheckDiskSpace(prefPath, 5*1024); err != nil {
		return fmt.Errorf("cannot write preferences: %w", err)
	}

	// Save to file
	if err := ps.saveToFile(prefs); err != nil {
		return fmt.Errorf("failed to save preferences: %w", err)
	}

	// Update cache
	ps.mu.Lock()
	ps.cache[apiKeyHash] = ps.copyPreferences(prefs)
	ps.cacheExpiry[apiKeyHash] = time.Now().Add(ps.cacheTTL)
	ps.mu.Unlock()

	return nil
}

// LearnFromOutcome updates preferences based on routing outcomes.
// This is the core learning mechanism that adjusts preferences based on success/failure patterns.
func (ps *PreferencesStore) LearnFromOutcome(decision *RoutingDecision) error {
	if decision == nil {
		return fmt.Errorf("routing decision cannot be nil")
	}

	// Get current preferences
	prefs, err := ps.GetUserPreferences(decision.APIKeyHash)
	if err != nil {
		return fmt.Errorf("failed to get current preferences: %w", err)
	}

	// Initialize maps if nil
	if prefs.ModelPreferences == nil {
		prefs.ModelPreferences = make(map[string]string)
	}
	if prefs.ProviderBias == nil {
		prefs.ProviderBias = make(map[string]float64)
	}

	// Learn model preferences based on success
	if decision.Outcome.Success && decision.Request.Intent != "" {
		// If this was successful, strengthen preference for this model for this intent
		prefs.ModelPreferences[decision.Request.Intent] = decision.Routing.SelectedModel
	}

	// Learn provider bias based on success rate and quality
	provider := ps.extractProvider(decision.Routing.SelectedModel)
	if provider != "" {
		currentBias := prefs.ProviderBias[provider]

		// Adjust bias based on outcome (more conservative adjustments)
		if decision.Outcome.Success {
			// Successful request: increase bias slightly
			adjustment := 0.05 // Reduced from 0.1
			if decision.Outcome.QualityScore > 0.8 {
				adjustment = 0.08 // Reduced from 0.15
			}
			prefs.ProviderBias[provider] = ps.clampBias(currentBias + adjustment)
		} else {
			// Failed request: decrease bias
			adjustment := -0.05 // Reduced from -0.2
			if decision.Outcome.Error != "" {
				adjustment = -0.08 // Reduced from -0.3
			}
			prefs.ProviderBias[provider] = ps.clampBias(currentBias + adjustment)
		}
	}

	// Learn custom rules based on patterns
	if err := ps.learnCustomRules(prefs, decision); err != nil {
		return fmt.Errorf("failed to learn custom rules: %w", err)
	}

	// Save updated preferences
	return ps.UpdatePreferences(decision.APIKeyHash, prefs)
}

// MergePreferences merges two preference sets, with the second taking priority in conflicts.
// This is useful for combining learned preferences with user-defined preferences.
func (ps *PreferencesStore) MergePreferences(base, override *UserPreferences) *UserPreferences {
	if base == nil && override == nil {
		return ps.createDefaultPreferences("")
	}
	if base == nil {
		return ps.copyPreferences(override)
	}
	if override == nil {
		return ps.copyPreferences(base)
	}

	merged := ps.copyPreferences(base)

	// Override model preferences
	if override.ModelPreferences != nil {
		if merged.ModelPreferences == nil {
			merged.ModelPreferences = make(map[string]string)
		}
		for intent, model := range override.ModelPreferences {
			merged.ModelPreferences[intent] = model
		}
	}

	// Merge provider bias (average conflicting values)
	if override.ProviderBias != nil {
		if merged.ProviderBias == nil {
			merged.ProviderBias = make(map[string]float64)
		}
		for provider, bias := range override.ProviderBias {
			if existingBias, exists := merged.ProviderBias[provider]; exists {
				// Average the biases for conflict resolution
				merged.ProviderBias[provider] = (existingBias + bias) / 2.0
			} else {
				merged.ProviderBias[provider] = bias
			}
		}
	}

	// Merge custom rules (override rules with higher priority take precedence)
	if override.CustomRules != nil {
		merged.CustomRules = append(merged.CustomRules, override.CustomRules...)
		// Sort by priority (higher priority first)
		ps.sortRulesByPriority(merged.CustomRules)
	}

	// Use the most recent update time
	if override.LastUpdated.After(merged.LastUpdated) {
		merged.LastUpdated = override.LastUpdated
	}

	return merged
}

// GetPreferencesByIntent retrieves the preferred model for a specific intent.
// Returns empty string if no preference exists for the intent.
func (ps *PreferencesStore) GetPreferencesByIntent(apiKeyHash, intent string) (string, error) {
	prefs, err := ps.GetUserPreferences(apiKeyHash)
	if err != nil {
		return "", err
	}

	if prefs.ModelPreferences == nil {
		return "", nil
	}

	return prefs.ModelPreferences[intent], nil
}

// GetProviderBias retrieves the bias for a specific provider.
// Returns 0.0 if no bias exists for the provider.
func (ps *PreferencesStore) GetProviderBias(apiKeyHash, provider string) (float64, error) {
	prefs, err := ps.GetUserPreferences(apiKeyHash)
	if err != nil {
		return 0.0, err
	}

	if prefs.ProviderBias == nil {
		return 0.0, nil
	}

	return prefs.ProviderBias[provider], nil
}

// ClearCache clears the preferences cache for all users or a specific user.
func (ps *PreferencesStore) ClearCache(apiKeyHash string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if apiKeyHash == "" {
		// Clear all cache
		ps.cache = make(map[string]*UserPreferences)
		ps.cacheExpiry = make(map[string]time.Time)
	} else {
		// Clear specific user cache
		delete(ps.cache, apiKeyHash)
		delete(ps.cacheExpiry, apiKeyHash)
	}
}

// loadFromFile loads preferences from a JSON file.
func (ps *PreferencesStore) loadFromFile(apiKeyHash string) (*UserPreferences, error) {
	filePath := ps.getFilePath(apiKeyHash)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File doesn't exist, return default preferences
		return ps.createDefaultPreferences(apiKeyHash), nil
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read preferences file: %w", err)
	}

	// Parse JSON
	var prefs UserPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, fmt.Errorf("failed to parse preferences JSON: %w", err)
	}

	// Ensure API key hash matches
	prefs.APIKeyHash = apiKeyHash

	// Initialize maps if nil
	if prefs.ModelPreferences == nil {
		prefs.ModelPreferences = make(map[string]string)
	}
	if prefs.ProviderBias == nil {
		prefs.ProviderBias = make(map[string]float64)
	}
	if prefs.CustomRules == nil {
		prefs.CustomRules = []PreferenceRule{}
	}

	return &prefs, nil
}

// saveToFile saves preferences to a JSON file.
func (ps *PreferencesStore) saveToFile(prefs *UserPreferences) error {
	filePath := ps.getFilePath(prefs.APIKeyHash)

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal preferences to JSON: %w", err)
	}

	// Write to file with proper permissions
	if err := os.WriteFile(filePath, data, filePermissions); err != nil {
		return fmt.Errorf("failed to write preferences file: %w", err)
	}

	return nil
}

// getFilePath returns the file path for a specific API key's preferences.
func (ps *PreferencesStore) getFilePath(apiKeyHash string) string {
	// Remove the "sha256:" prefix for the filename to keep it clean
	filename := strings.TrimPrefix(apiKeyHash, "sha256:")
	return filepath.Join(ps.baseDir, fmt.Sprintf("%s.json", filename))
}

// createDefaultPreferences creates default preferences for a new user.
func (ps *PreferencesStore) createDefaultPreferences(apiKeyHash string) *UserPreferences {
	return &UserPreferences{
		APIKeyHash:       apiKeyHash,
		LastUpdated:      time.Now(),
		ModelPreferences: make(map[string]string),
		ProviderBias:     make(map[string]float64),
		CustomRules:      []PreferenceRule{},
	}
}

// copyPreferences creates a deep copy of preferences to prevent external modification.
func (ps *PreferencesStore) copyPreferences(prefs *UserPreferences) *UserPreferences {
	if prefs == nil {
		return nil
	}

	copy := &UserPreferences{
		APIKeyHash:  prefs.APIKeyHash,
		LastUpdated: prefs.LastUpdated,
	}

	// Copy model preferences
	if prefs.ModelPreferences != nil {
		copy.ModelPreferences = make(map[string]string)
		for k, v := range prefs.ModelPreferences {
			copy.ModelPreferences[k] = v
		}
	}

	// Copy provider bias
	if prefs.ProviderBias != nil {
		copy.ProviderBias = make(map[string]float64)
		for k, v := range prefs.ProviderBias {
			copy.ProviderBias[k] = v
		}
	}

	// Copy custom rules
	if prefs.CustomRules != nil {
		copy.CustomRules = make([]PreferenceRule, len(prefs.CustomRules))
		for i, rule := range prefs.CustomRules {
			copy.CustomRules[i] = PreferenceRule{
				Condition: rule.Condition,
				Model:     rule.Model,
				Priority:  rule.Priority,
			}
		}
	}

	return copy
}

// _validatePreferences validates that preferences are well-formed.
// Removed unused function to satisfy linter.

// extractProvider extracts the provider name from a model string.
// E.g., "claudecli:claude-sonnet-4" -> "claudecli"
func (ps *PreferencesStore) extractProvider(model string) string {
	parts := strings.SplitN(model, ":", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// clampBias ensures bias values stay within the valid range [-1.0, 1.0].
func (ps *PreferencesStore) clampBias(bias float64) float64 {
	if bias < -1.0 {
		return -1.0
	}
	if bias > 1.0 {
		return 1.0
	}
	return bias
}

// learnCustomRules learns custom rules based on routing patterns.
func (ps *PreferencesStore) learnCustomRules(prefs *UserPreferences, decision *RoutingDecision) error {
	// For now, implement basic time-based rule learning
	// More sophisticated rule learning can be added later

	if !decision.Outcome.Success {
		return nil // Only learn from successful decisions
	}

	hour := decision.Timestamp.Hour()
	intent := decision.Request.Intent
	model := decision.Routing.SelectedModel

	// Create a time-based rule if we see consistent patterns
	condition := fmt.Sprintf("intent == '%s' && hour >= %d && hour <= %d", intent, hour-1, hour+1)

	// Check if similar rule already exists
	for _, rule := range prefs.CustomRules {
		if strings.Contains(rule.Condition, fmt.Sprintf("intent == '%s'", intent)) &&
			strings.Contains(rule.Condition, "hour") {
			// Similar rule exists, don't add duplicate
			return nil
		}
	}

	// Add new rule with low priority (learned rules have lower priority than explicit rules)
	newRule := PreferenceRule{
		Condition: condition,
		Model:     model,
		Priority:  10, // Low priority for learned rules
	}

	prefs.CustomRules = append(prefs.CustomRules, newRule)
	ps.sortRulesByPriority(prefs.CustomRules)

	return nil
}

// sortRulesByPriority sorts custom rules by priority (higher priority first).
func (ps *PreferencesStore) sortRulesByPriority(rules []PreferenceRule) {
	// Simple bubble sort for small arrays
	n := len(rules)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if rules[j].Priority < rules[j+1].Priority {
				rules[j], rules[j+1] = rules[j+1], rules[j]
			}
		}
	}
}

// Count returns the total number of users with preferences.
func (ps *PreferencesStore) Count() (int, error) {
	files, err := os.ReadDir(ps.baseDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read preferences directory: %w", err)
	}

	count := 0
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			count++
		}
	}

	return count, nil
}

// ListUsers returns a list of API key hashes that have preferences.
func (ps *PreferencesStore) ListUsers() ([]string, error) {
	files, err := os.ReadDir(ps.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read preferences directory: %w", err)
	}

	var users []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			// Convert filename back to API key hash
			filename := strings.TrimSuffix(file.Name(), ".json")
			apiKeyHash := fmt.Sprintf("sha256:%s", filename)
			users = append(users, apiKeyHash)
		}
	}

	return users, nil
}

// DeleteUserPreferences removes preferences for a specific user.
func (ps *PreferencesStore) DeleteUserPreferences(apiKeyHash string) error {
	if apiKeyHash == "" {
		return fmt.Errorf("API key hash cannot be empty")
	}

	filePath := ps.getFilePath(apiKeyHash)

	// Remove from cache
	ps.ClearCache(apiKeyHash)

	// Remove file
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete preferences file: %w", err)
	}

	return nil
}

// Close gracefully shuts down the preferences store.
// Currently a no-op but provided for consistency with other stores.
func (ps *PreferencesStore) Close() error {
	return nil
}
