package learning

import (
	"time"
)

// PreferenceModel represents a learned model of user preferences.
type PreferenceModel struct {
	UserID           string                      `json:"user_id"`
	LastUpdated      time.Time                   `json:"last_updated"`
	TotalRequests    int                         `json:"total_requests"`
	ModelPreferences map[string]*ModelPreference `json:"model_preferences"` // Key: Intent
	ProviderBias     map[string]float64          `json:"provider_bias"`     // Key: Provider, Value: -1.0 to 1.0
	TimePatterns     map[string]*TimePattern     `json:"time_patterns"`     // Key: "weekday" or "weekend"
}

// ModelPreference represents a specific preference for a model given an intent.
type ModelPreference struct {
	Model       string  `json:"model"`
	Confidence  float64 `json:"confidence"`
	UsageCount  int     `json:"usage_count"`
	SuccessRate float64 `json:"success_rate"`
}

// TimePattern represents usage patterns based on time of day.
type TimePattern struct {
	// Simple hourly distribution (0-23)
	HourlyUsage map[int]int `json:"hourly_usage"`
	// Peak hours where specific intents are more common
	PeakIntents map[int]string `json:"peak_intents"`
}

// AnalysisResult holds the result of a history analysis run.
type AnalysisResult struct {
	UserID           string
	Timestamp        time.Time
	RequestsAnalyzed int
	NewPreferences   *PreferenceModel
	Suggestions      []string
}
