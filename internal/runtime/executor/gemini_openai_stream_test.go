package executor

import (
	"bytes"
	"testing"

	"github.com/tidwall/gjson"
)

const googleOpenAIBase = "https://generativelanguage.googleapis.com/v1beta/openai"

func TestGeminiOpenAIStreamToolFinishAcrossChunksAndChoices(t *testing.T) {
	n := newGeminiOpenAIStream(googleOpenAIBase)
	tool := []byte(`data: {"choices":[{"index":1,"delta":{"tool_calls":[{"index":0,"function":{"name":"echo","arguments":"{}"},"extra_content":{"google":{"thought_signature":"synthetic"}}}]},"finish_reason":null}]}`)
	if !bytes.Equal(n.normalize(tool), tool) {
		t.Fatal("tool delta or signature changed")
	}
	final := []byte(`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{},"finish_reason":"stop"}]}`)
	before := bytes.Clone(final)
	out := n.normalize(final)
	if gjson.GetBytes(out[6:], "choices.0.finish_reason").String() != "stop" || gjson.GetBytes(out[6:], "choices.1.finish_reason").String() != "tool_calls" {
		t.Fatalf("incorrect per-choice finish reasons: %s", out)
	}
	if !bytes.Equal(before, final) {
		t.Fatal("mutated caller buffer")
	}
	if !bytes.Equal(newGeminiOpenAIStream(googleOpenAIBase).normalize(final), final) {
		t.Fatal("state leaked across requests")
	}
}

func TestGeminiOpenAIStreamOnlyRewritesSuccessfulToolStop(t *testing.T) {
	for _, reason := range []string{"length", "content_filter", "tool_calls", "error", ""} {
		n := newGeminiOpenAIStream(googleOpenAIBase)
		tool := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0}]},"finish_reason":null}]}`)
		n.normalize(tool)
		line := []byte(`data: {"choices":[{"index":0,"delta":{},"finish_reason":"` + reason + `"}]}`)
		if !bytes.Equal(n.normalize(line), line) {
			t.Fatalf("changed non-success reason %s", reason)
		}
	}
	n := newGeminiOpenAIStream(googleOpenAIBase)
	same := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0}]},"finish_reason":"stop"}]}`)
	if gjson.GetBytes(n.normalize(same)[6:], "choices.0.finish_reason").String() != "tool_calls" {
		t.Fatal("same-frame tool finish not normalized")
	}
}

func TestGeminiOpenAIStreamUnrelatedEndpointsAndFramesUnchanged(t *testing.T) {
	for _, base := range []string{"https://api.deepseek.com/v1", "http://127.0.0.1:18080/v1", "https://generativelanguage.googleapis.com.attacker.invalid/v1beta/openai", "http://generativelanguage.googleapis.com/v1beta/openai", "https://user@generativelanguage.googleapis.com/v1beta/openai", "https://generativelanguage.googleapis.com/v1beta/openai?x=1"} {
		n := newGeminiOpenAIStream(base)
		line := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0}]},"finish_reason":"stop"}]}`)
		if !bytes.Equal(n.normalize(line), line) {
			t.Fatalf("unrelated endpoint changed: %s", base)
		}
	}
	n := newGeminiOpenAIStream(googleOpenAIBase)
	for _, line := range []string{"", ":keepalive", "data: [DONE]", "data: invalid", "data: []", `data: {"choices":[{"index":-1,"delta":{"tool_calls":[{}]},"finish_reason":"stop"}]}`, `data: {"choices":[{"index":1000000,"delta":{"tool_calls":[{}]},"finish_reason":"stop"}]}`} {
		if !bytes.Equal(n.normalize([]byte(line)), []byte(line)) {
			t.Fatalf("malformed/control frame changed: %s", line)
		}
	}
}
