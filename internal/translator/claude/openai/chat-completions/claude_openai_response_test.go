package chat_completions

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// Anthropic emits tool-call argument fragments in `input_json_delta` events under the
// field name `partial_json`. Regression guard: these fragments must survive translation
// to OpenAI `tool_calls[].function.arguments` on both the streaming and non-streaming
// paths. Reading the wrong field (`input_json`) silently drops every tool call.

// streamEvents is a realistic Anthropic SSE sequence for a single tool call whose
// arguments {"city":"Paris"} arrive split across two partial_json fragments.
var streamEvents = []string{
	`data: {"type":"message_start","message":{"id":"msg_test","model":"claude","usage":{"input_tokens":5}}}`,
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather"}}`,
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}`,
	`data: {"type":"content_block_stop","index":0}`,
	`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
}

func TestConvertClaudeResponseToOpenAI_StreamingToolCallArgs(t *testing.T) {
	var param any
	var gotArgs strings.Builder

	for _, ev := range streamEvents {
		out := ConvertClaudeResponseToOpenAI(context.Background(), "claude", nil, nil, []byte(ev), &param)
		for _, chunk := range out {
			if a := gjson.Get(chunk, "choices.0.delta.tool_calls.0.function.arguments"); a.Exists() {
				gotArgs.WriteString(a.String())
			}
		}
	}

	if got := gotArgs.String(); got != `{"city":"Paris"}` {
		t.Fatalf("streamed tool-call arguments not reassembled: got %q, want %q", got, `{"city":"Paris"}`)
	}
}

func TestConvertClaudeResponseToOpenAINonStream_ToolCallArgs(t *testing.T) {
	var param any
	body := strings.Join(streamEvents, "\n")

	out := ConvertClaudeResponseToOpenAINonStream(context.Background(), "claude", nil, nil, []byte(body), &param)

	args := gjson.Get(out, "choices.0.message.tool_calls.0.function.arguments").String()
	if args != `{"city":"Paris"}` {
		t.Fatalf("non-stream tool-call arguments dropped: got %q, want %q", args, `{"city":"Paris"}`)
	}
	if fr := gjson.Get(out, "choices.0.finish_reason").String(); fr != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", fr)
	}
}
