// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"testing"
)

// TestLoadConfig_NativeToolsParsed pins the YAML parse contract for
// the native_tools key on an openai-compatibility model. Agentic
// callers (OpenClaw, Hermes) rely on /v1/models to discover the
// provider-native tools each model supports; if this parse breaks,
// the discovery endpoint would silently omit the field and all
// downstream pieces (Phase 2 plugin, Phase 3 CLI, Phase 4 passthrough)
// would regress to pre-2026-04-22 autoinject-only behavior.
func TestLoadConfig_NativeToolsParsed(t *testing.T) {
	content := []byte(`
openai-compatibility:
  - name: "minimax"
    base-url: "https://api.minimax.io/v1"
    api-key-entries:
      - api-key: "sk-x"
    models:
      - name: "MiniMax-M2.7"
        alias: "ail-compound"
        native_tools:
          - type: "web_search"
            description: "MiniMax native web search"
            params:
              force_search: { type: "boolean", default: false }
              limit:        { type: "integer", default: 3 }
      - name: "image-01"
        alias: "ail-image"
`)
	f, err := os.CreateTemp("", "config_native_tools_*.yaml")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.OpenAICompatibility) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.OpenAICompatibility))
	}
	models := cfg.OpenAICompatibility[0].Models
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	// First model: MiniMax-M2.7 with a single web_search native tool.
	m := models[0]
	if m.Alias != "ail-compound" {
		t.Errorf("alias: got %q want ail-compound", m.Alias)
	}
	if len(m.NativeTools) != 1 {
		t.Fatalf("expected 1 native_tool entry, got %d", len(m.NativeTools))
	}
	nt := m.NativeTools[0]
	if nt.Type != "web_search" {
		t.Errorf("native_tools[0].type: got %q want web_search", nt.Type)
	}
	if nt.Description == "" {
		t.Error("native_tools[0].description: want non-empty")
	}
	if _, ok := nt.Params["force_search"]; !ok {
		t.Errorf("native_tools[0].params missing force_search: %#v", nt.Params)
	}

	// Second model: no native_tools key — must surface as empty slice,
	// not nil-with-garbage. This is what the /v1/models handler keys
	// off to decide whether to emit the field.
	m2 := models[1]
	if len(m2.NativeTools) != 0 {
		t.Errorf("native_tools should be empty for image-01, got %#v", m2.NativeTools)
	}
}
