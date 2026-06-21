package virtualmodels

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/traylinx/switchAILocal/internal/config"
)

func boolPtr(v bool) *bool { return &v }

func testConfig() *config.SDKConfig {
	return &config.SDKConfig{VirtualModels: map[string]config.VirtualModelConfig{
		"ail-compound": {
			Expose:   true,
			Strategy: "weighted-round-robin",
			Members: []config.VirtualModelMemberConfig{
				{ID: "text-a", Provider: "opaque-provider-a", Model: "native-text-a", Enabled: boolPtr(true), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat", "chat_text_tools"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
				{ID: "text-b", Provider: "opaque-provider-b", Model: "native-text-b", Enabled: boolPtr(true), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
				{ID: "disabled-c", Provider: "opaque-provider-c", Model: "native-text-c", Enabled: boolPtr(false), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
				{ID: "vision-d", Provider: "opaque-provider-d", Model: "native-vision-d", Enabled: boolPtr(true), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}}},
			},
		},
	}}
}

func weightedTextConfig(weightA, weightB int) *config.SDKConfig {
	return &config.SDKConfig{VirtualModels: map[string]config.VirtualModelConfig{
		"ail-compound": {
			Expose:   true,
			Strategy: "weighted-round-robin",
			Members: []config.VirtualModelMemberConfig{
				{ID: "backend-a", Provider: "opaque-provider-a", Model: "native-model-a", Weight: weightA, Enabled: boolPtr(true), Capabilities: textCaps()},
				{ID: "backend-b", Provider: "opaque-provider-b", Model: "native-model-b", Weight: weightB, Enabled: boolPtr(true), Capabilities: textCaps()},
			},
		},
	}}
}

func textCaps() config.VirtualModelCapabilitiesConfig {
	return config.VirtualModelCapabilitiesConfig{Operations: []string{"chat"}, Input: []string{"text"}, Output: []string{"text"}}
}

func selectIDs(t *testing.T, router *Router, cfg *config.SDKConfig, n int, body []byte) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		route, err := router.Select(cfg, "ail-compound", body, "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, route.MemberID)
	}
	return ids
}

func countIDs(ids []string) map[string]int {
	counts := map[string]int{}
	for _, id := range ids {
		counts[id]++
	}
	return counts
}

func poolFromDisk(t *testing.T, statePath, key string) RouterPoolState {
	t.Helper()
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var disk RouterState
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.SchemaVersion != routerStateSchemaVersion {
		t.Fatalf("schema version = %d, want %d", disk.SchemaVersion, routerStateSchemaVersion)
	}
	if disk.Algorithm != routerAlgorithm {
		t.Fatalf("algorithm = %q, want %q", disk.Algorithm, routerAlgorithm)
	}
	pool, ok := disk.Pools[key]
	if !ok {
		t.Fatalf("missing pool %s in %#v", key, disk.Pools)
	}
	return pool
}

func TestDetectRequirementsToolHistory(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"text", `{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`, ClassChatText},
		{"one shot tools", `{"model":"ail-compound","tools":[{"type":"function","function":{"name":"x"}}],"messages":[{"role":"user","content":"hi"}]}`, ClassChatTextTools},
		{"assistant tool calls", `{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"1","type":"function","function":{"name":"x","arguments":"{}"}}]}]}`, ClassChatMultiturnTools},
		{"tool role", `{"model":"ail-compound","messages":[{"role":"tool","tool_call_id":"1","content":"ok"}]}`, ClassChatMultiturnTools},
		{"tool call id", `{"model":"ail-compound","messages":[{"role":"assistant","tool_call_id":"1","content":"ok"}]}`, ClassChatMultiturnTools},
		{"responses function output", `{"model":"ail-compound","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`, ClassChatMultiturnTools},
		{"responses input image", `{"model":"ail-compound","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`, ClassChatImageUnderstanding},
		{"responses input audio", `{"model":"ail-compound","input":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"abc","format":"mp3"}}]}]}`, ClassChatAudioUnderstanding},
	}
	for _, tt := range tests {
		got := DetectRequirements([]byte(tt.body), "").Class
		if got != tt.want {
			t.Fatalf("%s: got %s want %s", tt.name, got, tt.want)
		}
	}
}

func TestSelectFiltersDisabledAndMedia(t *testing.T) {
	router := NewRouterWithStatePath(filepath.Join(t.TempDir(), "virtual_models_state.json"))
	cfg := testConfig()
	imageBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"text","text":"color?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`)
	route, err := router.Select(cfg, "ail-compound", imageBody, "")
	if err != nil {
		t.Fatal(err)
	}
	if route.Provider != "opaque-provider-d" || route.MemberID != "vision-d" {
		t.Fatalf("image routed to %#v", route)
	}

	textBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		route, err = router.Select(cfg, "ail-compound", textBody, "")
		if err != nil {
			t.Fatal(err)
		}
		if route.MemberID == "disabled-c" {
			t.Fatal("disabled member selected")
		}
		seen[route.MemberID] = true
	}
	if !seen["text-a"] || !seen["text-b"] {
		t.Fatalf("text did not balance across opaque text backends: %#v", seen)
	}
}

func TestNoEligibleBackendForImageWhenNoMediaMember(t *testing.T) {
	cfg := testConfig()
	pool := cfg.VirtualModels["ail-compound"]
	pool.Members = pool.Members[:3]
	cfg.VirtualModels["ail-compound"] = pool
	router := NewRouterWithStatePath(filepath.Join(t.TempDir(), "virtual_models_state.json"))
	_, err := router.Select(cfg, "ail-compound", []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]}]}`), "")
	if err == nil {
		t.Fatal("expected no eligible backend")
	}
	if e, ok := err.(NoEligibleBackendError); !ok || e.Code() != "no_eligible_backend" {
		t.Fatalf("wrong error %#v", err)
	}
}

func TestRewriteModelField(t *testing.T) {
	got := string(RewriteModelField([]byte(`{"model":"deepseek-v4-pro","choices":[]}`), "ail-compound"))
	if got != `{"model":"ail-compound","choices":[]}` {
		t.Fatalf("json rewrite: %s", got)
	}
	got = string(RewriteModelField([]byte("data: {\"model\":\"MiniMax-M2.7\"}\n\ndata: [DONE]\n\n"), "ail-compound"))
	if got != "data: {\"model\":\"ail-compound\"}\n\ndata: [DONE]\n\n" {
		t.Fatalf("sse rewrite: %q", got)
	}
}

func TestSmoothWeightedRoundRobinSequences(t *testing.T) {
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)

	equalRouter := NewRouterWithStatePath(filepath.Join(t.TempDir(), "equal.json"))
	gotEqual := selectIDs(t, equalRouter, weightedTextConfig(1, 1), 6, body)
	wantEqual := []string{"backend-a", "backend-b", "backend-a", "backend-b", "backend-a", "backend-b"}
	if !reflect.DeepEqual(gotEqual, wantEqual) {
		t.Fatalf("equal SWRR sequence got %v want %v", gotEqual, wantEqual)
	}

	weightedRouter := NewRouterWithStatePath(filepath.Join(t.TempDir(), "weighted.json"))
	gotWeighted := selectIDs(t, weightedRouter, weightedTextConfig(3, 2), 10, body)
	wantWeighted := []string{"backend-a", "backend-b", "backend-a", "backend-b", "backend-a", "backend-a", "backend-b", "backend-a", "backend-b", "backend-a"}
	if !reflect.DeepEqual(gotWeighted, wantWeighted) {
		t.Fatalf("3/2 SWRR sequence got %v want %v", gotWeighted, wantWeighted)
	}
	counts := countIDs(gotWeighted)
	if counts["backend-a"] != 6 || counts["backend-b"] != 4 {
		t.Fatalf("expected 6/4 over 10 requests, got %#v", counts)
	}
}

func TestWeightedLoadBalancingDurable(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
	cfg := weightedTextConfig(3, 2)
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)

	router := NewRouterWithStatePath(statePath)
	choices := selectIDs(t, router, cfg, 5, body)
	if counts := countIDs(choices); counts["backend-a"] != 3 || counts["backend-b"] != 2 {
		t.Fatalf("expected 3 backend-a and 2 backend-b, got: %#v", counts)
	}

	router2 := NewRouterWithStatePath(statePath)
	route, err := router2.Select(cfg, "ail-compound", body, "")
	if err != nil {
		t.Fatal(err)
	}
	if route.MemberID != "backend-a" {
		t.Fatalf("expected persisted SWRR to resume at backend-a, got %s", route.MemberID)
	}

	pool := poolFromDisk(t, statePath, "ail-compound|chat_text")
	if pool.Counts["backend-a"] != 4 || pool.Counts["backend-b"] != 2 {
		t.Fatalf("expected persisted counts 4/2, got %#v", pool.Counts)
	}
	if pool.ConfigHash == "" {
		t.Fatal("expected config hash on persisted pool")
	}
}

func TestDefaultAndZeroWeightsAreBalanced(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
	cfg := weightedTextConfig(0, 0)
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	router := NewRouterWithStatePath(statePath)

	ids := selectIDs(t, router, cfg, 10, body)
	counts := countIDs(ids)
	if counts["backend-a"] != 5 || counts["backend-b"] != 5 {
		t.Fatalf("expected default equal weights to route 5/5, got %#v", counts)
	}
}

func TestCapabilityCountersAreSeparated(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
	router := NewRouterWithStatePath(statePath)
	cfg := testConfig()

	textBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	imageBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"text","text":"color?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`)

	if _, err := router.Select(cfg, "ail-compound", textBody, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := router.Select(cfg, "ail-compound", imageBody, ""); err != nil {
		t.Fatal(err)
	}

	snap := router.Snapshot()
	textPool := snap.Pools["ail-compound|chat_text"]
	imagePool := snap.Pools["ail-compound|chat_image_understanding"]
	if textPool.Counts["text-a"] != 1 {
		t.Fatalf("expected text counter on chat_text key, got %#v", textPool.Counts)
	}
	if imagePool.Counts["vision-d"] != 1 {
		t.Fatalf("expected image counter on chat_image_understanding key, got %#v", imagePool.Counts)
	}
	if textPool.ConfigHash == imagePool.ConfigHash {
		t.Fatalf("expected capability-separated config hashes, got %q", textPool.ConfigHash)
	}
}

func TestTextOnlyMemberNeverSelectedForToolHistory(t *testing.T) {
	cfg := &config.SDKConfig{VirtualModels: map[string]config.VirtualModelConfig{
		"ail-compound": {
			Expose:   true,
			Strategy: "weighted-round-robin",
			Members: []config.VirtualModelMemberConfig{
				{ID: "agentic", Provider: "provider-agentic", Model: "native-agentic", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat", "chat_text_tools", "chat_multiturn_tools"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true, ToolHistoryReplay: true}},
				{ID: "text-only", Provider: "provider-text", Model: "native-text", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat", "chat_text_tools"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
			},
		},
	}}
	router := NewRouterWithStatePath(filepath.Join(t.TempDir(), "virtual_models_state.json"))
	toolHistoryBody := []byte(`{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"}]}`)
	for i := 0; i < 5; i++ {
		route, err := router.Select(cfg, "ail-compound", toolHistoryBody, "")
		if err != nil {
			t.Fatal(err)
		}
		if route.MemberID != "agentic" {
			t.Fatalf("tool history routed to unsafe member: %#v", route)
		}
	}

	plainTextBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		route, err := router.Select(cfg, "ail-compound", plainTextBody, "")
		if err != nil {
			t.Fatal(err)
		}
		seen[route.MemberID] = true
	}
	if !seen["agentic"] || !seen["text-only"] {
		t.Fatalf("plain text should still use both text-capable members, got %#v", seen)
	}
}

func TestConfigHashChangeResetsPoolState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	router := NewRouterWithStatePath(statePath)
	if _, err := router.Select(weightedTextConfig(1, 1), "ail-compound", body, ""); err != nil {
		t.Fatal(err)
	}
	before := poolFromDisk(t, statePath, "ail-compound|chat_text")
	if before.Counts["backend-a"]+before.Counts["backend-b"] != 1 {
		t.Fatalf("expected first config to have one count, got %#v", before.Counts)
	}

	if _, err := router.Select(weightedTextConfig(3, 2), "ail-compound", body, ""); err != nil {
		t.Fatal(err)
	}
	after := poolFromDisk(t, statePath, "ail-compound|chat_text")
	if after.ConfigHash == before.ConfigHash {
		t.Fatal("expected config hash to change after weight change")
	}
	if after.Counts["backend-a"]+after.Counts["backend-b"] != 1 {
		t.Fatalf("expected pool counts reset after config change, got %#v", after.Counts)
	}
}

func TestOldMalformedAndUnwritableStateDoNotBreakRouting(t *testing.T) {
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	cfg := weightedTextConfig(1, 1)

	t.Run("old cursor state", func(t *testing.T) {
		statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
		if err := os.WriteFile(statePath, []byte(`{"cursors":{"ail-compound|chat_text":1},"counts":{"ail-compound|chat_text|backend-a":99}}`), 0600); err != nil {
			t.Fatal(err)
		}
		router := NewRouterWithStatePath(statePath)
		if _, err := router.Select(cfg, "ail-compound", body, ""); err != nil {
			t.Fatal(err)
		}
		pool := poolFromDisk(t, statePath, "ail-compound|chat_text")
		if pool.Counts["backend-a"]+pool.Counts["backend-b"] != 1 {
			t.Fatalf("old state should reset safely, got %#v", pool.Counts)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
		if err := os.WriteFile(statePath, []byte(`{not-json`), 0600); err != nil {
			t.Fatal(err)
		}
		router := NewRouterWithStatePath(statePath)
		if _, err := router.Select(cfg, "ail-compound", body, ""); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unwritable path", func(t *testing.T) {
		tempDir := t.TempDir()
		parentFile := filepath.Join(tempDir, "not-a-directory")
		if err := os.WriteFile(parentFile, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		router := NewRouterWithStatePath(filepath.Join(parentFile, "virtual_models_state.json"))
		if _, err := router.Select(cfg, "ail-compound", body, ""); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSelectExcludingDoesNotMutatePrimaryPoolState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
	cfg := weightedTextConfig(1, 1)
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	router := NewRouterWithStatePath(statePath)
	first, err := router.Select(cfg, "ail-compound", body, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.MemberID != "backend-a" {
		t.Fatalf("expected first primary selection backend-a, got %s", first.MemberID)
	}
	before := router.Snapshot().Pools["ail-compound|chat_text"].clone()

	fallback, err := router.SelectExcluding(cfg, "ail-compound", body, "", map[string]struct{}{"backend-a": {}})
	if err != nil {
		t.Fatal(err)
	}
	if fallback.MemberID != "backend-b" {
		t.Fatalf("expected fallback to backend-b, got %s", fallback.MemberID)
	}
	after := router.Snapshot().Pools["ail-compound|chat_text"]
	if !reflect.DeepEqual(before.Counts, after.Counts) || !reflect.DeepEqual(before.Current, after.Current) {
		t.Fatalf("fallback mutated primary state: before=%#v after=%#v", before, after)
	}
}

func TestWeightedLoadBalancingConcurrency(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "virtual_models_state.json")
	cfg := weightedTextConfig(1, 1)
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)

	router := NewRouterWithStatePath(statePath)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if _, err := router.Select(cfg, "ail-compound", body, ""); err != nil {
					t.Errorf("Select failed: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	pool := poolFromDisk(t, statePath, "ail-compound|chat_text")
	totalCounts := pool.Counts["backend-a"] + pool.Counts["backend-b"]
	if totalCounts != 100 {
		t.Fatalf("expected total routed count to be 100, got %d", totalCounts)
	}
	if pool.Counts["backend-a"] != 50 || pool.Counts["backend-b"] != 50 {
		t.Fatalf("expected equal weights to stay balanced under concurrency, got %#v", pool.Counts)
	}
}

func TestDetectRequirementsMixedMediaToolHistoryPreservesReplayFlags(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantClass  string
		wantImage  bool
		wantAudio  bool
		wantTools  bool
		wantReplay bool
	}{
		{
			name:       "chat image plus tool history",
			body:       `{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"},{"role":"user","content":[{"type":"text","text":"inspect"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`,
			wantClass:  ClassChatImageUnderstanding,
			wantImage:  true,
			wantTools:  true,
			wantReplay: true,
		},
		{
			name:       "chat audio plus tool history",
			body:       `{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"},{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"abc","format":"mp3"}}]}]}`,
			wantClass:  ClassChatAudioUnderstanding,
			wantAudio:  true,
			wantTools:  true,
			wantReplay: true,
		},
		{
			name:       "responses image plus function output",
			body:       `{"model":"ail-compound","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"},{"role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`,
			wantClass:  ClassChatImageUnderstanding,
			wantImage:  true,
			wantTools:  true,
			wantReplay: true,
		},
		{
			name:       "responses audio plus function output",
			body:       `{"model":"ail-compound","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"},{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"abc","format":"mp3"}}]}]}`,
			wantClass:  ClassChatAudioUnderstanding,
			wantAudio:  true,
			wantTools:  true,
			wantReplay: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := DetectRequirements([]byte(tt.body), "")
			if req.Class != tt.wantClass {
				t.Fatalf("class got %s want %s", req.Class, tt.wantClass)
			}
			if req.InputImage != tt.wantImage || req.InputAudio != tt.wantAudio || req.NeedsTools != tt.wantTools || req.HasToolHistory != tt.wantReplay {
				t.Fatalf("flags got image=%v audio=%v tools=%v replay=%v", req.InputImage, req.InputAudio, req.NeedsTools, req.HasToolHistory)
			}
		})
	}
}

func TestMediaToolHistoryRequiresReplaySafeMediaMember(t *testing.T) {
	cfg := &config.SDKConfig{VirtualModels: map[string]config.VirtualModelConfig{
		"ail-compound": {
			Expose:   true,
			Strategy: "weighted-round-robin",
			Members: []config.VirtualModelMemberConfig{
				{ID: "unsafe-vision", Provider: "provider-vision-unsafe", Model: "native-vision-unsafe", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}, Tools: true}},
				{ID: "safe-vision", Provider: "provider-vision-safe", Model: "native-vision-safe", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}, Tools: true, ToolHistoryReplay: true}},
			},
		},
	}}
	router := NewRouterWithStatePath(filepath.Join(t.TempDir(), "virtual_models_state.json"))
	mixedBody := []byte(`{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`)
	for i := 0; i < 5; i++ {
		route, err := router.Select(cfg, "ail-compound", mixedBody, "")
		if err != nil {
			t.Fatal(err)
		}
		if route.MemberID != "safe-vision" {
			t.Fatalf("mixed media/tool-history routed to unsafe member: %#v", route)
		}
	}

	plainImageBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`)
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		route, err := router.Select(cfg, "ail-compound", plainImageBody, "")
		if err != nil {
			t.Fatal(err)
		}
		seen[route.MemberID] = true
	}
	if !seen["safe-vision"] || !seen["unsafe-vision"] {
		t.Fatalf("plain image requests should still balance across both vision members, got %#v", seen)
	}

	snap := router.Snapshot()
	if _, ok := snap.Pools["ail-compound|chat_image_understanding|tools|tool_history"]; !ok {
		t.Fatalf("missing tool-history media pool key in %#v", snap.Pools)
	}
	if _, ok := snap.Pools["ail-compound|chat_image_understanding"]; !ok {
		t.Fatalf("missing plain media pool key in %#v", snap.Pools)
	}
}

func TestDetectRequirementsMixedImageAudioRequiresBothMediaFlags(t *testing.T) {
	req := DetectRequirements([]byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}},{"type":"input_audio","input_audio":{"data":"abc","format":"mp3"}}]}]}`), "")
	if req.Class != ClassChatMultimodalUnderstanding {
		t.Fatalf("class got %s want %s", req.Class, ClassChatMultimodalUnderstanding)
	}
	if !req.InputImage || !req.InputAudio {
		t.Fatalf("expected both media flags, got image=%v audio=%v", req.InputImage, req.InputAudio)
	}
}

func TestMixedImageAudioRequiresBackendWithBothMediaCapabilities(t *testing.T) {
	cfg := &config.SDKConfig{VirtualModels: map[string]config.VirtualModelConfig{
		"ail-compound": {
			Expose:   true,
			Strategy: "weighted-round-robin",
			Members: []config.VirtualModelMemberConfig{
				{ID: "vision-only", Provider: "provider-vision", Model: "native-vision", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}}},
				{ID: "audio-only", Provider: "provider-audio", Model: "native-audio", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_audio_understanding"}, Input: []string{"text", "audio"}, Output: []string{"text"}}},
				{ID: "both-media", Provider: "provider-multimodal", Model: "native-multimodal", Weight: 1, Enabled: boolPtr(true), Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding", "chat_audio_understanding"}, Input: []string{"text", "image", "audio"}, Output: []string{"text"}}},
			},
		},
	}}
	router := NewRouterWithStatePath(filepath.Join(t.TempDir(), "virtual_models_state.json"))
	mixedBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}},{"type":"input_audio","input_audio":{"data":"abc","format":"mp3"}}]}]}`)
	for i := 0; i < 3; i++ {
		route, err := router.Select(cfg, "ail-compound", mixedBody, "")
		if err != nil {
			t.Fatal(err)
		}
		if route.MemberID != "both-media" {
			t.Fatalf("mixed image+audio routed to media-incomplete member: %#v", route)
		}
	}

	pool := cfg.VirtualModels["ail-compound"]
	pool.Members = pool.Members[:2]
	cfg.VirtualModels["ail-compound"] = pool
	if _, err := NewRouterWithStatePath(filepath.Join(t.TempDir(), "state.json")).Select(cfg, "ail-compound", mixedBody, ""); err == nil {
		t.Fatal("expected no eligible backend when no member supports both image and audio")
	}
}
