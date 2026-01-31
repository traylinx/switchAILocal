// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package util provides utility functions used across the switchAILocal application.
// These functions handle common tasks such as determining AI service providers
// from model names and managing HTTP proxies.
package util

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/constant"
	"github.com/traylinx/switchAILocal/internal/registry"
)

// ProviderErrorType defines the type of provider-related error.
type ProviderErrorType int

const (
	// ErrUnknownProvider indicates the provider prefix is not recognized.
	ErrUnknownProvider ProviderErrorType = iota
	// ErrProviderNotConfigured indicates the provider exists but has no active credentials.
	ErrProviderNotConfigured
	// ErrModelNotAvailable indicates the model is not available from the specified provider.
	ErrModelNotAvailable
)

// ProviderError represents an error related to provider routing.
type ProviderError struct {
	Type               ProviderErrorType
	Provider           string
	Model              string
	Message            string
	AvailableProviders []string
}

func (e *ProviderError) Error() string {
	return e.Message
}

// KnownProviders contains all recognized provider prefixes.
var KnownProviders = map[string]string{
	"switchai":         "Traylinx switchAI",
	constant.Gemini:    "Google Gemini",
	constant.GeminiCLI: "Gemini CLI",
	constant.Claude:    "Anthropic Claude",
	constant.ClaudeCLI: "Claude CLI",
	constant.Codex:     "OpenAI Codex",
	"ollama":           "Ollama (Local)",
	constant.VibeCLI:   "Vibe CLI",
	constant.OpenAI:    "OpenAI",
	"openai-compat":    "OpenAI Compatible",
	"opencode":         "OpenCode Agent",
}

// ParseProviderPrefix extracts provider prefix from model name.
// Syntax: "provider:model" where ":" is the separator.
// Returns (provider, model) or ("", originalModel) if no prefix.
//
// Examples:
//   - "ollama:llama3.2" -> ("ollama", "llama3.2")
//   - "geminicli:" -> ("geminicli", "")
//   - "llama3.2" -> ("", "llama3.2")
//   - "meta-llama/llama-4" -> ("", "meta-llama/llama-4")
func ParseProviderPrefix(modelName string) (provider, model string) {
	if modelName == "" {
		return "", ""
	}

	// Find the first colon
	idx := strings.Index(modelName, ":")
	if idx <= 0 {
		// No colon, or colon at start (invalid) - treat as model name
		return "", modelName
	}

	candidateProvider := strings.ToLower(modelName[:idx])

	// Check if it's a known provider
	if _, known := KnownProviders[candidateProvider]; known {
		model = strings.TrimSpace(modelName[idx+1:])
		return candidateProvider, model
	}

	// Not a known provider, treat entire string as model name
	return "", modelName
}

// ValidateProviderPrefix checks if a provider prefix is valid and active.
// Returns the actual model name if valid, or an error with helpful message.
func ValidateProviderPrefix(providerPrefix, model string) (string, error) {
	if providerPrefix == "" {
		return model, nil
	}

	// Check if provider is known
	if _, known := KnownProviders[providerPrefix]; !known {
		available := make([]string, 0, len(KnownProviders))
		for p := range KnownProviders {
			available = append(available, p)
		}
		return "", &ProviderError{
			Type:               ErrUnknownProvider,
			Provider:           providerPrefix,
			Message:            fmt.Sprintf("Unknown provider '%s'. Use one of: %s", providerPrefix, strings.Join(available, ", ")),
			AvailableProviders: available,
		}
	}

	// Local CLI providers don't pre-register models, so trust them if known
	// They will handle errors during execution if actually unavailable
	localProviders := map[string]bool{
		constant.GeminiCLI: true,
		constant.ClaudeCLI: true,
		constant.VibeCLI:   true,
		"ollama":           true,
		"opencode":         true,
	}
	if localProviders[providerPrefix] {
		return model, nil
	}

	// For API-based providers, check if they have active credentials in registry
	activeProviders := registry.GetGlobalRegistry().GetAllProviders()
	for _, p := range activeProviders {
		if p.ID == providerPrefix && p.Status == "active" {
			return model, nil
		}
	}

	return "", &ProviderError{
		Type:     ErrProviderNotConfigured,
		Provider: providerPrefix,
		Message:  fmt.Sprintf("Provider '%s' is not active. Please configure it in config.yaml or use --login flags.", providerPrefix),
	}
}

// GetProviderName determines all AI service providers capable of serving a registered model.
// It first queries the global model registry to retrieve the providers backing the supplied model name.
// When the model has not been registered yet, it falls back to legacy string heuristics to infer
// potential providers.
//
// Supported providers include (but are not limited to):
//   - "gemini" for Google's Gemini family
//   - "codex" for OpenAI GPT-compatible providers
//   - "claude" for Anthropic models
//   - "qwen" for Alibaba's Qwen models
//   - "openai-compatibility" for external OpenAI-compatible providers
//
// Parameters:
//   - modelName: The name of the model to identify providers for.
//   - cfg: The application configuration containing OpenAI compatibility settings.
//
// Returns:
//   - []string: All provider identifiers capable of serving the model, ordered by preference.
func GetProviderName(modelName string) []string {
	if modelName == "" {
		return nil
	}

	providers := make([]string, 0, 4)
	seen := make(map[string]struct{})

	appendProvider := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		providers = append(providers, name)
	}

	for _, provider := range registry.GetGlobalRegistry().GetModelProviders(modelName) {
		appendProvider(provider)
	}

	if len(providers) > 0 {
		return providers
	}

	return providers
}

// GetLoginHint returns the recommended command-line flag or instruction
// to obtain credentials for the given provider.
func GetLoginHint(provider string) string {
	switch strings.ToLower(provider) {
	case constant.GeminiCLI:
		return "--login"
	case constant.Codex:
		return "--codex-login"
	case constant.Claude:
		return "--claude-login"
	case "qwen":
		return "--qwen-login"
	case constant.VibeCLI:
		return "--vibe-login"
	case "ollama":
		return "--ollama-login"
	case "iflow":
		return "--iflow-login"
	case "antigravity":
		return "--antigravity-login"
	case "vertex":
		return "--vertex-import <path-to-json>"
	case "gemini", "claude-api", "openai", "switchai":
		return "Please configure the API key in config.yaml"
	default:
		return "Please check configuration or run --help for login options"
	}
}

// ResolveAutoModel resolves the "auto" model name to an actual available model.
// It uses an empty handler type to get any available model from the registry.
//
// Parameters:
//   - modelName: The model name to check (should be "auto")
//   - priorityList: Optional list of model IDs to prioritize
//
// Returns:
//   - string: The resolved model name, or the original if not "auto" or resolution fails
func ResolveAutoModel(modelName string, priorityList []string) string {
	if modelName != "auto" {
		return modelName
	}

	// Use empty string as handler type to get any available model
	firstModel, err := registry.GetGlobalRegistry().GetFirstAvailableModel("", priorityList)
	if err != nil {
		log.Warnf("Failed to resolve 'auto' model: %v, falling back to original model name", err)
		return modelName
	}

	log.Infof("Resolved 'auto' model to: %s", firstModel)
	return firstModel
}

// IsOpenAICompatibilityAlias checks if the given model name is an alias
// configured for OpenAI compatibility routing.
//
// Parameters:
//   - modelName: The model name to check
//   - cfg: The application configuration containing OpenAI compatibility settings
//
// Returns:
//   - bool: True if the model name is an OpenAI compatibility alias, false otherwise
func IsOpenAICompatibilityAlias(modelName string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}

	for _, compat := range cfg.OpenAICompatibility {
		for _, model := range compat.Models {
			if model.Alias == modelName {
				return true
			}
		}
	}
	return false
}

// GetOpenAICompatibilityConfig returns the OpenAI compatibility configuration
// and model details for the given alias.
//
// Parameters:
//   - alias: The model alias to find configuration for
//   - cfg: The application configuration containing OpenAI compatibility settings
//
// Returns:
//   - *config.OpenAICompatibility: The matching compatibility configuration, or nil if not found
//   - *config.OpenAICompatibilityModel: The matching model configuration, or nil if not found
func GetOpenAICompatibilityConfig(alias string, cfg *config.Config) (*config.OpenAICompatibility, *config.OpenAICompatibilityModel) {
	if cfg == nil {
		return nil, nil
	}

	for _, compat := range cfg.OpenAICompatibility {
		for _, model := range compat.Models {
			if model.Alias == alias {
				return &compat, &model
			}
		}
	}
	return nil, nil
}

// InArray checks if a string exists in a slice of strings.
// It iterates through the slice and returns true if the target string is found,
// otherwise it returns false.
//
// Parameters:
//   - hystack: The slice of strings to search in
//   - needle: The string to search for
//
// Returns:
//   - bool: True if the string is found, false otherwise
func InArray(hystack []string, needle string) bool {
	for _, item := range hystack {
		if needle == item {
			return true
		}
	}
	return false
}

// HideAPIKey obscures an API key for logging purposes, showing only the first and last few characters.
//
// Parameters:
//   - apiKey: The API key to hide.
//
// Returns:
//   - string: The obscured API key.
func HideAPIKey(apiKey string) string {
	if len(apiKey) > 8 {
		return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
	} else if len(apiKey) > 4 {
		return apiKey[:2] + "..." + apiKey[len(apiKey)-2:]
	} else if len(apiKey) > 2 {
		return apiKey[:1] + "..." + apiKey[len(apiKey)-1:]
	}
	return apiKey
}

// maskAuthorizationHeader masks the Authorization header value while preserving the auth type prefix.
// Common formats: "Bearer <token>", "Basic <credentials>", "ApiKey <key>", etc.
// It preserves the prefix (e.g., "Bearer ") and only masks the token/credential part.
//
// Parameters:
//   - value: The Authorization header value
//
// Returns:
//   - string: The masked Authorization value with prefix preserved
func MaskAuthorizationHeader(value string) string {
	parts := strings.SplitN(strings.TrimSpace(value), " ", 2)
	if len(parts) < 2 {
		return HideAPIKey(value)
	}
	return parts[0] + " " + HideAPIKey(parts[1])
}

// MaskSensitiveHeaderValue masks sensitive header values while preserving expected formats.
//
// Behavior by header key (case-insensitive):
//   - "Authorization": Preserve the auth type prefix (e.g., "Bearer ") and mask only the credential part.
//   - Headers containing "api-key": Mask the entire value using HideAPIKey.
//   - Others: Return the original value unchanged.
//
// Parameters:
//   - key:   The HTTP header name to inspect (case-insensitive matching).
//   - value: The header value to mask when sensitive.
//
// Returns:
//   - string: The masked value according to the header type; unchanged if not sensitive.
func MaskSensitiveHeaderValue(key, value string) string {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(lowerKey, "authorization"):
		return MaskAuthorizationHeader(value)
	case strings.Contains(lowerKey, "api-key"),
		strings.Contains(lowerKey, "apikey"),
		strings.Contains(lowerKey, "token"),
		strings.Contains(lowerKey, "secret"):
		return HideAPIKey(value)
	default:
		return value
	}
}

// MaskSensitiveQuery masks sensitive query parameters, e.g. auth_token, within the raw query string.
func MaskSensitiveQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	changed := false
	for i, part := range parts {
		if part == "" {
			continue
		}
		keyPart := part
		valuePart := ""
		if idx := strings.Index(part, "="); idx >= 0 {
			keyPart = part[:idx]
			valuePart = part[idx+1:]
		}
		decodedKey, err := url.QueryUnescape(keyPart)
		if err != nil {
			decodedKey = keyPart
		}
		if !shouldMaskQueryParam(decodedKey) {
			continue
		}
		decodedValue, err := url.QueryUnescape(valuePart)
		if err != nil {
			decodedValue = valuePart
		}
		masked := HideAPIKey(strings.TrimSpace(decodedValue))
		parts[i] = keyPart + "=" + url.QueryEscape(masked)
		changed = true
	}
	if !changed {
		return raw
	}
	return strings.Join(parts, "&")
}

func shouldMaskQueryParam(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	key = strings.TrimSuffix(key, "[]")
	if key == "key" || strings.Contains(key, "api-key") || strings.Contains(key, "apikey") || strings.Contains(key, "api_key") {
		return true
	}
	if strings.Contains(key, "token") || strings.Contains(key, "secret") {
		return true
	}
	return false
}

// LuaDateFormatToGo converts a LUA date format string (strftime style)
// to a Go time.Format string (layout style).
// Note: This is an architectural mapping; for standard LUA plugins we support
// the most common strftime tokens used for basic logging and headers.
func LuaDateFormatToGo(luaFormat string) string {
	if luaFormat == "" || luaFormat == "%c" {
		return time.RFC3339
	}
	replacements := map[string]string{
		"%Y": "2006",
		"%m": "01",
		"%d": "02",
		"%H": "15",
		"%M": "04",
		"%S": "05",
		"%z": "-0700",
		"%Z": "MST",
	}
	out := luaFormat
	for l, g := range replacements {
		out = strings.ReplaceAll(out, l, g)
	}
	// Default fallback if no known tokens found
	if out == luaFormat && !strings.Contains(luaFormat, "-") && !strings.Contains(luaFormat, ":") {
		return time.RFC3339
	}
	return out
}

var (
	// sensitiveJSONKeys matches keys in JSON that should be masked.
	// It matches "key": "value" patterns where key is one of the sensitive terms.
	// It handles escaped quotes within the value string.
	sensitiveJSONKeys = regexp.MustCompile(`(?i)"(api_?key|apikey|token|secret|password|authorization)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
)

// MaskSensitiveJSONBody masks sensitive fields in a JSON body.
// It uses regex to identify and mask values of sensitive keys while preserving the JSON structure.
func MaskSensitiveJSONBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	// We use ReplaceAllFunc to handle each match individually
	return sensitiveJSONKeys.ReplaceAllFunc(body, func(match []byte) []byte {
		// match contains the full "key": "value" string
		submatches := sensitiveJSONKeys.FindSubmatchIndex(match)
		if len(submatches) < 6 {
			return match
		}

		// submatches indices:
		// [0, 1] - full match
		// [2, 3] - key group
		// [4, 5] - value group

		keyStart := submatches[2]
		keyEnd := submatches[3]
		valStart := submatches[4]
		valEnd := submatches[5]

		key := string(match[keyStart:keyEnd])
		val := string(match[valStart:valEnd])

		// Determine masking strategy based on key
		var maskedVal string
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "secret") {
			maskedVal = "******"
		} else {
			maskedVal = HideAPIKey(val)
		}

		// Construct the replacement
		// We replace the value part within the match
		var result bytes.Buffer
		result.Write(match[:valStart])
		result.WriteString(maskedVal)
		result.Write(match[valEnd:])

		return result.Bytes()
	})
}
