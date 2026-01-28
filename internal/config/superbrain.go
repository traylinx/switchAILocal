package config

import (
	"strings"
)

// SuperbrainConfig holds the intelligent orchestration and self-healing configuration.
// The Superbrain transforms switchAILocal from a passive proxy into an autonomous,
// self-healing AI gateway with real-time monitoring, failure diagnosis, and recovery.
type SuperbrainConfig struct {
	// Enabled toggles the entire Superbrain system. When false, the gateway operates
	// in legacy pass-through mode with no monitoring or healing.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Mode controls the operational mode of the Superbrain system.
	// Valid values:
	//   - "disabled": Superbrain is completely disabled (same as Enabled: false)
	//   - "observe": Monitor and log but take no autonomous actions
	//   - "diagnose": Diagnose failures and log proposed actions without executing them
	//   - "human-in-the-loop": Diagnose and queue actions for operator approval
	//   - "conservative": Enable autonomous healing for whitelisted safe actions only
	//   - "autopilot": Enable all autonomous healing capabilities
	Mode string `yaml:"mode" json:"mode"`

	// ComponentFlags provides fine-grained control over individual Superbrain components.
	// Each component can be independently enabled or disabled for gradual rollout.
	// When a component is disabled, its functionality is completely bypassed.
	ComponentFlags ComponentFlags `yaml:"component_flags" json:"component_flags"`

	// Overwatch configures real-time execution monitoring.
	Overwatch OverwatchConfig `yaml:"overwatch" json:"overwatch"`

	// Doctor configures AI-powered failure diagnosis.
	Doctor DoctorConfig `yaml:"doctor" json:"doctor"`

	// StdinInjection configures autonomous input injection for interactive prompts.
	StdinInjection StdinInjectionConfig `yaml:"stdin_injection" json:"stdin_injection"`

	// ContextSculptor configures pre-flight content optimization.
	ContextSculptor ContextSculptorConfig `yaml:"context_sculptor" json:"context_sculptor"`

	// Fallback configures intelligent failover routing.
	Fallback FallbackConfig `yaml:"fallback" json:"fallback"`

	// Consensus configures multi-model response verification.
	Consensus ConsensusConfig `yaml:"consensus" json:"consensus"`

	// Security configures audit logging and safety controls.
	Security SecurityConfig `yaml:"security" json:"security"`
}

// ComponentFlags provides fine-grained control over individual Superbrain components.
// This allows for gradual rollout and independent testing of each capability.
type ComponentFlags struct {
	// OverwatchEnabled toggles real-time execution monitoring.
	// When false, no process monitoring or silence detection occurs.
	OverwatchEnabled bool `yaml:"overwatch_enabled" json:"overwatch_enabled"`

	// DoctorEnabled toggles AI-powered failure diagnosis.
	// When false, failures are not analyzed and no diagnosis is performed.
	DoctorEnabled bool `yaml:"doctor_enabled" json:"doctor_enabled"`

	// InjectorEnabled toggles autonomous stdin injection.
	// When false, no automatic responses to interactive prompts occur.
	InjectorEnabled bool `yaml:"injector_enabled" json:"injector_enabled"`

	// RecoveryEnabled toggles process restart with corrective flags.
	// When false, failed processes are not automatically restarted.
	RecoveryEnabled bool `yaml:"recovery_enabled" json:"recovery_enabled"`

	// FallbackEnabled toggles intelligent failover routing.
	// When false, no automatic routing to alternative providers occurs.
	FallbackEnabled bool `yaml:"fallback_enabled" json:"fallback_enabled"`

	// SculptorEnabled toggles pre-flight content optimization.
	// When false, no token analysis or content optimization is performed.
	SculptorEnabled bool `yaml:"sculptor_enabled" json:"sculptor_enabled"`
}

// OverwatchConfig defines real-time execution monitoring parameters.
type OverwatchConfig struct {
	// SilenceThresholdMs is the duration in milliseconds of no output before
	// flagging an execution as potentially hung. Default: 30000 (30 seconds).
	SilenceThresholdMs int64 `yaml:"silence_threshold_ms" json:"silence_threshold_ms"`

	// LogBufferSize is the number of recent log lines to retain in the rolling buffer.
	// Default: 50 lines.
	LogBufferSize int `yaml:"log_buffer_size" json:"log_buffer_size"`

	// HeartbeatIntervalMs is the interval in milliseconds for checking process health.
	// Default: 1000 (1 second).
	HeartbeatIntervalMs int64 `yaml:"heartbeat_interval_ms" json:"heartbeat_interval_ms"`

	// MaxRestartAttempts is the maximum number of times to restart a failed process
	// before escalating to fallback routing. Default: 2.
	MaxRestartAttempts int `yaml:"max_restart_attempts" json:"max_restart_attempts"`
}

// DoctorConfig defines AI-powered failure diagnosis settings.
type DoctorConfig struct {
	// Model is the lightweight AI model used for failure diagnosis.
	// Default: "gemini-flash" for fast, cost-effective analysis.
	Model string `yaml:"model" json:"model"`

	// TimeoutMs is the maximum time in milliseconds to wait for diagnosis.
	// If exceeded, falls back to pattern-only diagnosis. Default: 5000 (5 seconds).
	TimeoutMs int64 `yaml:"timeout_ms" json:"timeout_ms"`
}

// StdinInjectionConfig defines autonomous input injection settings.
type StdinInjectionConfig struct {
	// Mode controls stdin injection behavior.
	// Valid values:
	//   - "disabled": Never inject stdin responses
	//   - "conservative": Only inject for explicitly whitelisted safe patterns
	//   - "autopilot": Automatically inject for all safe patterns
	Mode string `yaml:"mode" json:"mode"`

	// CustomPatterns defines additional prompt patterns to recognize and respond to.
	// Each pattern should include: name, regex, response, is_safe, description.
	CustomPatterns []StdinPattern `yaml:"custom_patterns,omitempty" json:"custom_patterns,omitempty"`

	// ForbiddenPatterns lists regex patterns that should never trigger automatic responses,
	// regardless of mode. Used to prevent dangerous operations like file deletion.
	ForbiddenPatterns []string `yaml:"forbidden_patterns,omitempty" json:"forbidden_patterns,omitempty"`
}

// StdinPattern defines a recognizable prompt pattern and its automatic response.
type StdinPattern struct {
	// Name is a unique identifier for this pattern.
	Name string `yaml:"name" json:"name"`

	// Regex is the regular expression pattern to match in process output.
	Regex string `yaml:"regex" json:"regex"`

	// Response is the text to inject into stdin when the pattern is matched.
	Response string `yaml:"response" json:"response"`

	// IsSafe indicates whether this pattern is safe for automatic injection in autopilot mode.
	IsSafe bool `yaml:"is_safe" json:"is_safe"`

	// Description provides human-readable context about what this pattern matches.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// ContextSculptorConfig defines pre-flight content optimization settings.
type ContextSculptorConfig struct {
	// Enabled toggles pre-flight token analysis and content optimization.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// TokenEstimator selects the token counting method.
	// Valid values: "tiktoken" (accurate), "simple" (fast approximation).
	// Default: "tiktoken".
	TokenEstimator string `yaml:"token_estimator" json:"token_estimator"`

	// PriorityFiles lists file patterns that should always be included when optimizing.
	// Supports glob patterns. Default: ["README.md", "main.go", "index.ts", "package.json"].
	PriorityFiles []string `yaml:"priority_files,omitempty" json:"priority_files,omitempty"`
}

// FallbackConfig defines intelligent failover routing settings.
type FallbackConfig struct {
	// Enabled toggles automatic failover to alternative providers.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Providers lists provider names in order of fallback preference.
	// When a provider fails, the next available provider in this list is tried.
	// Example: ["geminicli", "gemini", "ollama"]
	Providers []string `yaml:"providers,omitempty" json:"providers,omitempty"`

	// MinSuccessRate is the minimum historical success rate (0.0-1.0) required
	// for a provider to be considered for fallback routing. Default: 0.5.
	MinSuccessRate float64 `yaml:"min_success_rate" json:"min_success_rate"`
}

// ConsensusConfig defines multi-model response verification settings.
type ConsensusConfig struct {
	// Enabled toggles consensus verification. Disabled by default due to added latency and cost.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// VerificationModel is the model used to verify potentially incomplete responses.
	// Default: "gemini-flash".
	VerificationModel string `yaml:"verification_model" json:"verification_model"`

	// TriggerPatterns lists patterns that indicate a response may need verification.
	// Example: ["abrupt_ending", "missing_code_blocks"]
	TriggerPatterns []string `yaml:"trigger_patterns,omitempty" json:"trigger_patterns,omitempty"`
}

// SecurityConfig defines audit logging and safety control settings.
type SecurityConfig struct {
	// AuditLogEnabled toggles audit logging of all autonomous actions.
	AuditLogEnabled bool `yaml:"audit_log_enabled" json:"audit_log_enabled"`

	// AuditLogPath is the file path for the audit log.
	// Default: "./logs/superbrain_audit.log"
	AuditLogPath string `yaml:"audit_log_path" json:"audit_log_path"`

	// ForbiddenOperations lists operation types that require human approval
	// and should never be performed autonomously, even in autopilot mode.
	// Example: ["file_delete", "system_command"]
	ForbiddenOperations []string `yaml:"forbidden_operations,omitempty" json:"forbidden_operations,omitempty"`
}

// SanitizeSuperbrain validates and normalizes Superbrain configuration.
// It ensures mode values are valid and applies sensible constraints to numeric settings.
func (cfg *Config) SanitizeSuperbrain() {
	if cfg == nil {
		return
	}

	sb := &cfg.Superbrain

	// Normalize and validate mode
	sb.Mode = strings.ToLower(strings.TrimSpace(sb.Mode))
	validModes := map[string]bool{
		"disabled":          true,
		"observe":           true,
		"diagnose":          true,
		"human-in-the-loop": true,
		"conservative":      true,
		"autopilot":         true,
	}
	if !validModes[sb.Mode] {
		// Default to disabled for invalid modes
		sb.Mode = "disabled"
	}

	// If mode is disabled, set Enabled to false for consistency
	if sb.Mode == "disabled" {
		sb.Enabled = false
	}

	// Validate Overwatch settings
	if sb.Overwatch.SilenceThresholdMs < 1000 {
		sb.Overwatch.SilenceThresholdMs = 1000 // Minimum 1 second
	}
	if sb.Overwatch.LogBufferSize < 10 {
		sb.Overwatch.LogBufferSize = 10 // Minimum 10 lines
	}
	if sb.Overwatch.HeartbeatIntervalMs < 100 {
		sb.Overwatch.HeartbeatIntervalMs = 100 // Minimum 100ms
	}
	if sb.Overwatch.MaxRestartAttempts < 0 {
		sb.Overwatch.MaxRestartAttempts = 0
	}
	if sb.Overwatch.MaxRestartAttempts > 5 {
		sb.Overwatch.MaxRestartAttempts = 5 // Maximum 5 retries
	}

	// Validate Doctor settings
	sb.Doctor.Model = strings.TrimSpace(sb.Doctor.Model)
	if sb.Doctor.Model == "" {
		sb.Doctor.Model = "gemini-flash"
	}
	if sb.Doctor.TimeoutMs < 1000 {
		sb.Doctor.TimeoutMs = 1000 // Minimum 1 second
	}
	if sb.Doctor.TimeoutMs > 30000 {
		sb.Doctor.TimeoutMs = 30000 // Maximum 30 seconds
	}

	// Validate StdinInjection mode
	sb.StdinInjection.Mode = strings.ToLower(strings.TrimSpace(sb.StdinInjection.Mode))
	validInjectionModes := map[string]bool{
		"disabled":     true,
		"conservative": true,
		"autopilot":    true,
	}
	if !validInjectionModes[sb.StdinInjection.Mode] {
		sb.StdinInjection.Mode = "conservative"
	}

	// Normalize forbidden patterns
	if len(sb.StdinInjection.ForbiddenPatterns) > 0 {
		normalized := make([]string, 0, len(sb.StdinInjection.ForbiddenPatterns))
		for _, pattern := range sb.StdinInjection.ForbiddenPatterns {
			trimmed := strings.TrimSpace(pattern)
			if trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		sb.StdinInjection.ForbiddenPatterns = normalized
	}

	// Validate ContextSculptor settings
	sb.ContextSculptor.TokenEstimator = strings.ToLower(strings.TrimSpace(sb.ContextSculptor.TokenEstimator))
	if sb.ContextSculptor.TokenEstimator != "tiktoken" && sb.ContextSculptor.TokenEstimator != "simple" {
		sb.ContextSculptor.TokenEstimator = "tiktoken"
	}

	// Normalize priority files
	if len(sb.ContextSculptor.PriorityFiles) > 0 {
		normalized := make([]string, 0, len(sb.ContextSculptor.PriorityFiles))
		for _, file := range sb.ContextSculptor.PriorityFiles {
			trimmed := strings.TrimSpace(file)
			if trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		sb.ContextSculptor.PriorityFiles = normalized
	}

	// Validate Fallback settings
	if sb.Fallback.MinSuccessRate < 0.0 {
		sb.Fallback.MinSuccessRate = 0.0
	}
	if sb.Fallback.MinSuccessRate > 1.0 {
		sb.Fallback.MinSuccessRate = 1.0
	}

	// Normalize provider list
	if len(sb.Fallback.Providers) > 0 {
		normalized := make([]string, 0, len(sb.Fallback.Providers))
		seen := make(map[string]bool)
		for _, provider := range sb.Fallback.Providers {
			trimmed := strings.ToLower(strings.TrimSpace(provider))
			if trimmed != "" && !seen[trimmed] {
				normalized = append(normalized, trimmed)
				seen[trimmed] = true
			}
		}
		sb.Fallback.Providers = normalized
	}

	// Validate Consensus settings
	sb.Consensus.VerificationModel = strings.TrimSpace(sb.Consensus.VerificationModel)
	if sb.Consensus.VerificationModel == "" {
		sb.Consensus.VerificationModel = "gemini-flash"
	}

	// Normalize trigger patterns
	if len(sb.Consensus.TriggerPatterns) > 0 {
		normalized := make([]string, 0, len(sb.Consensus.TriggerPatterns))
		for _, pattern := range sb.Consensus.TriggerPatterns {
			trimmed := strings.TrimSpace(pattern)
			if trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		sb.Consensus.TriggerPatterns = normalized
	}

	// Validate Security settings
	sb.Security.AuditLogPath = strings.TrimSpace(sb.Security.AuditLogPath)
	if sb.Security.AuditLogPath == "" {
		sb.Security.AuditLogPath = "./logs/superbrain_audit.log"
	}

	// Normalize forbidden operations
	if len(sb.Security.ForbiddenOperations) > 0 {
		normalized := make([]string, 0, len(sb.Security.ForbiddenOperations))
		for _, op := range sb.Security.ForbiddenOperations {
			trimmed := strings.TrimSpace(op)
			if trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		sb.Security.ForbiddenOperations = normalized
	}
}
