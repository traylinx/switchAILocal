package virtualmodels

import (
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
				{ID: "minimax", Provider: "minimax", Model: "MiniMax-M2.7", Enabled: boolPtr(true), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat", "chat_text_tools"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
				{ID: "deepseek", Provider: "deepseek", Model: "deepseek-v4-pro", Enabled: boolPtr(true), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
				{ID: "kimi", Provider: "moonshot", Model: "kimi-k2.6", Enabled: boolPtr(false), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}},
				{ID: "qwen-vl", Provider: "alibaba", Model: "qwen3-vl-plus", Enabled: boolPtr(true), Weight: 1, Capabilities: config.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}}},
			},
		},
	}}
}

func TestDetectRequirementsToolHistory(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"text", `{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`, ClassChatText},
		{"one shot tools", `{"model":"ail-compound","tools":[{"type":"function","function":{"name":"x"}}],"messages":[{"role":"user","content":"hi"}]}`, ClassChatTextTools},
		{"assistant tool calls", `{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"1","type":"function","function":{"name":"x","arguments":"{}"}}]}]}`, ClassChatMultiturnTools},
		{"tool role", `{"model":"ail-compound","messages":[{"role":"tool","tool_call_id":"1","content":"ok"}]}`, ClassChatMultiturnTools},
		{"tool call id", `{"model":"ail-compound","messages":[{"role":"assistant","tool_call_id":"1","content":"ok"}]}`, ClassChatMultiturnTools},
	}
	for _, tt := range tests {
		got := DetectRequirements([]byte(tt.body), "").Class
		if got != tt.want {
			t.Fatalf("%s: got %s want %s", tt.name, got, tt.want)
		}
	}
}

func TestSelectFiltersDisabledAndMedia(t *testing.T) {
	router := NewRouter()
	cfg := testConfig()
	imageBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"text","text":"color?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`)
	route, err := router.Select(cfg, "ail-compound", imageBody, "")
	if err != nil {
		t.Fatal(err)
	}
	if route.Provider != "alibaba" || route.MemberID != "qwen-vl" {
		t.Fatalf("image routed to %#v", route)
	}

	textBody := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		route, err = router.Select(cfg, "ail-compound", textBody, "")
		if err != nil {
			t.Fatal(err)
		}
		if route.MemberID == "kimi" {
			t.Fatal("disabled kimi selected")
		}
		seen[route.MemberID] = true
	}
	if !seen["minimax"] || !seen["deepseek"] {
		t.Fatalf("text did not balance across minimax/deepseek: %#v", seen)
	}
}

func TestNoEligibleBackendForImageWhenNoMediaMember(t *testing.T) {
	cfg := testConfig()
	pool := cfg.VirtualModels["ail-compound"]
	pool.Members = pool.Members[:3]
	cfg.VirtualModels["ail-compound"] = pool
	_, err := NewRouter().Select(cfg, "ail-compound", []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]}]}`), "")
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
