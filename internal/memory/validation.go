package memory

import (
	"fmt"
	"regexp"
	"strings"
	"syscall"
)

var (
	// API key hash must be sha256: followed by 64 hex characters
	apiKeyHashRegex = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	
	// Model name validation (provider:model format)
	modelNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+:[a-zA-Z0-9_.-]+$`)
)

// ValidateRoutingDecision validates all fields of a routing decision before storage.
func ValidateRoutingDecision(decision *RoutingDecision) error {
	if decision == nil {
		return fmt.Errorf("decision cannot be nil")
	}

	// Validate API key hash format
	if decision.APIKeyHash == "" {
		return fmt.Errorf("api_key_hash cannot be empty")
	}
	if !apiKeyHashRegex.MatchString(decision.APIKeyHash) {
		return fmt.Errorf("invalid api_key_hash format (must be sha256:64hexchars)")
	}

	// Validate request fields
	if err := validateRequestInfo(&decision.Request); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	// Validate routing fields
	if err := validateRoutingInfo(&decision.Routing); err != nil {
		return fmt.Errorf("invalid routing: %w", err)
	}

	// Validate outcome fields
	if err := validateOutcomeInfo(&decision.Outcome); err != nil {
		return fmt.Errorf("invalid outcome: %w", err)
	}

	return nil
}

// validateRequestInfo validates request information fields.
func validateRequestInfo(req *RequestInfo) error {
	// Validate intent
	if strings.ContainsAny(req.Intent, "\n\r\t") {
		return fmt.Errorf("intent cannot contain control characters")
	}
	if len(req.Intent) > 100 {
		return fmt.Errorf("intent too long (max 100 chars, got %d)", len(req.Intent))
	}

	// Validate model name
	if req.Model != "" && req.Model != "auto" {
		if strings.ContainsAny(req.Model, "\n\r\t") {
			return fmt.Errorf("model cannot contain control characters")
		}
		if len(req.Model) > 200 {
			return fmt.Errorf("model name too long (max 200 chars, got %d)", len(req.Model))
		}
	}

	// Validate content hash format (if present)
	if req.ContentHash != "" {
		if !strings.HasPrefix(req.ContentHash, "sha256:") {
			return fmt.Errorf("content_hash must start with 'sha256:'")
		}
		if len(req.ContentHash) != 71 { // "sha256:" + 64 hex chars
			return fmt.Errorf("invalid content_hash length")
		}
	}

	// Validate content length
	if req.ContentLength < 0 {
		return fmt.Errorf("content_length cannot be negative")
	}
	if req.ContentLength > 10*1024*1024 { // 10MB max
		return fmt.Errorf("content_length too large (max 10MB)")
	}

	return nil
}

// validateRoutingInfo validates routing information fields.
func validateRoutingInfo(routing *RoutingInfo) error {
	// Validate tier
	validTiers := map[string]bool{
		"reflex":    true,
		"semantic":  true,
		"cognitive": true,
		"learned":   true,
	}
	if !validTiers[routing.Tier] {
		return fmt.Errorf("invalid tier: %s (must be reflex, semantic, cognitive, or learned)", routing.Tier)
	}

	// Validate selected model
	if routing.SelectedModel == "" {
		return fmt.Errorf("selected_model cannot be empty")
	}
	if strings.ContainsAny(routing.SelectedModel, "\n\r\t") {
		return fmt.Errorf("selected_model cannot contain control characters")
	}
	if len(routing.SelectedModel) > 200 {
		return fmt.Errorf("selected_model too long (max 200 chars)")
	}
	// Validate model name format (provider:model)
	if !modelNameRegex.MatchString(routing.SelectedModel) {
		return fmt.Errorf("invalid selected_model format (must be provider:model)")
	}

	// Validate confidence range
	if routing.Confidence < 0.0 || routing.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0 (got %.2f)", routing.Confidence)
	}

	// Validate latency
	if routing.LatencyMs < 0 {
		return fmt.Errorf("latency_ms cannot be negative")
	}
	if routing.LatencyMs > 60000 { // 60 seconds max
		return fmt.Errorf("latency_ms too large (max 60000ms)")
	}

	return nil
}

// validateOutcomeInfo validates outcome information fields.
func validateOutcomeInfo(outcome *OutcomeInfo) error {
	// Validate response time
	if outcome.ResponseTimeMs < 0 {
		return fmt.Errorf("response_time_ms cannot be negative")
	}
	if outcome.ResponseTimeMs > 300000 { // 5 minutes max
		return fmt.Errorf("response_time_ms too large (max 300000ms)")
	}

	// Validate quality score range
	if outcome.QualityScore < 0.0 || outcome.QualityScore > 1.0 {
		return fmt.Errorf("quality_score must be between 0.0 and 1.0 (got %.2f)", outcome.QualityScore)
	}

	// Validate error message length
	if len(outcome.Error) > 1000 {
		return fmt.Errorf("error message too long (max 1000 chars)")
	}
	if strings.ContainsAny(outcome.Error, "\n\r") {
		// Replace newlines with spaces for JSONL compatibility
		outcome.Error = strings.ReplaceAll(outcome.Error, "\n", " ")
		outcome.Error = strings.ReplaceAll(outcome.Error, "\r", " ")
	}

	return nil
}

// ValidateUserPreferences validates user preferences before storage.
func ValidateUserPreferences(prefs *UserPreferences) error {
	if prefs == nil {
		return fmt.Errorf("preferences cannot be nil")
	}

	// Validate API key hash
	if !apiKeyHashRegex.MatchString(prefs.APIKeyHash) {
		return fmt.Errorf("invalid api_key_hash format")
	}

	// Validate model preferences
	for intent, model := range prefs.ModelPreferences {
		if strings.ContainsAny(intent, "\n\r\t") {
			return fmt.Errorf("intent '%s' contains control characters", intent)
		}
		if len(intent) > 100 {
			return fmt.Errorf("intent '%s' too long (max 100 chars)", intent)
		}
		if !modelNameRegex.MatchString(model) {
			return fmt.Errorf("invalid model format for intent '%s': %s", intent, model)
		}
	}

	// Validate model confidences
	for intent, confidence := range prefs.ModelConfidences {
		if confidence < 0.0 || confidence > 1.0 {
			return fmt.Errorf("invalid confidence for intent '%s': %.2f", intent, confidence)
		}
	}

	// Validate provider bias
	for provider, bias := range prefs.ProviderBias {
		if bias < -1.0 || bias > 1.0 {
			return fmt.Errorf("invalid bias for provider '%s': %.2f (must be -1.0 to 1.0)", provider, bias)
		}
	}

	// Validate custom rules
	for i, rule := range prefs.CustomRules {
		if rule.Condition == "" {
			return fmt.Errorf("custom rule %d has empty condition", i)
		}
		if len(rule.Condition) > 500 {
			return fmt.Errorf("custom rule %d condition too long (max 500 chars)", i)
		}
		if rule.Model == "" {
			return fmt.Errorf("custom rule %d has empty model", i)
		}
		if !modelNameRegex.MatchString(rule.Model) {
			return fmt.Errorf("custom rule %d has invalid model format: %s", i, rule.Model)
		}
	}

	return nil
}

// ValidateQuirk validates a provider quirk before storage.
func ValidateQuirk(quirk *Quirk) error {
	if quirk == nil {
		return fmt.Errorf("quirk cannot be nil")
	}

	// Validate provider name
	if quirk.Provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if strings.ContainsAny(quirk.Provider, "\n\r\t") {
		return fmt.Errorf("provider cannot contain control characters")
	}
	if len(quirk.Provider) > 100 {
		return fmt.Errorf("provider name too long (max 100 chars)")
	}

	// Validate issue description
	if quirk.Issue == "" {
		return fmt.Errorf("issue cannot be empty")
	}
	if len(quirk.Issue) > 500 {
		return fmt.Errorf("issue description too long (max 500 chars)")
	}

	// Validate workaround
	if quirk.Workaround == "" {
		return fmt.Errorf("workaround cannot be empty")
	}
	if len(quirk.Workaround) > 500 {
		return fmt.Errorf("workaround description too long (max 500 chars)")
	}

	// Validate frequency
	if len(quirk.Frequency) > 100 {
		return fmt.Errorf("frequency description too long (max 100 chars)")
	}

	// Validate severity
	validSeverities := map[string]bool{
		"low":      true,
		"medium":   true,
		"high":     true,
		"critical": true,
	}
	if !validSeverities[quirk.Severity] {
		return fmt.Errorf("invalid severity: %s (must be low, medium, high, or critical)", quirk.Severity)
	}

	return nil
}

// SanitizeString removes control characters and limits length.
func SanitizeString(s string, maxLen int) string {
	// Remove control characters
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)

	// Limit length
	if len(s) > maxLen {
		s = s[:maxLen]
	}

	return s
}

// CheckDiskSpace verifies sufficient disk space is available before writing.
// It requires at least 100MB free space plus the estimated write size.
func CheckDiskSpace(path string, requiredBytes int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		// If we can't check disk space, log warning but don't fail
		// (might be on a filesystem that doesn't support statfs)
		return nil
	}

	// Calculate available bytes
	availableBytes := int64(stat.Bavail * uint64(stat.Bsize))

	// Require at least 100MB free + required bytes
	minRequired := int64(100*1024*1024) + requiredBytes

	if availableBytes < minRequired {
		return fmt.Errorf("insufficient disk space: %d MB available, %d MB required",
			availableBytes/(1024*1024), minRequired/(1024*1024))
	}

	return nil
}
