// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package config provides configuration management for the switchAILocal server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const DefaultPanelGitHubRepository = "https://github.com/traylinx/switchAILocal-Management-Center"

// Config represents the application's configuration, loaded from a YAML file.
type Config struct {
	SDKConfig `yaml:",inline"`
	// Host is the network host/interface on which the API server will bind.
	// Default is empty ("") to bind all interfaces (IPv4 + IPv6). Use "127.0.0.1" or "localhost" for local-only access.
	Host string `yaml:"host" json:"-"`
	// Port is the network port on which the API server will listen.
	Port int `yaml:"port" json:"-"`

	// TLS config controls HTTPS server settings.
	TLS TLSConfig `yaml:"tls" json:"tls"`

	// RemoteManagement nests management-related options under 'remote-management'.
	RemoteManagement RemoteManagement `yaml:"remote-management" json:"-"`

	// AuthDir is the directory where authentication token files are stored.
	AuthDir string `yaml:"auth-dir" json:"-"`

	// Debug enables or disables debug-level logging and other debug features.
	Debug bool `yaml:"debug" json:"debug"`

	// LoggingToFile controls whether application logs are written to rotating files or stdout.
	LoggingToFile bool `yaml:"logging-to-file" json:"logging-to-file"`

	// LogsMaxTotalSizeMB limits the total size (in MB) of log files under the logs directory.
	// When exceeded, the oldest log files are deleted until within the limit. Set to 0 to disable.
	LogsMaxTotalSizeMB int `yaml:"logs-max-total-size-mb" json:"logs-max-total-size-mb"`

	// UsageStatisticsEnabled toggles in-memory usage aggregation; when false, usage data is discarded.
	UsageStatisticsEnabled bool `yaml:"usage-statistics-enabled" json:"usage-statistics-enabled"`

	// DisableCooling disables quota cooldown scheduling when true.
	DisableCooling bool `yaml:"disable-cooling" json:"disable-cooling"`

	// RequestRetry defines the retry times when the request failed.
	RequestRetry int `yaml:"request-retry" json:"request-retry"`
	// MaxRetryInterval defines the maximum wait time in seconds before retrying a cooled-down credential.
	MaxRetryInterval int `yaml:"max-retry-interval" json:"max-retry-interval"`

	// QuotaExceeded defines the behavior when a quota is exceeded.
	QuotaExceeded QuotaExceeded `yaml:"quota-exceeded" json:"quota-exceeded"`

	// Routing controls credential selection behavior.
	Routing RoutingConfig `yaml:"routing" json:"routing"`

	// WebsocketAuth enables or disables authentication for the WebSocket API.
	WebsocketAuth bool `yaml:"ws-auth" json:"ws-auth"`

	// GeminiKey defines Gemini API key configurations with optional routing overrides.
	GeminiKey []GeminiKey `yaml:"gemini-api-key" json:"gemini-api-key"`

	// Codex defines a list of Codex API key configurations as specified in the YAML configuration file.
	CodexKey []CodexKey `yaml:"codex-api-key" json:"codex-api-key"`

	// ClaudeKey defines a list of Claude API key configurations as specified in the YAML configuration file.
	ClaudeKey []ClaudeKey `yaml:"claude-api-key" json:"claude-api-key"`

	// SwitchAIKey defines a list of Traylinx switchAI API key configurations as specified in the YAML configuration file.
	SwitchAIKey []SwitchAIKey `yaml:"switchai-api-key" json:"switchai-api-key"`

	// OpenAICompatibility defines OpenAI API compatibility configurations for external providers.
	OpenAICompatibility []OpenAICompatibility `yaml:"openai-compatibility" json:"openai-compatibility"`

	// VertexCompatAPIKey defines Vertex AI-compatible API key configurations for third-party providers.
	// Used for services that use Vertex AI-style paths but with simple API key authentication.
	VertexCompatAPIKey []VertexCompatKey `yaml:"vertex-api-key" json:"vertex-api-key"`

	// AmpCode contains Amp CLI upstream configuration, management restrictions, and model mappings.
	AmpCode AmpCode `yaml:"ampcode" json:"ampcode"`

	// OAuthExcludedModels defines per-provider global model exclusions applied to OAuth/file-backed auth entries.
	OAuthExcludedModels map[string][]string `yaml:"oauth-excluded-models,omitempty" json:"oauth-excluded-models,omitempty"`

	// Ollama configures the local Ollama server integration.
	Ollama OllamaConfig `yaml:"ollama" json:"ollama"`

	// LMStudio configures the local LM Studio server integration.
	LMStudio LMStudioConfig `yaml:"lmstudio" json:"lmstudio"`

	// OpenCode configures the local OpenCode server integration.
	OpenCode OpenCodeConfig `yaml:"opencode" json:"opencode"`

	// Payload defines default and override rules for provider payload parameters.
	Payload PayloadConfig `yaml:"payload" json:"payload"`

	// Plugin configures the LUA plugin system.
	Plugin PluginConfig `yaml:"plugin" json:"plugin"`

	// Superbrain configures the intelligent orchestration and self-healing capabilities.
	Superbrain SuperbrainConfig `yaml:"superbrain" json:"superbrain"`

	legacyMigrationPending bool `yaml:"-" json:"-"`
}

// PluginConfig holds LUA plugin system settings.
type PluginConfig struct {
	// Enabled toggles the LUA plugin system.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// PluginDir is the directory containing LUA scripts.
	PluginDir string `yaml:"plugin-dir" json:"plugin-dir"`
}

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

// OllamaConfig holds local Ollama server settings.
type OllamaConfig struct {
	// Enabled toggles Ollama provider registration.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// BaseURL is the Ollama API endpoint. Default: http://localhost:11434
	BaseURL string `yaml:"base-url" json:"base-url"`

	// AutoDiscover when true, fetches available models from Ollama on startup.
	AutoDiscover bool `yaml:"auto-discover" json:"auto-discover"`

	// ProxyURL optionally overrides the global proxy for this provider.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`

	// ModelsURL overrides the default models discovery endpoint.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// Headers optionally adds extra HTTP headers for requests.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`

	// Models defines manual model aliases.
	Models []OpenAICompatibilityModel `yaml:"models,omitempty" json:"models,omitempty"`
}

// LMStudioConfig holds local LM Studio server settings.
type LMStudioConfig struct {
	// Enabled toggles LM Studio provider registration.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// BaseURL is the LM Studio API endpoint. Default: http://localhost:1234/v1
	BaseURL string `yaml:"base-url" json:"base-url"`

	// AutoDiscover when true, fetches available models from LM Studio on startup.
	AutoDiscover bool `yaml:"auto-discover" json:"auto-discover"`

	// ProxyURL optionally overrides the global proxy for this provider.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`

	// ModelsURL overrides the default models discovery endpoint.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// Headers optionally adds extra HTTP headers for requests.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`

	// Models defines manual model aliases.
	Models []OpenAICompatibilityModel `yaml:"models,omitempty" json:"models,omitempty"`
}

// OpenCodeConfig holds local OpenCode server settings.
type OpenCodeConfig struct {
	// Enabled toggles OpenCode provider integration.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// BaseURL is the OpenCode API endpoint. Default: http://localhost:4096
	BaseURL string `yaml:"base-url" json:"base-url"`

	// DefaultAgent is the default agent to use if no specific model is requested.
	DefaultAgent string `yaml:"default-agent" json:"default-agent"`
}

// TLSConfig holds HTTPS server settings.
type TLSConfig struct {
	// Enable toggles HTTPS server mode.
	Enable bool `yaml:"enable" json:"enable"`
	// Cert is the path to the TLS certificate file.
	Cert string `yaml:"cert" json:"cert"`
	// Key is the path to the TLS private key file.
	Key string `yaml:"key" json:"key"`
}

// RemoteManagement holds management API configuration under 'remote-management'.
type RemoteManagement struct {
	// AllowRemote toggles remote (non-localhost) access to management API.
	AllowRemote bool `yaml:"allow-remote"`
	// SecretKey is the management key (plaintext or bcrypt hashed). YAML key intentionally 'secret-key'.
	SecretKey string `yaml:"secret-key"`
	// DisableControlPanel skips serving and syncing the bundled management UI when true.
	DisableControlPanel bool `yaml:"disable-control-panel"`
	// PanelGitHubRepository overrides the GitHub repository used to fetch the management panel asset.
	// Accepts either a repository URL (https://github.com/org/repo) or an API releases endpoint.
	PanelGitHubRepository string `yaml:"panel-github-repository"`
}

// QuotaExceeded defines the behavior when API quota limits are exceeded.
// It provides configuration options for automatic failover mechanisms.
type QuotaExceeded struct {
	// SwitchProject indicates whether to automatically switch to another project when a quota is exceeded.
	SwitchProject bool `yaml:"switch-project" json:"switch-project"`

	// SwitchPreviewModel indicates whether to automatically switch to a preview model when a quota is exceeded.
	SwitchPreviewModel bool `yaml:"switch-preview-model" json:"switch-preview-model"`
}

// RoutingConfig configures how credentials are selected for requests.
type RoutingConfig struct {
	// Strategy selects the credential selection strategy.
	// Supported values: "round-robin" (default), "fill-first".
	Strategy string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

// AmpModelMapping defines a model name mapping for Amp CLI requests.
// When Amp requests a model that isn't available locally, this mapping
// allows routing to an alternative model that IS available.
type AmpModelMapping struct {
	// From is the model name that Amp CLI requests (e.g., "claude-opus-4.5").
	From string `yaml:"from" json:"from"`

	// To is the target model name to route to (e.g., "claude-sonnet-4").
	// The target model must have available providers in the registry.
	To string `yaml:"to" json:"to"`

	// Regex indicates whether the 'from' field should be interpreted as a regular
	// expression for matching model names. When true, this mapping is evaluated
	// after exact matches and in the order provided. Defaults to false (exact match).
	Regex bool `yaml:"regex,omitempty" json:"regex,omitempty"`
}

// AmpCode groups Amp CLI integration settings including upstream routing,
// optional overrides, management route restrictions, and model fallback mappings.
type AmpCode struct {
	// UpstreamURL defines the upstream Amp control plane used for non-provider calls.
	UpstreamURL string `yaml:"upstream-url" json:"upstream-url"`

	// UpstreamAPIKey optionally overrides the Authorization header when proxying Amp upstream calls.
	UpstreamAPIKey string `yaml:"upstream-api-key" json:"upstream-api-key"`

	// RestrictManagementToLocalhost restricts Amp management routes (/api/user, /api/threads, etc.)
	// to only accept connections from localhost (127.0.0.1, ::1). When true, prevents drive-by
	// browser attacks and remote access to management endpoints. Default: false (API key auth is sufficient).
	RestrictManagementToLocalhost bool `yaml:"restrict-management-to-localhost" json:"restrict-management-to-localhost"`

	// ModelMappings defines model name mappings for Amp CLI requests.
	// When Amp requests a model that isn't available locally, these mappings
	// allow routing to an alternative model that IS available.
	ModelMappings []AmpModelMapping `yaml:"model-mappings" json:"model-mappings"`

	// ForceModelMappings when true, model mappings take precedence over local API keys.
	// When false (default), local API keys are used first if available.
	ForceModelMappings bool `yaml:"force-model-mappings" json:"force-model-mappings"`
}

// PayloadConfig defines default and override parameter rules applied to provider payloads.
type PayloadConfig struct {
	// Default defines rules that only set parameters when they are missing in the payload.
	Default []PayloadRule `yaml:"default" json:"default"`
	// Override defines rules that always set parameters, overwriting any existing values.
	Override []PayloadRule `yaml:"override" json:"override"`
}

// PayloadRule describes a single rule targeting a list of models with parameter updates.
type PayloadRule struct {
	// Models lists model entries with name pattern and protocol constraint.
	Models []PayloadModelRule `yaml:"models" json:"models"`
	// Params maps JSON paths (gjson/sjson syntax) to values written into the payload.
	Params map[string]any `yaml:"params" json:"params"`
}

// PayloadModelRule ties a model name pattern to a specific translator protocol.
type PayloadModelRule struct {
	// Name is the model name or wildcard pattern (e.g., "gpt-*", "*-5", "gemini-*-pro").
	Name string `yaml:"name" json:"name"`
	// Protocol restricts the rule to a specific translator format (e.g., "gemini", "responses").
	Protocol string `yaml:"protocol" json:"protocol"`
}

// ClaudeKey represents the configuration for a Claude API key,
// including the API key itself and an optional base URL for the API endpoint.
type ClaudeKey struct {
	// APIKey is the authentication key for accessing Claude API services.
	APIKey string `yaml:"api-key" json:"api-key"`

	// Prefix optionally namespaces models for this credential (e.g., "teamA/claude-sonnet-4").
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL is the base URL for the Claude API endpoint.
	// If empty, the default Claude API URL will be used.
	BaseURL string `yaml:"base-url" json:"base-url"`

	// ProxyURL overrides the global proxy setting for this API key if provided.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ModelsURL optionally overrides the endpoint used to discover available models.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// Models defines upstream model names and aliases for request routing.
	Models []ClaudeModel `yaml:"models" json:"models"`

	// Headers optionally adds extra HTTP headers for requests sent with this key.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded for this provider.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`
}

// ClaudeModel describes a mapping between an alias and the actual upstream model name.
type ClaudeModel struct {
	// Name is the upstream model identifier used when issuing requests.
	Name string `yaml:"name" json:"name"`

	// Alias is the client-facing model name that maps to Name.
	Alias string `yaml:"alias" json:"alias"`
}

// SwitchAIKey represents the configuration for a Traylinx switchAI API key,
// including the API key itself and an optional base URL for the API endpoint.
type SwitchAIKey struct {
	// APIKey is the authentication key for accessing switchAI API services.
	APIKey string `yaml:"api-key" json:"api-key"`

	// Prefix optionally namespaces models for this credential (e.g., "teamA/deepseek").
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL is the base URL for the switchAI API endpoint.
	// Default: https://switchai.traylinx.com/v1
	BaseURL string `yaml:"base-url" json:"base-url"`

	// ProxyURL overrides the global proxy setting for this API key if provided.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ModelsURL optionally overrides the endpoint used to discover available models.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// Models defines upstream model names and aliases for request routing.
	Models []SwitchAIModel `yaml:"models" json:"models"`

	// Headers optionally adds extra HTTP headers for requests sent with this key.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded for this provider.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`
}

// SwitchAIModel describes a mapping between an alias and the actual upstream model name.
type SwitchAIModel struct {
	// Name is the upstream model identifier used when issuing requests.
	Name string `yaml:"name" json:"name"`

	// Alias is the client-facing model name that maps to Name.
	Alias string `yaml:"alias" json:"alias"`
}

// CodexKey represents the configuration for a Codex API key,
// including the API key itself and an optional base URL for the API endpoint.
type CodexKey struct {
	// APIKey is the authentication key for accessing Codex API services.
	APIKey string `yaml:"api-key" json:"api-key"`

	// Prefix optionally namespaces models for this credential (e.g., "teamA/gpt-5-codex").
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL is the base URL for the Codex API endpoint.
	// If empty, the default Codex API URL will be used.
	BaseURL string `yaml:"base-url" json:"base-url"`

	// ProxyURL overrides the global proxy setting for this API key if provided.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ModelsURL optionally overrides the endpoint used to discover available models.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// Headers optionally adds extra HTTP headers for requests sent with this key.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded for this provider.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`
}

// GeminiKey represents the configuration for a Gemini API key,
// including optional overrides for upstream base URL, proxy routing, and headers.
type GeminiKey struct {
	// APIKey is the authentication key for accessing Gemini API services.
	APIKey string `yaml:"api-key" json:"api-key"`

	// Prefix optionally namespaces models for this credential (e.g., "teamA/gemini-3-pro-preview").
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL optionally overrides the Gemini API endpoint.
	BaseURL string `yaml:"base-url,omitempty" json:"base-url,omitempty"`

	// ProxyURL optionally overrides the global proxy for this API key.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`

	// ModelsURL optionally overrides the endpoint used to discover available models.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// Headers optionally adds extra HTTP headers for requests sent with this key.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded for this provider.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`
}

// OpenAICompatibility represents the configuration for OpenAI API compatibility
// with external providers, allowing model aliases to be routed through OpenAI API format.
type OpenAICompatibility struct {
	// Name is the identifier for this OpenAI compatibility configuration.
	Name string `yaml:"name" json:"name"`

	// Prefix optionally namespaces model aliases for this provider (e.g., "teamA/kimi-k2").
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL is the base URL for the external OpenAI-compatible API endpoint.
	BaseURL string `yaml:"base-url" json:"base-url"`

	// ModelsURL optionally overrides the endpoint used to discover available models.
	ModelsURL string `yaml:"models-url,omitempty" json:"models-url,omitempty"`

	// APIKeyEntries defines API keys with optional per-key proxy configuration.
	APIKeyEntries []OpenAICompatibilityAPIKey `yaml:"api-key-entries,omitempty" json:"api-key-entries,omitempty"`

	// Models defines the model configurations including aliases for routing.
	Models []OpenAICompatibilityModel `yaml:"models" json:"models"`

	// Headers optionally adds extra HTTP headers for requests sent to this provider.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels lists model IDs that should be excluded for this provider.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`

	// ProxyURL overrides the global proxy setting for this provider.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`
}

// OpenAICompatibilityAPIKey represents an API key configuration with optional proxy setting.
type OpenAICompatibilityAPIKey struct {
	// APIKey is the authentication key for accessing the external API services.
	APIKey string `yaml:"api-key" json:"api-key"`

	// ProxyURL overrides the global proxy setting for this API key if provided.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`
}

// OpenAICompatibilityModel represents a model configuration for OpenAI compatibility,
// including the actual model name and its alias for API routing.
type OpenAICompatibilityModel struct {
	// Name is the actual model name used by the external provider.
	Name string `yaml:"name" json:"name"`

	// Alias is the model name alias that clients will use to reference this model.
	Alias string `yaml:"alias" json:"alias"`
}

// LoadConfig reads a YAML configuration file from the given path,
// unmarshals it into a Config struct, applies environment variable overrides,
// and returns it.
//
// Parameters:
//   - configFile: The path to the YAML configuration file
//
// Returns:
//   - *Config: The loaded configuration
//   - error: An error if the configuration could not be loaded
func LoadConfig(configFile string) (*Config, error) {
	return LoadConfigOptional(configFile, false)
}

// LoadConfigOptional reads YAML from configFile.
// If optional is true and the file is missing, it returns an empty Config.
// If optional is true and the file is empty or invalid, it returns an empty Config.
func LoadConfigOptional(configFile string, optional bool) (*Config, error) {
	// Read the entire configuration file into memory.
	data, err := os.ReadFile(configFile)
	if err != nil {
		if optional {
			if os.IsNotExist(err) || errors.Is(err, syscall.EISDIR) {
				// Missing and optional: return empty config (cloud deploy standby).
				return &Config{}, nil
			}
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// In cloud deploy mode (optional=true), if file is empty or contains only whitespace, return empty config.
	if optional && len(data) == 0 {
		return &Config{}, nil
	}

	// Unmarshal the YAML data into the Config struct.
	var cfg Config
	// Set defaults before unmarshal so that absent keys keep defaults.
	cfg.Host = "" // Default empty: binds to all interfaces (IPv4 + IPv6)
	cfg.LoggingToFile = false
	cfg.LogsMaxTotalSizeMB = 0
	cfg.UsageStatisticsEnabled = false
	cfg.DisableCooling = false
	cfg.AmpCode.RestrictManagementToLocalhost = false // Default to false: API key auth is sufficient
	cfg.RemoteManagement.PanelGitHubRepository = DefaultPanelGitHubRepository
	cfg.WebsocketAuth = true // Default to true: Secure by default
	
	// Set Superbrain defaults
	cfg.Superbrain.Enabled = false
	cfg.Superbrain.Mode = "disabled"
	// Set component flags defaults (all enabled by default when Superbrain is enabled)
	cfg.Superbrain.ComponentFlags.OverwatchEnabled = true
	cfg.Superbrain.ComponentFlags.DoctorEnabled = true
	cfg.Superbrain.ComponentFlags.InjectorEnabled = true
	cfg.Superbrain.ComponentFlags.RecoveryEnabled = true
	cfg.Superbrain.ComponentFlags.FallbackEnabled = true
	cfg.Superbrain.ComponentFlags.SculptorEnabled = true
	cfg.Superbrain.Overwatch.SilenceThresholdMs = 30000 // 30 seconds
	cfg.Superbrain.Overwatch.LogBufferSize = 50
	cfg.Superbrain.Overwatch.HeartbeatIntervalMs = 1000 // 1 second
	cfg.Superbrain.Overwatch.MaxRestartAttempts = 2
	cfg.Superbrain.Doctor.Model = "gemini-flash"
	cfg.Superbrain.Doctor.TimeoutMs = 5000 // 5 seconds
	cfg.Superbrain.StdinInjection.Mode = "conservative"
	cfg.Superbrain.ContextSculptor.Enabled = true
	cfg.Superbrain.ContextSculptor.TokenEstimator = "tiktoken"
	cfg.Superbrain.ContextSculptor.PriorityFiles = []string{"README.md", "main.go", "index.ts", "package.json"}
	cfg.Superbrain.Fallback.Enabled = true
	cfg.Superbrain.Fallback.Providers = []string{"geminicli", "gemini", "ollama"}
	cfg.Superbrain.Fallback.MinSuccessRate = 0.5
	cfg.Superbrain.Consensus.Enabled = false
	cfg.Superbrain.Consensus.VerificationModel = "gemini-flash"
	cfg.Superbrain.Consensus.TriggerPatterns = []string{"abrupt_ending", "missing_code_blocks"}
	cfg.Superbrain.Security.AuditLogEnabled = true
	cfg.Superbrain.Security.AuditLogPath = "./logs/superbrain_audit.log"
	cfg.Superbrain.Security.ForbiddenOperations = []string{"file_delete", "system_command"}
	
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		if optional {
			// In cloud deploy mode, if YAML parsing fails, return empty config instead of error.
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	var legacy legacyConfigData
	if errLegacy := yaml.Unmarshal(data, &legacy); errLegacy == nil {
		if cfg.migrateLegacyGeminiKeys(legacy.LegacyGeminiKeys) {
			cfg.legacyMigrationPending = true
		}
		if cfg.migrateLegacyOpenAICompatibilityKeys(legacy.OpenAICompat) {
			cfg.legacyMigrationPending = true
		}
		if cfg.migrateLegacyAmpConfig(&legacy) {
			cfg.legacyMigrationPending = true
		}
	}

	// Hash remote management key if plaintext is detected (nested)
	// We consider a value to be already hashed if it looks like a bcrypt hash ($2a$, $2b$, or $2y$ prefix).
	if cfg.RemoteManagement.SecretKey != "" && !looksLikeBcrypt(cfg.RemoteManagement.SecretKey) {
		hashed, errHash := hashSecret(cfg.RemoteManagement.SecretKey)
		if errHash != nil {
			return nil, fmt.Errorf("failed to hash remote management key: %w", errHash)
		}
		cfg.RemoteManagement.SecretKey = hashed

		// Persist the hashed value back to the config file to avoid re-hashing on next startup.
		// Preserve YAML comments and ordering; update only the nested key.
		_ = SaveConfigPreserveCommentsUpdateNestedScalar(configFile, []string{"remote-management", "secret-key"}, hashed)
	}

	cfg.RemoteManagement.PanelGitHubRepository = strings.TrimSpace(cfg.RemoteManagement.PanelGitHubRepository)
	if cfg.RemoteManagement.PanelGitHubRepository == "" {
		cfg.RemoteManagement.PanelGitHubRepository = DefaultPanelGitHubRepository
	}

	if cfg.LogsMaxTotalSizeMB < 0 {
		cfg.LogsMaxTotalSizeMB = 0
	}

	// Sync request authentication providers with inline API keys for backwards compatibility.
	syncInlineAccessProvider(&cfg)

	// Sanitize Gemini API key configuration and migrate legacy entries.
	cfg.SanitizeGeminiKeys()

	// Sanitize Vertex-compatible API keys: drop entries without base-url
	cfg.SanitizeVertexCompatKeys()

	// Sanitize Codex keys: drop entries without base-url
	cfg.SanitizeCodexKeys()

	// Sanitize Claude key headers
	cfg.SanitizeClaudeKeys()

	// Sanitize SwitchAI keys: default base-url and normalize headers
	cfg.SanitizeSwitchAIKeys()

	// Sanitize OpenAI compatibility providers: drop entries without base-url
	cfg.SanitizeOpenAICompatibility()

	// Sanitize Superbrain configuration
	cfg.SanitizeSuperbrain()

	// Normalize OAuth provider model exclusion map.
	cfg.OAuthExcludedModels = NormalizeOAuthExcludedModels(cfg.OAuthExcludedModels)

	if cfg.legacyMigrationPending {
		fmt.Println("Detected legacy configuration keys, attempting to persist the normalized config...")
		if !optional && configFile != "" {
			if err := SaveConfigPreserveComments(configFile, &cfg); err != nil {
				return nil, fmt.Errorf("failed to persist migrated legacy config: %w", err)
			}
			fmt.Println("Legacy configuration normalized and persisted.")
		} else {
			fmt.Println("Legacy configuration normalized in memory; persistence skipped.")
		}
	}

	// Return the populated configuration struct.
	return &cfg, nil
}

// SanitizeOpenAICompatibility removes OpenAI-compatibility provider entries that are
// not actionable, specifically those missing a BaseURL. It trims whitespace before
// evaluation and preserves the relative order of remaining entries.
func (cfg *Config) SanitizeOpenAICompatibility() {
	if cfg == nil || len(cfg.OpenAICompatibility) == 0 {
		return
	}
	out := make([]OpenAICompatibility, 0, len(cfg.OpenAICompatibility))
	for i := range cfg.OpenAICompatibility {
		e := cfg.OpenAICompatibility[i]
		e.Name = strings.TrimSpace(e.Name)
		e.Prefix = normalizeModelPrefix(e.Prefix)
		e.BaseURL = strings.TrimSpace(e.BaseURL)
		e.Headers = NormalizeHeaders(e.Headers)
		if e.BaseURL == "" {
			// Skip providers with no base-url; treated as removed
			continue
		}
		out = append(out, e)
	}
	cfg.OpenAICompatibility = out
}

// SanitizeSwitchAIKeys normalizes SwitchAI API key entries.
// It applies the default switchAI BaseURL if none is provided.
func (cfg *Config) SanitizeSwitchAIKeys() {
	if cfg == nil || len(cfg.SwitchAIKey) == 0 {
		return
	}
	for i := range cfg.SwitchAIKey {
		e := &cfg.SwitchAIKey[i]
		e.APIKey = strings.TrimSpace(e.APIKey)
		e.Prefix = normalizeModelPrefix(e.Prefix)
		e.BaseURL = strings.TrimSpace(e.BaseURL)
		if e.BaseURL == "" {
			e.BaseURL = "https://switchai.traylinx.com/v1"
		}
		e.ProxyURL = strings.TrimSpace(e.ProxyURL)
		e.Headers = NormalizeHeaders(e.Headers)
		e.ExcludedModels = NormalizeExcludedModels(e.ExcludedModels)
		if len(e.Models) > 0 {
			normalized := make([]SwitchAIModel, 0, len(e.Models))
			for j := range e.Models {
				m := e.Models[j]
				m.Name = strings.TrimSpace(m.Name)
				m.Alias = strings.TrimSpace(m.Alias)
				if m.Name == "" && m.Alias == "" {
					continue
				}
				normalized = append(normalized, m)
			}
			e.Models = normalized
		}
	}
}

// SanitizeCodexKeys removes Codex API key entries missing a BaseURL.
// It trims whitespace and preserves order for remaining entries.
func (cfg *Config) SanitizeCodexKeys() {
	if cfg == nil || len(cfg.CodexKey) == 0 {
		return
	}
	out := make([]CodexKey, 0, len(cfg.CodexKey))
	for i := range cfg.CodexKey {
		e := cfg.CodexKey[i]
		e.Prefix = normalizeModelPrefix(e.Prefix)
		e.BaseURL = strings.TrimSpace(e.BaseURL)
		e.Headers = NormalizeHeaders(e.Headers)
		e.ExcludedModels = NormalizeExcludedModels(e.ExcludedModels)
		if e.BaseURL == "" {
			continue
		}
		out = append(out, e)
	}
	cfg.CodexKey = out
}

// SanitizeClaudeKeys normalizes headers for Claude credentials.
func (cfg *Config) SanitizeClaudeKeys() {
	if cfg == nil || len(cfg.ClaudeKey) == 0 {
		return
	}
	for i := range cfg.ClaudeKey {
		entry := &cfg.ClaudeKey[i]
		entry.Prefix = normalizeModelPrefix(entry.Prefix)
		entry.Headers = NormalizeHeaders(entry.Headers)
		entry.ExcludedModels = NormalizeExcludedModels(entry.ExcludedModels)
	}
}

// SanitizeGeminiKeys deduplicates and normalizes Gemini credentials.
func (cfg *Config) SanitizeGeminiKeys() {
	if cfg == nil {
		return
	}

	seen := make(map[string]struct{}, len(cfg.GeminiKey))
	out := cfg.GeminiKey[:0]
	for i := range cfg.GeminiKey {
		entry := cfg.GeminiKey[i]
		entry.APIKey = strings.TrimSpace(entry.APIKey)
		if entry.APIKey == "" {
			continue
		}
		entry.Prefix = normalizeModelPrefix(entry.Prefix)
		entry.BaseURL = strings.TrimSpace(entry.BaseURL)
		entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
		entry.Headers = NormalizeHeaders(entry.Headers)
		entry.ExcludedModels = NormalizeExcludedModels(entry.ExcludedModels)
		if _, exists := seen[entry.APIKey]; exists {
			continue
		}
		seen[entry.APIKey] = struct{}{}
		out = append(out, entry)
	}
	cfg.GeminiKey = out
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

func normalizeModelPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "/") {
		return ""
	}
	return trimmed
}

func syncInlineAccessProvider(cfg *Config) {
	if cfg == nil {
		return
	}
	if len(cfg.APIKeys) == 0 {
		if provider := cfg.ConfigAPIKeyProvider(); provider != nil && len(provider.APIKeys) > 0 {
			cfg.APIKeys = append([]string(nil), provider.APIKeys...)
		}
	}
	cfg.Access.Providers = nil
}

// looksLikeBcrypt returns true if the provided string appears to be a bcrypt hash.
func looksLikeBcrypt(s string) bool {
	return len(s) > 4 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

// NormalizeHeaders trims header keys and values and removes empty pairs.
func NormalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	clean := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		clean[key] = val
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// NormalizeExcludedModels trims, lowercases, and deduplicates model exclusion patterns.
// It preserves the order of first occurrences and drops empty entries.
func NormalizeExcludedModels(models []string) []string {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, raw := range models {
		trimmed := strings.ToLower(strings.TrimSpace(raw))
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeOAuthExcludedModels cleans provider -> excluded models mappings by normalizing provider keys
// and applying model exclusion normalization to each entry.
func NormalizeOAuthExcludedModels(entries map[string][]string) map[string][]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string][]string, len(entries))
	for provider, models := range entries {
		key := strings.ToLower(strings.TrimSpace(provider))
		if key == "" {
			continue
		}
		normalized := NormalizeExcludedModels(models)
		if len(normalized) == 0 {
			continue
		}
		out[key] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hashSecret hashes the given secret using bcrypt.
func hashSecret(secret string) (string, error) {
	// Use default cost for simplicity.
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// SaveConfigPreserveComments writes the config back to YAML while preserving existing comments
// and key ordering by loading the original file into a yaml.Node tree and updating values in-place.
func SaveConfigPreserveComments(configFile string, cfg *Config) error {
	persistCfg := sanitizeConfigForPersist(cfg)
	// Load original YAML as a node tree to preserve comments and ordering.
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	var original yaml.Node
	if err = yaml.Unmarshal(data, &original); err != nil {
		return err
	}
	if original.Kind != yaml.DocumentNode || len(original.Content) == 0 {
		return fmt.Errorf("invalid yaml document structure")
	}
	if original.Content[0] == nil || original.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("expected root mapping node")
	}

	// Marshal the current cfg to YAML, then unmarshal to a yaml.Node we can merge from.
	rendered, err := yaml.Marshal(persistCfg)
	if err != nil {
		return err
	}
	var generated yaml.Node
	if err = yaml.Unmarshal(rendered, &generated); err != nil {
		return err
	}
	if generated.Kind != yaml.DocumentNode || len(generated.Content) == 0 || generated.Content[0] == nil {
		return fmt.Errorf("invalid generated yaml structure")
	}
	if generated.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("expected generated root mapping node")
	}

	// Remove deprecated sections before merging back the sanitized config.
	removeLegacyAuthBlock(original.Content[0])
	removeLegacyOpenAICompatAPIKeys(original.Content[0])
	removeLegacyAmpKeys(original.Content[0])
	removeLegacyGenerativeLanguageKeys(original.Content[0])

	pruneMappingToGeneratedKeys(original.Content[0], generated.Content[0], "oauth-excluded-models")

	// Merge generated into original in-place, preserving comments/order of existing nodes.
	mergeMappingPreserve(original.Content[0], generated.Content[0])
	normalizeCollectionNodeStyles(original.Content[0])

	// Write back.
	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&original); err != nil {
		_ = enc.Close()
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	data = NormalizeCommentIndentation(buf.Bytes())
	_, err = f.Write(data)
	return err
}

func sanitizeConfigForPersist(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	clone.SDKConfig = cfg.SDKConfig
	clone.SDKConfig.Access = AccessConfig{}
	return &clone
}

// SaveConfigPreserveCommentsUpdateNestedScalar updates a nested scalar key path like ["a","b"]
// while preserving comments and positions.
func SaveConfigPreserveCommentsUpdateNestedScalar(configFile string, path []string, value string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err = yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("invalid yaml document structure")
	}
	node := root.Content[0]
	// descend mapping nodes following path
	for i, key := range path {
		if i == len(path)-1 {
			// set final scalar
			v := getOrCreateMapValue(node, key)
			v.Kind = yaml.ScalarNode
			v.Tag = "!!str"
			v.Value = value
		} else {
			next := getOrCreateMapValue(node, key)
			if next.Kind != yaml.MappingNode {
				next.Kind = yaml.MappingNode
				next.Tag = "!!map"
			}
			node = next
		}
	}
	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&root); err != nil {
		_ = enc.Close()
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	data = NormalizeCommentIndentation(buf.Bytes())
	_, err = f.Write(data)
	return err
}

// NormalizeCommentIndentation removes indentation from standalone YAML comment lines to keep them left aligned.
func NormalizeCommentIndentation(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	changed := false
	for i, line := range lines {
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 || trimmed[0] != '#' {
			continue
		}
		if len(trimmed) == len(line) {
			continue
		}
		lines[i] = append([]byte(nil), trimmed...)
		changed = true
	}
	if !changed {
		return data
	}
	return bytes.Join(lines, []byte("\n"))
}

// getOrCreateMapValue finds the value node for a given key in a mapping node.
// If not found, it appends a new key/value pair and returns the new value node.
func getOrCreateMapValue(mapNode *yaml.Node, key string) *yaml.Node {
	if mapNode.Kind != yaml.MappingNode {
		mapNode.Kind = yaml.MappingNode
		mapNode.Tag = "!!map"
		mapNode.Content = nil
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		k := mapNode.Content[i]
		if k.Value == key {
			return mapNode.Content[i+1]
		}
	}
	// append new key/value
	mapNode.Content = append(mapNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key})
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ""}
	mapNode.Content = append(mapNode.Content, val)
	return val
}

// mergeMappingPreserve merges keys from src into dst mapping node while preserving
// key order and comments of existing keys in dst. Unknown keys from src are appended
// to dst at the end, copying their node structure from src.
func mergeMappingPreserve(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		// If kinds do not match, prefer replacing dst with src semantics in-place
		// but keep dst node object to preserve any attached comments at the parent level.
		copyNodeShallow(dst, src)
		return
	}
	// Build a lookup of existing keys in dst
	for i := 0; i+1 < len(src.Content); i += 2 {
		sk := src.Content[i]
		sv := src.Content[i+1]
		idx := findMapKeyIndex(dst, sk.Value)
		if idx >= 0 {
			// Merge into existing value node
			dv := dst.Content[idx+1]
			mergeNodePreserve(dv, sv)
		} else {
			if shouldSkipEmptyCollectionOnPersist(sk.Value, sv) {
				continue
			}
			// Append new key/value pair by deep-copying from src
			dst.Content = append(dst.Content, deepCopyNode(sk), deepCopyNode(sv))
		}
	}
}

// mergeNodePreserve merges src into dst for scalars, mappings and sequences while
// reusing destination nodes to keep comments and anchors. For sequences, it updates
// in-place by index.
func mergeNodePreserve(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	switch src.Kind {
	case yaml.MappingNode:
		if dst.Kind != yaml.MappingNode {
			copyNodeShallow(dst, src)
		}
		mergeMappingPreserve(dst, src)
	case yaml.SequenceNode:
		// Preserve explicit null style if dst was null and src is empty sequence
		if dst.Kind == yaml.ScalarNode && dst.Tag == "!!null" && len(src.Content) == 0 {
			// Keep as null to preserve original style
			return
		}
		if dst.Kind != yaml.SequenceNode {
			dst.Kind = yaml.SequenceNode
			dst.Tag = "!!seq"
			dst.Content = nil
		}
		reorderSequenceForMerge(dst, src)
		// Update elements in place
		minContent := len(dst.Content)
		if len(src.Content) < minContent {
			minContent = len(src.Content)
		}
		for i := 0; i < minContent; i++ {
			if dst.Content[i] == nil {
				dst.Content[i] = deepCopyNode(src.Content[i])
				continue
			}
			mergeNodePreserve(dst.Content[i], src.Content[i])
			if dst.Content[i] != nil && src.Content[i] != nil &&
				dst.Content[i].Kind == yaml.MappingNode && src.Content[i].Kind == yaml.MappingNode {
				pruneMissingMapKeys(dst.Content[i], src.Content[i])
			}
		}
		// Append any extra items from src
		for i := len(dst.Content); i < len(src.Content); i++ {
			dst.Content = append(dst.Content, deepCopyNode(src.Content[i]))
		}
		// Truncate if dst has extra items not in src
		if len(src.Content) < len(dst.Content) {
			dst.Content = dst.Content[:len(src.Content)]
		}
	case yaml.ScalarNode, yaml.AliasNode:
		// For scalars, update Tag and Value but keep Style from dst to preserve quoting
		dst.Kind = src.Kind
		dst.Tag = src.Tag
		dst.Value = src.Value
		// Keep dst.Style as-is intentionally
	case 0:
		// Unknown/empty kind; do nothing
	default:
		// Fallback: replace shallowly
		copyNodeShallow(dst, src)
	}
}

// findMapKeyIndex returns the index of key node in dst mapping (index of key, not value).
// Returns -1 when not found.
func findMapKeyIndex(mapNode *yaml.Node, key string) int {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i] != nil && mapNode.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func shouldSkipEmptyCollectionOnPersist(key string, node *yaml.Node) bool {
	switch key {
	case "generative-language-api-key",
		"gemini-api-key",
		"vertex-api-key",
		"claude-api-key",
		"switchai-api-key",
		"codex-api-key",
		"openai-compatibility":
		return isEmptyCollectionNode(node)
	default:
		return false
	}
}

func isEmptyCollectionNode(node *yaml.Node) bool {
	if node == nil {
		return true
	}
	switch node.Kind {
	case yaml.SequenceNode:
		return len(node.Content) == 0
	case yaml.ScalarNode:
		return node.Tag == "!!null"
	default:
		return false
	}
}

// deepCopyNode creates a deep copy of a yaml.Node graph.
func deepCopyNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	cp := *n
	if len(n.Content) > 0 {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i := range n.Content {
			cp.Content[i] = deepCopyNode(n.Content[i])
		}
	}
	return &cp
}

// copyNodeShallow copies type/tag/value and resets content to match src, but
// keeps the same destination node pointer to preserve parent relations/comments.
func copyNodeShallow(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	dst.Kind = src.Kind
	dst.Tag = src.Tag
	dst.Value = src.Value
	// Replace content with deep copy from src
	if len(src.Content) > 0 {
		dst.Content = make([]*yaml.Node, len(src.Content))
		for i := range src.Content {
			dst.Content[i] = deepCopyNode(src.Content[i])
		}
	} else {
		dst.Content = nil
	}
}

func reorderSequenceForMerge(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	if len(dst.Content) == 0 {
		return
	}
	if len(src.Content) == 0 {
		return
	}
	original := append([]*yaml.Node(nil), dst.Content...)
	used := make([]bool, len(original))
	ordered := make([]*yaml.Node, len(src.Content))
	for i := range src.Content {
		if idx := matchSequenceElement(original, used, src.Content[i]); idx >= 0 {
			ordered[i] = original[idx]
			used[idx] = true
		}
	}
	dst.Content = ordered
}

func matchSequenceElement(original []*yaml.Node, used []bool, target *yaml.Node) int {
	if target == nil {
		return -1
	}
	switch target.Kind {
	case yaml.MappingNode:
		id := sequenceElementIdentity(target)
		if id != "" {
			for i := range original {
				if used[i] || original[i] == nil || original[i].Kind != yaml.MappingNode {
					continue
				}
				if sequenceElementIdentity(original[i]) == id {
					return i
				}
			}
		}
	case yaml.ScalarNode:
		val := strings.TrimSpace(target.Value)
		if val != "" {
			for i := range original {
				if used[i] || original[i] == nil || original[i].Kind != yaml.ScalarNode {
					continue
				}
				if strings.TrimSpace(original[i].Value) == val {
					return i
				}
			}
		}
	default:
	}
	// Fallback to structural equality to preserve nodes lacking explicit identifiers.
	for i := range original {
		if used[i] || original[i] == nil {
			continue
		}
		if nodesStructurallyEqual(original[i], target) {
			return i
		}
	}
	return -1
}

func sequenceElementIdentity(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	identityKeys := []string{"id", "name", "alias", "api-key", "api_key", "apikey", "key", "provider", "model"}
	for _, k := range identityKeys {
		if v := mappingScalarValue(node, k); v != "" {
			return k + "=" + v
		}
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		if keyNode == nil || valNode == nil || valNode.Kind != yaml.ScalarNode {
			continue
		}
		val := strings.TrimSpace(valNode.Value)
		if val != "" {
			return strings.ToLower(strings.TrimSpace(keyNode.Value)) + "=" + val
		}
	}
	return ""
}

func mappingScalarValue(node *yaml.Node, key string) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	lowerKey := strings.ToLower(key)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		if keyNode == nil || valNode == nil || valNode.Kind != yaml.ScalarNode {
			continue
		}
		if strings.ToLower(strings.TrimSpace(keyNode.Value)) == lowerKey {
			return strings.TrimSpace(valNode.Value)
		}
	}
	return ""
}

func nodesStructurallyEqual(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := 0; i+1 < len(a.Content); i += 2 {
			if !nodesStructurallyEqual(a.Content[i], b.Content[i]) {
				return false
			}
			if !nodesStructurallyEqual(a.Content[i+1], b.Content[i+1]) {
				return false
			}
		}
		return true
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodesStructurallyEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	case yaml.ScalarNode:
		return strings.TrimSpace(a.Value) == strings.TrimSpace(b.Value)
	case yaml.AliasNode:
		return nodesStructurallyEqual(a.Alias, b.Alias)
	default:
		return strings.TrimSpace(a.Value) == strings.TrimSpace(b.Value)
	}
}

func removeMapKey(mapNode *yaml.Node, key string) {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode || key == "" {
		return
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i] != nil && mapNode.Content[i].Value == key {
			mapNode.Content = append(mapNode.Content[:i], mapNode.Content[i+2:]...)
			return
		}
	}
}

func pruneMappingToGeneratedKeys(dstRoot, srcRoot *yaml.Node, key string) {
	if key == "" || dstRoot == nil || srcRoot == nil {
		return
	}
	if dstRoot.Kind != yaml.MappingNode || srcRoot.Kind != yaml.MappingNode {
		return
	}
	dstIdx := findMapKeyIndex(dstRoot, key)
	if dstIdx < 0 || dstIdx+1 >= len(dstRoot.Content) {
		return
	}
	srcIdx := findMapKeyIndex(srcRoot, key)
	if srcIdx < 0 {
		removeMapKey(dstRoot, key)
		return
	}
	if srcIdx+1 >= len(srcRoot.Content) {
		return
	}
	srcVal := srcRoot.Content[srcIdx+1]
	dstVal := dstRoot.Content[dstIdx+1]
	if srcVal == nil {
		dstRoot.Content[dstIdx+1] = nil
		return
	}
	if srcVal.Kind != yaml.MappingNode {
		dstRoot.Content[dstIdx+1] = deepCopyNode(srcVal)
		return
	}
	if dstVal == nil || dstVal.Kind != yaml.MappingNode {
		dstRoot.Content[dstIdx+1] = deepCopyNode(srcVal)
		return
	}
	pruneMissingMapKeys(dstVal, srcVal)
}

func pruneMissingMapKeys(dstMap, srcMap *yaml.Node) {
	if dstMap == nil || srcMap == nil || dstMap.Kind != yaml.MappingNode || srcMap.Kind != yaml.MappingNode {
		return
	}
	keep := make(map[string]struct{}, len(srcMap.Content)/2)
	for i := 0; i+1 < len(srcMap.Content); i += 2 {
		keyNode := srcMap.Content[i]
		if keyNode == nil {
			continue
		}
		key := strings.TrimSpace(keyNode.Value)
		if key == "" {
			continue
		}
		keep[key] = struct{}{}
	}
	for i := 0; i+1 < len(dstMap.Content); {
		keyNode := dstMap.Content[i]
		if keyNode == nil {
			i += 2
			continue
		}
		key := strings.TrimSpace(keyNode.Value)
		if _, ok := keep[key]; !ok {
			dstMap.Content = append(dstMap.Content[:i], dstMap.Content[i+2:]...)
			continue
		}
		i += 2
	}
}

// normalizeCollectionNodeStyles forces YAML collections to use block notation, keeping
// lists and maps readable. Empty sequences retain flow style ([]) so empty list markers
// remain compact.
func normalizeCollectionNodeStyles(node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		node.Style = 0
		for i := range node.Content {
			normalizeCollectionNodeStyles(node.Content[i])
		}
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			node.Style = yaml.FlowStyle
		} else {
			node.Style = 0
		}
		for i := range node.Content {
			normalizeCollectionNodeStyles(node.Content[i])
		}
	default:
		// Scalars keep their existing style to preserve quoting
	}
}

// Legacy migration helpers (move deprecated config keys into structured fields).
type legacyConfigData struct {
	LegacyGeminiKeys      []string                    `yaml:"generative-language-api-key"`
	OpenAICompat          []legacyOpenAICompatibility `yaml:"openai-compatibility"`
	AmpUpstreamURL        string                      `yaml:"amp-upstream-url"`
	AmpUpstreamAPIKey     string                      `yaml:"amp-upstream-api-key"`
	AmpRestrictManagement *bool                       `yaml:"amp-restrict-management-to-localhost"`
	AmpModelMappings      []AmpModelMapping           `yaml:"amp-model-mappings"`
}

type legacyOpenAICompatibility struct {
	Name    string   `yaml:"name"`
	BaseURL string   `yaml:"base-url"`
	APIKeys []string `yaml:"api-keys"`
}

func (cfg *Config) migrateLegacyGeminiKeys(legacy []string) bool {
	if cfg == nil || len(legacy) == 0 {
		return false
	}
	changed := false
	seen := make(map[string]struct{}, len(cfg.GeminiKey))
	for i := range cfg.GeminiKey {
		key := strings.TrimSpace(cfg.GeminiKey[i].APIKey)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	for _, raw := range legacy {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		cfg.GeminiKey = append(cfg.GeminiKey, GeminiKey{APIKey: key})
		seen[key] = struct{}{}
		changed = true
	}
	return changed
}

func (cfg *Config) migrateLegacyOpenAICompatibilityKeys(legacy []legacyOpenAICompatibility) bool {
	if cfg == nil || len(cfg.OpenAICompatibility) == 0 || len(legacy) == 0 {
		return false
	}
	changed := false
	for _, legacyEntry := range legacy {
		if len(legacyEntry.APIKeys) == 0 {
			continue
		}
		target := findOpenAICompatTarget(cfg.OpenAICompatibility, legacyEntry.Name, legacyEntry.BaseURL)
		if target == nil {
			continue
		}
		if mergeLegacyOpenAICompatAPIKeys(target, legacyEntry.APIKeys) {
			changed = true
		}
	}
	return changed
}

func mergeLegacyOpenAICompatAPIKeys(entry *OpenAICompatibility, keys []string) bool {
	if entry == nil || len(keys) == 0 {
		return false
	}
	changed := false
	existing := make(map[string]struct{}, len(entry.APIKeyEntries))
	for i := range entry.APIKeyEntries {
		key := strings.TrimSpace(entry.APIKeyEntries[i].APIKey)
		if key == "" {
			continue
		}
		existing[key] = struct{}{}
	}
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		entry.APIKeyEntries = append(entry.APIKeyEntries, OpenAICompatibilityAPIKey{APIKey: key})
		existing[key] = struct{}{}
		changed = true
	}
	return changed
}

func findOpenAICompatTarget(entries []OpenAICompatibility, legacyName, legacyBase string) *OpenAICompatibility {
	nameKey := strings.ToLower(strings.TrimSpace(legacyName))
	baseKey := strings.ToLower(strings.TrimSpace(legacyBase))
	if nameKey != "" && baseKey != "" {
		for i := range entries {
			if strings.ToLower(strings.TrimSpace(entries[i].Name)) == nameKey &&
				strings.ToLower(strings.TrimSpace(entries[i].BaseURL)) == baseKey {
				return &entries[i]
			}
		}
	}
	if baseKey != "" {
		for i := range entries {
			if strings.ToLower(strings.TrimSpace(entries[i].BaseURL)) == baseKey {
				return &entries[i]
			}
		}
	}
	if nameKey != "" {
		for i := range entries {
			if strings.ToLower(strings.TrimSpace(entries[i].Name)) == nameKey {
				return &entries[i]
			}
		}
	}
	return nil
}

func (cfg *Config) migrateLegacyAmpConfig(legacy *legacyConfigData) bool {
	if cfg == nil || legacy == nil {
		return false
	}
	changed := false
	if cfg.AmpCode.UpstreamURL == "" {
		if val := strings.TrimSpace(legacy.AmpUpstreamURL); val != "" {
			cfg.AmpCode.UpstreamURL = val
			changed = true
		}
	}
	if cfg.AmpCode.UpstreamAPIKey == "" {
		if val := strings.TrimSpace(legacy.AmpUpstreamAPIKey); val != "" {
			cfg.AmpCode.UpstreamAPIKey = val
			changed = true
		}
	}
	if legacy.AmpRestrictManagement != nil {
		cfg.AmpCode.RestrictManagementToLocalhost = *legacy.AmpRestrictManagement
		changed = true
	}
	if len(cfg.AmpCode.ModelMappings) == 0 && len(legacy.AmpModelMappings) > 0 {
		cfg.AmpCode.ModelMappings = append([]AmpModelMapping(nil), legacy.AmpModelMappings...)
		changed = true
	}
	return changed
}

func removeLegacyOpenAICompatAPIKeys(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	idx := findMapKeyIndex(root, "openai-compatibility")
	if idx < 0 || idx+1 >= len(root.Content) {
		return
	}
	seq := root.Content[idx+1]
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := range seq.Content {
		if seq.Content[i] != nil && seq.Content[i].Kind == yaml.MappingNode {
			removeMapKey(seq.Content[i], "api-keys")
		}
	}
}

func removeLegacyAmpKeys(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	removeMapKey(root, "amp-upstream-url")
	removeMapKey(root, "amp-upstream-api-key")
	removeMapKey(root, "amp-restrict-management-to-localhost")
	removeMapKey(root, "amp-model-mappings")
}

func removeLegacyGenerativeLanguageKeys(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	removeMapKey(root, "generative-language-api-key")
}

func removeLegacyAuthBlock(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	removeMapKey(root, "auth")
}
