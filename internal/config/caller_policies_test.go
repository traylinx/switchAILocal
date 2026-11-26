// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func validCallerPolicies() CallerPoliciesConfig {
	return CallerPoliciesConfig{
		StateFile: "state/caller-usage.json",
		Callers: []CallerPolicyConfig{{
			ID: "agent-1", APIKey: "inbound-placeholder", AllowedModels: []string{"chat-model"}, MaxConcurrency: 2, DailyTokenCap: 1000,
		}},
	}
}

func TestValidateCallerPoliciesNormalizesAndAcceptsBoundedCaller(t *testing.T) {
	cfg := SDKConfig{CallerPolicies: validCallerPolicies()}
	cfg.CallerPolicies.Callers[0].AllowedModels = []string{" chat-model ", "chat-model"}
	if err := cfg.ValidateCallerPolicies(); err != nil {
		t.Fatal(err)
	}
	models := cfg.CallerPolicies.Callers[0].AllowedModels
	if len(models) != 1 || models[0] != "chat-model" {
		t.Fatalf("allowed models were not normalized: %#v", models)
	}
}

func TestValidateCallerPoliciesRejectsUnboundedOrAmbiguousConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CallerPoliciesConfig)
	}{
		{"missing state file", func(c *CallerPoliciesConfig) { c.StateFile = "" }},
		{"missing id", func(c *CallerPoliciesConfig) { c.Callers[0].ID = "" }},
		{"unsafe id", func(c *CallerPoliciesConfig) { c.Callers[0].ID = "agent one" }},
		{"missing key", func(c *CallerPoliciesConfig) { c.Callers[0].APIKey = "" }},
		{"empty models", func(c *CallerPoliciesConfig) { c.Callers[0].AllowedModels = nil }},
		{"empty model", func(c *CallerPoliciesConfig) { c.Callers[0].AllowedModels = []string{""} }},
		{"zero concurrency", func(c *CallerPoliciesConfig) { c.Callers[0].MaxConcurrency = 0 }},
		{"zero daily cap", func(c *CallerPoliciesConfig) { c.Callers[0].DailyTokenCap = 0 }},
		{"duplicate id", func(c *CallerPoliciesConfig) { c.Callers = append(c.Callers, c.Callers[0]) }},
		{"duplicate key", func(c *CallerPoliciesConfig) {
			other := c.Callers[0]
			other.ID = "agent-2"
			c.Callers = append(c.Callers, other)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies := validCallerPolicies()
			tt.mutate(&policies)
			cfg := SDKConfig{CallerPolicies: policies}
			if err := cfg.ValidateCallerPolicies(); err == nil {
				t.Fatal("invalid caller policy unexpectedly accepted")
			}
		})
	}
}

func TestValidateCallerPoliciesRejectsLegacyKeyCallerIDCollision(t *testing.T) {
	cfg := SDKConfig{APIKeys: []string{"agent-1"}, CallerPolicies: validCallerPolicies()}
	if err := cfg.ValidateCallerPolicies(); err == nil {
		t.Fatal("legacy api-key equal to caller id unexpectedly accepted")
	}
}

func TestInboundAPIKeysIncludesLegacyAndNamedCallerKeys(t *testing.T) {
	cfg := SDKConfig{APIKeys: []string{"legacy", "shared"}, CallerPolicies: validCallerPolicies()}
	cfg.CallerPolicies.Callers[0].APIKey = "shared"
	cfg.CallerPolicies.Callers = append(cfg.CallerPolicies.Callers, CallerPolicyConfig{
		ID: "agent-2", APIKey: "second", AllowedModels: []string{"chat-model"}, MaxConcurrency: 1, DailyTokenCap: 10,
	})
	keys := cfg.InboundAPIKeys()
	if len(keys) != 3 || keys[0] != "legacy" || keys[1] != "shared" || keys[2] != "second" {
		t.Fatalf("unexpected inbound keys: %#v", keys)
	}
}

func TestLoadConfigRejectsInvalidCallerPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("caller-policies:\n  state-file: state.json\n  callers:\n    - id: agent-1\n      api-key: placeholder\n      allowed-models: [chat-model]\n      max-concurrency: 0\n      daily-token-cap: 100\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigOptional(path, false); err == nil {
		t.Fatal("invalid policy unexpectedly loaded")
	}
	if _, err := LoadConfigOptional(path, true); err == nil {
		t.Fatal("optional load downgraded invalid policy to an open config")
	}
}
