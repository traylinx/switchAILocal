package steering

import "time"

// SteeringRule represents a single routing steering rule.
type SteeringRule struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Activation  ActivationRule    `yaml:"activation" json:"activation"`
	Preferences RoutePreferences  `yaml:"preferences" json:"preferences"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	// FilePath is the source file of the rule (not in YAML)
	FilePath string `yaml:"-" json:"-"`
}

// ActivationRule defines when a rule should be triggered.
type ActivationRule struct {
	Condition string `yaml:"condition" json:"condition"` // Expression: "intent == 'coding'"
	Priority  int    `yaml:"priority" json:"priority"`   // Higher = more important
}

// RoutePreferences defines the routing overrides and modifications.
type RoutePreferences struct {
	PrimaryModel     string          `yaml:"primary_model,omitempty" json:"primary_model,omitempty"`
	FallbackModels   []string        `yaml:"fallback_models,omitempty" json:"fallback_models,omitempty"`
	OverrideRouter   bool            `yaml:"override_router" json:"override_router"`
	ContextInjection string          `yaml:"context_injection,omitempty" json:"context_injection,omitempty"`
	ProviderSettings map[string]any  `yaml:"provider_settings,omitempty" json:"provider_settings,omitempty"`
	TimeBasedRules   []TimeBasedRule `yaml:"time_based_rules,omitempty" json:"time_based_rules,omitempty"`
}

// TimeBasedRule enables time-specific overrides.
type TimeBasedRule struct {
	Hours       string `yaml:"hours,omitempty" json:"hours,omitempty"` // "9-11" or "9:00-11:00"
	Days        string `yaml:"days,omitempty" json:"days,omitempty"`   // "Mon-Fri" or "1,2,3,4,5"
	PreferModel string `yaml:"prefer_model,omitempty" json:"prefer_model,omitempty"`
	Reason      string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// RoutingContext provides the environment for condition evaluation.
type RoutingContext struct {
	Intent        string                 `json:"intent"`
	Provider      string                 `json:"provider,omitempty"`
	Model         string                 `json:"model,omitempty"`
	APIKeyHash    string                 `json:"api_key_hash,omitempty"`
	ContentLength int                    `json:"content_length"`
	Hour          int                    `json:"hour"`
	DayOfWeek     string                 `json:"day_of_week"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
}
