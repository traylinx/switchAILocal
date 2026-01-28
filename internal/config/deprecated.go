package config

import (
	"strings"

	"gopkg.in/yaml.v3"
)

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
