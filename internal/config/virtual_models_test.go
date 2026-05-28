package config

import "testing"

func vmBool(v bool) *bool { return &v }

func baseConfigForVirtualModels() *Config {
	return &Config{OpenAICompatibility: []OpenAICompatibility{{Name: "minimax", BaseURL: "https://example.invalid", Models: []OpenAICompatibilityModel{{Name: "MiniMax-M2.7", Alias: "ail-compound-minimax"}}}, {Name: "deepseek", BaseURL: "https://example.invalid", Models: []OpenAICompatibilityModel{{Name: "deepseek-v4-pro", Alias: "ail-fast"}}}}}
}

func TestValidateVirtualModels(t *testing.T) {
	cfg := baseConfigForVirtualModels()
	cfg.VirtualModels = map[string]VirtualModelConfig{"ail-compound": {Expose: true, Members: []VirtualModelMemberConfig{{ID: "m1", Provider: "minimax", Model: "MiniMax-M2.7", Weight: 1, Enabled: vmBool(true), Capabilities: VirtualModelCapabilitiesConfig{Operations: []string{"chat"}, Input: []string{"TEXT"}, Output: []string{"text"}}}, {ID: "m2", Provider: "deepseek", Model: "deepseek-v4-pro", Enabled: vmBool(true)}}}}
	if err := cfg.ValidateVirtualModels(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := cfg.VirtualModels["ail-compound"].Strategy; got != "weighted-round-robin" {
		t.Fatalf("strategy default %q", got)
	}
	if got := cfg.VirtualModels["ail-compound"].Members[0].Capabilities.Input[0]; got != "text" {
		t.Fatalf("input normalized %q", got)
	}
}

func TestValidateVirtualModelsRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"duplicate", func(c *Config) {
			c.VirtualModels = map[string]VirtualModelConfig{"ail-compound": {Expose: true, Members: []VirtualModelMemberConfig{{ID: "x", Provider: "minimax", Model: "a"}, {ID: "x", Provider: "deepseek", Model: "b"}}}}
		}},
		{"unknown provider", func(c *Config) {
			c.VirtualModels = map[string]VirtualModelConfig{"ail-compound": {Expose: true, Members: []VirtualModelMemberConfig{{ID: "x", Provider: "kimi", Model: "k"}}}}
		}},
		{"bad weight", func(c *Config) {
			c.VirtualModels = map[string]VirtualModelConfig{"ail-compound": {Expose: true, Members: []VirtualModelMemberConfig{{ID: "x", Provider: "minimax", Model: "m", Weight: -1}}}}
		}},
		{"exposed no enabled", func(c *Config) {
			c.VirtualModels = map[string]VirtualModelConfig{"ail-compound": {Expose: true, Members: []VirtualModelMemberConfig{{ID: "x", Provider: "minimax", Model: "m", Enabled: vmBool(false)}}}}
		}},
	}
	for _, tt := range tests {
		cfg := baseConfigForVirtualModels()
		tt.mutate(cfg)
		if err := cfg.ValidateVirtualModels(); err == nil {
			t.Fatalf("%s: expected error", tt.name)
		}
	}
}
