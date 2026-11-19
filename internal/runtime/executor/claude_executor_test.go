package executor

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/config"
	_ "github.com/traylinx/switchAILocal/internal/translator/claude/openai/chat-completions"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	sdktranslator "github.com/traylinx/switchAILocal/sdk/translator"
)

func TestApplyClaudeHeaders_APIKeyDoesNotSpoofClaudeCode(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	auth := &switchailocalauth.Auth{
		Attributes: map[string]string{
			"api_key": "sk-ant-usr-test-key",
		},
	}

	applyClaudeHeaders(req, auth, "sk-ant-usr-test-key", false, nil)

	if got := req.Header.Get("x-api-key"); got != "sk-ant-usr-test-key" {
		t.Fatalf("x-api-key = %q, want api key", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
	if got := req.Header.Get("Anthropic-Beta"); got != "" {
		t.Fatalf("Anthropic-Beta = %q, want empty when caller sent no betas", got)
	}
	if got := req.Header.Get("User-Agent"); strings.Contains(got, "claude-cli") {
		t.Fatalf("User-Agent = %q, must not spoof claude-cli for API keys", got)
	}
	if got := req.Header.Get("X-App"); got != "" {
		t.Fatalf("X-App = %q, want empty for API keys", got)
	}
	if got := req.Header.Get("Anthropic-Version"); got != "2023-06-01" {
		t.Fatalf("Anthropic-Version = %q, want 2023-06-01", got)
	}
}

func TestApplyClaudeHeaders_OAuthKeepsClaudeCodeHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	auth := &switchailocalauth.Auth{
		Metadata: map[string]any{"access_token": "oauth-token"},
	}

	applyClaudeHeaders(req, auth, "oauth-token", true, nil)

	if got := req.Header.Get("Authorization"); got != "Bearer oauth-token" {
		t.Fatalf("Authorization = %q, want Bearer oauth-token", got)
	}
	if got := req.Header.Get("Anthropic-Beta"); !strings.Contains(got, "oauth-2025-04-20") {
		t.Fatalf("Anthropic-Beta = %q, want oauth beta for OAuth auth", got)
	}
	if got := req.Header.Get("User-Agent"); !strings.Contains(got, "claude-cli") {
		t.Fatalf("User-Agent = %q, want claude-cli for OAuth auth", got)
	}
}

func TestReadClaudeErrorBody_DecodesGzip(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(`{"type":"error","error":{"message":"plain json"}}`))
	_ = gw.Close()

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Encoding": []string{"gzip"}},
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
	}

	got := string(readClaudeErrorBody(resp))
	if !strings.Contains(got, "plain json") {
		t.Fatalf("decoded error body = %q, want readable JSON", got)
	}
}

func TestClaudeExecutor_OpenAIToClaudeExecuteAndStream(t *testing.T) {
	ssePayload := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"HELLO_OPENAI"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	var seenSystem bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "You are Claude Code") {
			seenSystem = true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(ssePayload))
	}))
	defer srv.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &switchailocalauth.Auth{
		Attributes: map[string]string{
			"api_key":  "sk-ant-usr-test",
			"base_url": srv.URL,
		},
	}
	origReq := []byte(`{"model":"claude-haiku-5-5","messages":[{"role":"user","content":"hi"}],"max_tokens":32}`)
	req := switchailocalexecutor.Request{Model: "claude-haiku-5-5", Payload: origReq}
	opts := switchailocalexecutor.Options{
		SourceFormat:    sdktranslator.FromString("openai"),
		OriginalRequest: origReq,
	}

	resp, err := exec.Execute(context.Background(), auth, req, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if seenSystem {
		t.Fatalf("API-key request must not inject Claude Code system prompt")
	}
	if got := gjson.GetBytes(resp.Payload, "object").String(); got != "chat.completion" {
		t.Fatalf("object = %q, want chat.completion; payload=%s", got, string(resp.Payload))
	}
	if got := gjson.GetBytes(resp.Payload, "choices.0.message.content").String(); got != "HELLO_OPENAI" {
		t.Fatalf("content = %q, want HELLO_OPENAI; payload=%s", got, string(resp.Payload))
	}

	streamCh, err := exec.ExecuteStream(context.Background(), auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}
	var joined strings.Builder
	for chunk := range streamCh {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		joined.Write(chunk.Payload)
	}
	if !strings.Contains(joined.String(), "HELLO_OPENAI") {
		t.Fatalf("stream output missing HELLO_OPENAI: %s", joined.String())
	}
}
