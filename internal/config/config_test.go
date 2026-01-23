// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"testing"
)

func TestLoadConfig_SecureDefaults(t *testing.T) {
	// Create a temporary empty config file
	f, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	// Load the config (should apply defaults)
	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Secure Defaults
	if !cfg.WebsocketAuth {
		t.Error("Security Regression: WebsocketAuth should be true by default")
	}

	if cfg.Host != "" {
		t.Errorf("Host should be empty by default (bind all), got: %s", cfg.Host)
	}
}

func TestLoadConfig_ExplicitDisable(t *testing.T) {
	// Create a config file that explicitly disables auth
	content := []byte("ws-auth: false")
	f, err := os.CreateTemp("", "config_insecure_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.WebsocketAuth {
		t.Error("Config Loader failed to respect explicit disable of WebsocketAuth")
	}
}
