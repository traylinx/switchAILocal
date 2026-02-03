// Package integration provides coordination and lifecycle management for intelligent systems.
package integration

import (
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"

	log "github.com/sirupsen/logrus"
)

// ConvertHeartbeatConfig converts config.HeartbeatConfig to heartbeat.HeartbeatConfig.
// It parses duration strings and applies defaults for invalid values.
func ConvertHeartbeatConfig(cfg *config.HeartbeatConfig) *heartbeat.HeartbeatConfig {
	if cfg == nil {
		return heartbeat.DefaultHeartbeatConfig()
	}

	hbConfig := &heartbeat.HeartbeatConfig{
		Enabled:                cfg.Enabled,
		AutoDiscovery:          cfg.AutoDiscovery,
		QuotaWarningThreshold:  cfg.QuotaWarningThreshold,
		QuotaCriticalThreshold: cfg.QuotaCriticalThreshold,
		MaxConcurrentChecks:    cfg.MaxConcurrentChecks,
		RetryAttempts:          cfg.RetryAttempts,
	}

	// Parse interval
	if cfg.Interval != "" {
		interval, err := time.ParseDuration(cfg.Interval)
		if err != nil {
			log.Warnf("Invalid heartbeat interval '%s', using default 5m: %v", cfg.Interval, err)
			hbConfig.Interval = 5 * time.Minute
		} else {
			hbConfig.Interval = interval
		}
	} else {
		hbConfig.Interval = 5 * time.Minute
	}

	// Parse timeout
	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			log.Warnf("Invalid heartbeat timeout '%s', using default 5s: %v", cfg.Timeout, err)
			hbConfig.Timeout = 5 * time.Second
		} else {
			hbConfig.Timeout = timeout
		}
	} else {
		hbConfig.Timeout = 5 * time.Second
	}

	// Parse retry delay
	if cfg.RetryDelay != "" {
		retryDelay, err := time.ParseDuration(cfg.RetryDelay)
		if err != nil {
			log.Warnf("Invalid heartbeat retry delay '%s', using default 1s: %v", cfg.RetryDelay, err)
			hbConfig.RetryDelay = time.Second
		} else {
			hbConfig.RetryDelay = retryDelay
		}
	} else {
		hbConfig.RetryDelay = time.Second
	}

	// Apply defaults for zero values
	if hbConfig.MaxConcurrentChecks == 0 {
		hbConfig.MaxConcurrentChecks = 10
	}
	if hbConfig.QuotaWarningThreshold == 0 {
		hbConfig.QuotaWarningThreshold = 0.80
	}
	if hbConfig.QuotaCriticalThreshold == 0 {
		hbConfig.QuotaCriticalThreshold = 0.95
	}

	return hbConfig
}

// RegisterProviderCheckers registers health checkers for all configured providers.
// It creates appropriate checker instances based on the main configuration.
func RegisterProviderCheckers(monitor heartbeat.HeartbeatMonitor, cfg *config.Config) error {
	if monitor == nil || cfg == nil {
		return nil
	}

	var registeredCount int

	// Register Ollama checker if enabled
	if cfg.Ollama.Enabled {
		baseURL := cfg.Ollama.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		checker := heartbeat.NewOllamaHealthChecker(baseURL)
		if err := monitor.RegisterChecker(checker); err != nil {
			log.Warnf("Failed to register Ollama health checker: %v", err)
		} else {
			log.Debugf("Registered Ollama health checker (baseURL: %s)", baseURL)
			registeredCount++
		}
	}

	// Register Gemini API checkers
	for i, geminiKey := range cfg.GeminiKey {
		if geminiKey.APIKey == "" {
			continue
		}
		checker := heartbeat.NewGeminiAPIHealthChecker(geminiKey.APIKey)
		if err := monitor.RegisterChecker(checker); err != nil {
			log.Warnf("Failed to register Gemini API health checker #%d: %v", i, err)
		} else {
			log.Debugf("Registered Gemini API health checker #%d", i)
			registeredCount++
		}
		// Only register one checker for Gemini (they all use the same API)
		break
	}

	// Register Claude API checkers
	for i, claudeKey := range cfg.ClaudeKey {
		if claudeKey.APIKey == "" {
			continue
		}
		checker := heartbeat.NewClaudeAPIHealthChecker(claudeKey.APIKey)
		if err := monitor.RegisterChecker(checker); err != nil {
			log.Warnf("Failed to register Claude API health checker #%d: %v", i, err)
		} else {
			log.Debugf("Registered Claude API health checker #%d", i)
			registeredCount++
		}
		// Only register one checker for Claude (they all use the same API)
		break
	}

	// Register SwitchAI API checkers
	for i, switchAIKey := range cfg.SwitchAIKey {
		if switchAIKey.APIKey == "" {
			continue
		}
		baseURL := switchAIKey.BaseURL
		if baseURL == "" {
			baseURL = "https://switchai.traylinx.com/v1"
		}
		checker := heartbeat.NewSwitchAIHealthChecker(switchAIKey.APIKey, baseURL)
		if err := monitor.RegisterChecker(checker); err != nil {
			log.Warnf("Failed to register SwitchAI health checker #%d: %v", i, err)
		} else {
			log.Debugf("Registered SwitchAI health checker #%d (baseURL: %s)", i, baseURL)
			registeredCount++
		}
		// Only register one checker for SwitchAI
		break
	}

	// Register OpenAI-compatible providers
	for i, compat := range cfg.OpenAICompatibility {
		if compat.BaseURL == "" {
			continue
		}
		name := compat.Name
		if name == "" {
			name = "openai-compat-" + string(rune(i))
		}
		apiKey := ""
		if len(compat.APIKeyEntries) > 0 {
			apiKey = compat.APIKeyEntries[0].APIKey
		}
		checker := heartbeat.NewOpenAICompatibilityHealthChecker(name, compat.BaseURL, apiKey)
		if err := monitor.RegisterChecker(checker); err != nil {
			log.Warnf("Failed to register OpenAI-compatible health checker '%s': %v", name, err)
		} else {
			log.Debugf("Registered OpenAI-compatible health checker '%s' (baseURL: %s)", name, compat.BaseURL)
			registeredCount++
		}
	}

	// Register LM Studio checker if enabled
	if cfg.LMStudio.Enabled {
		baseURL := cfg.LMStudio.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:1234/v1"
		}
		// LM Studio is OpenAI-compatible
		checker := heartbeat.NewOpenAICompatibilityHealthChecker("lmstudio", baseURL, "")
		if err := monitor.RegisterChecker(checker); err != nil {
			log.Warnf("Failed to register LM Studio health checker: %v", err)
		} else {
			log.Debugf("Registered LM Studio health checker (baseURL: %s)", baseURL)
			registeredCount++
		}
	}

	log.Infof("Registered %d provider health checkers", registeredCount)
	return nil
}
