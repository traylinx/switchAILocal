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
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"github.com/traylinx/switchAILocal/internal/autoroute"
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
	// This is the core engine that executes user-defined logic.
	Plugin PluginConfig `yaml:"plugin" json:"plugin"`

	// Superbrain configures the intelligent orchestration and self-healing capabilities.
	Superbrain SuperbrainConfig `yaml:"superbrain" json:"superbrain"`

	// Heartbeat configures proactive background monitoring for provider health.
	Heartbeat HeartbeatConfig `yaml:"heartbeat" json:"heartbeat"`

	// Memory configures the memory system for routing decisions and learning.
	Memory MemoryConfig `yaml:"memory" json:"memory"`

	// Steering configures context-aware routing rules.
	Steering SteeringConfig `yaml:"steering" json:"steering"`

	// Hooks configures the event-driven automation system.
	Hooks HooksConfig `yaml:"hooks" json:"hooks"`

	// Billboard configures the shared working directory for CLI provider requests.
	Billboard BillboardConfig `yaml:"billboard" json:"billboard"`

	// Observability configures logging format, metrics export, and request event streaming.
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`

	// Performance configures production performance tuning: rate limiting, circuit breakers,
	// load shedding, and profiling.
	Performance PerformanceConfig `yaml:"performance" json:"performance"`

	legacyMigrationPending bool `yaml:"-" json:"-"`
}

// ObservabilityConfig controls logging, metrics, and tracing output for external monitoring tools.
type ObservabilityConfig struct {
	// LogFormat sets the global log output format: "text" (default, human-readable) or "json" (structured, for Loki/ELK/Datadog).
	LogFormat string `yaml:"log-format" json:"log-format"`

	// Metrics configures the Prometheus metrics endpoint.
	Metrics MetricsConfig `yaml:"metrics" json:"metrics"`

	// RequestEvents configures structured NDJSON event logging for every proxied request.
	RequestEvents RequestEventsConfig `yaml:"request-events" json:"request-events"`
}

// MetricsConfig controls the Prometheus /metrics endpoint.
type MetricsConfig struct {
	// Enabled toggles the Prometheus metrics server.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Path is the HTTP path to serve metrics on. Default: "/metrics".
	Path string `yaml:"path" json:"path"`
}

// RequestEventsConfig controls structured per-request event logging.
type RequestEventsConfig struct {
	// Enabled toggles structured request event logging.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Output destination: "stdout" or "file".
	Output string `yaml:"output" json:"output"`
	// FilePath is the path for NDJSON event logs when Output is "file".
	FilePath string `yaml:"file-path" json:"file-path"`
	// IncludeBody includes request/response bodies in events (warning: large).
	IncludeBody bool `yaml:"include-body" json:"include-body"`
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

// MemoryConfig represents configuration for the memory system.
// The memory system records routing decisions, learns user preferences, and tracks provider quirks.
type MemoryConfig struct {
	// Enabled toggles the memory system on or off.
	// When false, no routing decisions are recorded.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// BaseDir is the directory where memory data is stored.
	// Default: ".switchailocal/memory"
	BaseDir string `yaml:"base-dir" json:"base-dir"`

	// RetentionDays is the number of days to retain routing decision logs.
	// Older logs are automatically cleaned up. Default: 90 days.
	RetentionDays int `yaml:"retention-days" json:"retention-days"`

	// MaxLogSizeMB is the maximum size in MB for a single log file.
	// When exceeded, the log file is rotated. Default: 100 MB.
	MaxLogSizeMB int `yaml:"max-log-size-mb" json:"max-log-size-mb"`

	// Compression enables gzip compression for rotated log files.
	// Default: true
	Compression bool `yaml:"compression" json:"compression"`
}

// BillboardConfig configures the shared working directory for CLI provider requests.
// When enabled, a system prompt is injected into CLI requests instructing the model
// to use a standard billboard folder for file operations when no specific path is provided.
type BillboardConfig struct {
	// Enabled toggles billboard system prompt injection for CLI providers.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// BaseDir is the shared billboard directory path.
	// Default: "~/.switchailocal/billboard"
	BaseDir string `yaml:"base-dir" json:"base-dir"`
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

	// Models defines upstream model names and aliases for request routing.
	Models []OpenAICompatibilityModel `yaml:"models,omitempty" json:"models,omitempty"`

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

	// Models defines upstream model names and aliases for request routing.
	Models []OpenAICompatibilityModel `yaml:"models,omitempty" json:"models,omitempty"`

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

	// KimiHistoryShim toggles a stateless body mutator that injects a
	// non-empty placeholder into reasoning_content on assistant messages
	// that carry tool_calls but lack reasoning_content. Required for Kimi
	// K2.6 (api.kimi.com/coding/v1) with thinking-mode-on used as a
	// failover candidate in heterogeneous aliases — Kimi 400s the
	// multi-turn request "thinking is enabled but reasoning_content is
	// missing in assistant tool call message" otherwise. DeepSeek-V4-Pro
	// has the same failure shape; the shim is harmless on providers that
	// don't require it (MiniMax/OpenAI ignore the field).
	//
	// Pointer semantics: nil → default (enabled), *true → enabled,
	// *false → explicitly disabled. Default-on prevents future operators
	// from recreating the 2026-04-27 outage by enabling Kimi without
	// reading docs.
	KimiHistoryShim *bool `yaml:"kimi-history-shim,omitempty" json:"kimi-history-shim,omitempty"`
}

// IsKimiHistoryShimEnabled reports whether the reasoning_content
// placeholder shim should be applied to outbound requests for this
// provider. Default is enabled (nil pointer); only an explicit *false
// disables it.
func (o *OpenAICompatibility) IsKimiHistoryShimEnabled() bool {
	if o == nil {
		return false
	}
	if o.KimiHistoryShim == nil {
		return true
	}
	return *o.KimiHistoryShim
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

	// Expose controls whether this concrete alias appears in normal public
	// model catalog responses. nil/true keeps legacy public behavior;
	// false keeps the alias routable internally while hiding it from /v1/models.
	Expose *bool `yaml:"expose,omitempty" json:"expose,omitempty"`

	// Visibility is a human-readable catalog visibility hint. "private" hides
	// the concrete alias from normal public model lists.
	Visibility string `yaml:"visibility,omitempty" json:"visibility,omitempty"`

	// ContextLength optionally declares the upstream context window.
	ContextLength int `yaml:"context_length,omitempty" json:"context_length,omitempty"`

	// Capabilities optionally declares explicit model capabilities for
	// /v1/models discovery. Virtual-pool routing uses its own member
	// capabilities; this field is for direct/provider aliases.
	Capabilities VirtualModelCapabilitiesConfig `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`

	// NativeTools declares provider-native tools this model supports out
	// of the box (e.g. MiniMax M2.7's `{"type":"web_search"}`). Surfaced
	// unmodified through /v1/models so agentic callers can discover and
	// splice them into their own caller tools[] at chat-completion time.
	// Empty / absent = no native tools.
	NativeTools []NativeTool `yaml:"native_tools,omitempty" json:"native_tools,omitempty"`
}

// NativeTool is the YAML-side mirror of registry.NativeTool. Kept in
// this package to avoid a dependency cycle (registry imports from
// config is fine; config importing registry would loop).
type NativeTool struct {
	// Type is the tool type the upstream model recognises (matches the
	// "type" field of an OpenAI tools[] entry, e.g. "web_search").
	Type string `yaml:"type" json:"type"`
	// Description is a short human-readable sentence for the operator /
	// client UI. Not forwarded to the model.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Params documents per-tool knobs (e.g. force_search, max_keyword).
	// Shape is provider-specific — YAML values are passed through as-is.
	Params map[string]any `yaml:"params,omitempty" json:"params,omitempty"`
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

	// Set Intelligence defaults
	cfg.SDKConfig.Intelligence.Enabled = false
	cfg.SDKConfig.Intelligence.RouterModel = "ollama:qwen:0.5b"
	cfg.SDKConfig.Intelligence.RouterFallback = "openai:gpt-4o-mini"
	cfg.SDKConfig.Intelligence.Matrix = make(map[string]string)

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

	// Set Heartbeat defaults
	cfg.Heartbeat.Enabled = false // Disabled by default (opt-in)
	cfg.Heartbeat.Interval = "5m"
	cfg.Heartbeat.Timeout = "5s"
	cfg.Heartbeat.AutoDiscovery = true
	cfg.Heartbeat.QuotaWarningThreshold = 0.80  // 80%
	cfg.Heartbeat.QuotaCriticalThreshold = 0.95 // 95%
	cfg.Heartbeat.MaxConcurrentChecks = 10
	cfg.Heartbeat.RetryAttempts = 2
	cfg.Heartbeat.RetryDelay = "1s"

	// Set Memory defaults
	cfg.Memory.Enabled = false // Disabled by default (opt-in)
	cfg.Memory.BaseDir = ".switchailocal/memory"
	cfg.Memory.RetentionDays = 90
	cfg.Memory.MaxLogSizeMB = 100
	cfg.Memory.Compression = true

	// Set Steering defaults
	cfg.Steering.Enabled = false // Disabled by default (opt-in)
	cfg.Steering.RulesDir = ".switchailocal/steering"
	cfg.Steering.HotReload = true

	// Set Hooks defaults
	cfg.Hooks.Enabled = false // Disabled by default (opt-in)
	cfg.Hooks.HooksDir = ".switchailocal/hooks"
	cfg.Hooks.HotReload = true

	// Set Billboard defaults
	cfg.Billboard.Enabled = false // Disabled by default (opt-in)
	cfg.Billboard.BaseDir = "~/.switchailocal/billboard"

	// Set Observability defaults
	cfg.Observability.LogFormat = "text" // Default: human-readable text logs
	cfg.Observability.Metrics.Enabled = false
	cfg.Observability.Metrics.Path = "/metrics"
	cfg.Observability.RequestEvents.Enabled = false
	cfg.Observability.RequestEvents.Output = "stdout"
	cfg.Observability.RequestEvents.FilePath = "logs/events.ndjson"

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
	// The sentinel value "disabled" is preserved as-is to signal auth-disabled mode.
	if cfg.RemoteManagement.SecretKey != "" && cfg.RemoteManagement.SecretKey != "disabled" && !looksLikeBcrypt(cfg.RemoteManagement.SecretKey) {
		hashed, errHash := hashSecret(cfg.RemoteManagement.SecretKey)
		if errHash != nil {
			return nil, fmt.Errorf("failed to hash remote management key: %w", errHash)
		}
		cfg.RemoteManagement.SecretKey = hashed

		// Persist the hashed value back to the config file to avoid re-hashing on next startup.
		// Preserve YAML comments and ordering; update only the nested key.
		_ = SaveConfigPreserveCommentsUpdateNestedScalar(configFile, []string{"remote-management", "secret-key"}, hashed)
	}

	// Fail closed: refuse to start a remotely-reachable management API with no secret.
	// When remote-management.secret-key is empty and no MANAGEMENT_PASSWORD is set, the
	// management middleware bypasses authentication entirely; combined with allow-remote
	// this exposes config read/write and provider-key dump to any remote host. The
	// localhost-only default (allow-remote: false) is unaffected. The "disabled" sentinel
	// is not empty, so it stays permitted (it rejects remote callers via hash mismatch).
	if cfg.RemoteManagement.AllowRemote && cfg.RemoteManagement.SecretKey == "" &&
		strings.TrimSpace(os.Getenv("MANAGEMENT_PASSWORD")) == "" {
		return nil, fmt.Errorf("remote-management.allow-remote is true but no secret is set: " +
			"set remote-management.secret-key or the MANAGEMENT_PASSWORD environment variable, " +
			"or disable allow-remote")
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

	// Sanitize Heartbeat configuration
	cfg.SanitizeHeartbeat()

	// Sanitize Memory configuration
	cfg.SanitizeMemory()

	// Sanitize Steering configuration
	cfg.SanitizeSteering()

	// Sanitize Hooks configuration
	cfg.SanitizeHooks()

	// Sanitize Intelligence configuration
	cfg.SDKConfig.SanitizeIntelligence()

	// Sanitize Auto-Routing configuration
	cfg.SanitizeAutoRouting()

	// Sanitize Performance configuration
	cfg.Performance.SanitizePerformance()

	if err := cfg.ValidateVirtualModels(); err != nil {
		if optional {
			return &Config{}, nil
		}
		return nil, err
	}

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

// SanitizeAutoRouting ensures safe defaults for the Phase 2 Auto-Routing system
// and auto-migrates the legacy AutoModelPriority list into basic model preferences.
func (cfg *Config) SanitizeAutoRouting() {
	if cfg == nil {
		return
	}

	ar := &cfg.AutoRouting

	// Legacy migration: routing.auto-model-priority -> auto-routing.preferences
	if len(cfg.Routing.AutoModelPriority) > 0 {
		for i, model := range cfg.Routing.AutoModelPriority {
			// Calculate a descending preference based on array index
			// e.g. first=0.9, second=0.8, etc., lower bound 0.1
			pref := 0.9 - (float64(i) * 0.1)
			if pref < 0.1 {
				pref = 0.1
			}

			// Add to preferences if not already explicitly defined
			exists := false
			for _, p := range ar.Preferences {
				if p.Model == model {
					exists = true
					break
				}
			}

			if !exists {
				ar.Preferences = append(ar.Preferences, autoroute.ModelPreference{
					Model:      model,
					Preference: pref,
					Reason:     "auto-migrated from routing.auto-model-priority",
				})
			}
		}
		// Clear legacy array to prevent confusion
		cfg.Routing.AutoModelPriority = nil
		cfg.legacyMigrationPending = true
	}

	// Apply safe defaults if structural components are missing
	if ar.MaxResolution <= 0 {
		ar.MaxResolution = 5 * time.Millisecond
	}

	// Scoring weights default to even sum if heavily invalid
	sum := ar.Weights.Availability + ar.Weights.Quota + ar.Weights.Latency + ar.Weights.SuccessRate
	if sum < 0.99 || sum > 1.01 {
		ar.Weights.Availability = 0.35
		ar.Weights.Quota = 0.25
		ar.Weights.Latency = 0.20
		ar.Weights.SuccessRate = 0.20
	}

	if ar.Discovery.ProbeInterval <= 0 {
		ar.Discovery.ProbeInterval = 15 * time.Minute
	}
	if ar.Discovery.ProbeTimeout <= 0 {
		ar.Discovery.ProbeTimeout = 5 * time.Second
	}
	if ar.Discovery.CacheTTL <= 0 {
		ar.Discovery.CacheTTL = 24 * time.Hour
	}
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

// ValidateVirtualModels normalizes and validates the virtual-models block.
func (cfg *Config) ValidateVirtualModels() error {
	if cfg == nil || len(cfg.SDKConfig.VirtualModels) == 0 {
		return nil
	}

	normalized := make(map[string]VirtualModelConfig, len(cfg.SDKConfig.VirtualModels))
	for rawID, pool := range cfg.SDKConfig.VirtualModels {
		poolID := strings.TrimSpace(rawID)
		if poolID == "" {
			return fmt.Errorf("virtual-models contains empty model id")
		}
		if pool.Strategy == "" {
			pool.Strategy = "weighted-round-robin"
		}
		if pool.ResponseModel == "" {
			pool.ResponseModel = "requested"
		}
		strategy := strings.ToLower(strings.TrimSpace(pool.Strategy))
		if strategy != "weighted-round-robin" && strategy != "round-robin" {
			return fmt.Errorf("virtual-models.%s has unsupported strategy %q", poolID, pool.Strategy)
		}
		pool.Strategy = strategy
		seenMembers := make(map[string]struct{}, len(pool.Members))
		enabledCount := 0
		for i := range pool.Members {
			member := &pool.Members[i]
			member.ID = strings.TrimSpace(member.ID)
			member.Provider = strings.ToLower(strings.TrimSpace(member.Provider))
			member.Model = strings.TrimSpace(member.Model)
			member.Visibility = strings.ToLower(strings.TrimSpace(member.Visibility))
			if member.ID == "" {
				return fmt.Errorf("virtual-models.%s member at index %d missing id", poolID, i)
			}
			memberKey := strings.ToLower(member.ID)
			if _, exists := seenMembers[memberKey]; exists {
				return fmt.Errorf("virtual-models.%s duplicate member id %q", poolID, member.ID)
			}
			seenMembers[memberKey] = struct{}{}
			if member.Provider == "" {
				return fmt.Errorf("virtual-models.%s member %s missing provider", poolID, member.ID)
			}
			// Provider identifiers are lower-case opaque backend IDs for virtual
			// pools. They may be supplied by runtime/provider plugins, so
			// validation only enforces shape here and leaves resolution to
			// request time.
			if member.Model == "" {
				return fmt.Errorf("virtual-models.%s member %s missing model", poolID, member.ID)
			}
			if member.Weight < 0 {
				return fmt.Errorf("virtual-models.%s member %s has invalid negative weight", poolID, member.ID)
			}
			if member.Weight == 0 {
				member.Weight = 1
			}
			if member.Enabled == nil || *member.Enabled {
				enabledCount++
			}
			member.Capabilities.Operations = normalizeStringList(member.Capabilities.Operations)
			member.Capabilities.Input = normalizeStringList(member.Capabilities.Input)
			member.Capabilities.Output = normalizeStringList(member.Capabilities.Output)
			member.Capabilities.ProofRequiredFor = normalizeStringList(member.Capabilities.ProofRequiredFor)
		}
		if pool.Expose && enabledCount == 0 {
			return fmt.Errorf("virtual-models.%s is exposed but has zero enabled members", poolID)
		}
		normalized[poolID] = pool
	}
	cfg.SDKConfig.VirtualModels = normalized
	return nil
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		v := strings.ToLower(strings.TrimSpace(s))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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

// SanitizeMemory validates and normalizes memory system configuration.
// It ensures directories are set, retention days are positive, and log sizes are reasonable.
func (cfg *Config) SanitizeMemory() {
	if cfg == nil {
		return
	}

	mem := &cfg.Memory

	// Normalize base directory
	mem.BaseDir = strings.TrimSpace(mem.BaseDir)
	if mem.BaseDir == "" {
		mem.BaseDir = ".switchailocal/memory"
	}

	// Validate retention days
	if mem.RetentionDays < 0 {
		mem.RetentionDays = 0 // 0 means no automatic cleanup
	}
	if mem.RetentionDays > 3650 { // Max 10 years
		mem.RetentionDays = 3650
	}

	// Validate max log size
	if mem.MaxLogSizeMB < 0 {
		mem.MaxLogSizeMB = 0 // 0 means no rotation
	}
	if mem.MaxLogSizeMB > 10000 { // Max 10GB per file
		mem.MaxLogSizeMB = 10000
	}
}

// SanitizeSteering validates and normalizes steering engine configuration.
// It ensures the rules directory is set and hot-reload is properly configured.
func (cfg *Config) SanitizeSteering() {
	if cfg == nil {
		return
	}

	steer := &cfg.Steering

	// Normalize rules directory (support both rules-dir and steering-dir for compatibility)
	steer.RulesDir = strings.TrimSpace(steer.RulesDir)
	steer.SteeringDir = strings.TrimSpace(steer.SteeringDir)

	// If RulesDir is empty but SteeringDir is set, use SteeringDir
	if steer.RulesDir == "" && steer.SteeringDir != "" {
		steer.RulesDir = steer.SteeringDir
	}

	// If both are empty, set default
	if steer.RulesDir == "" {
		steer.RulesDir = ".switchailocal/steering"
	}

	// Sync SteeringDir with RulesDir for backwards compatibility
	steer.SteeringDir = steer.RulesDir

	// Hot-reload defaults to true when steering is enabled
	// No additional validation needed for boolean
}

// SanitizeHooks validates and normalizes hooks system configuration.
// It ensures the hooks directory is set and hot-reload is properly configured.
func (cfg *Config) SanitizeHooks() {
	if cfg == nil {
		return
	}

	hooks := &cfg.Hooks

	// Normalize hooks directory
	hooks.HooksDir = strings.TrimSpace(hooks.HooksDir)
	if hooks.HooksDir == "" {
		hooks.HooksDir = ".switchailocal/hooks"
	}

	// Hot-reload defaults to true when hooks are enabled
	// No additional validation needed for boolean
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
