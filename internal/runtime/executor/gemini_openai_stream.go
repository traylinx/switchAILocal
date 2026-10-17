package executor

import (
	"bytes"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Google's OpenAI-compatible SSE currently finishes tool turns with "stop".
// Keep per-request, per-choice state: SDKs must receive "tool_calls" for an
// observed tool turn, but truncation/safety reasons must never become success.
// See googleapis/python-genai#2868 and the OpenAI Chat Completions contract.
type geminiOpenAIStream struct {
	enabled bool
	tools   map[int64]bool
}

func newGeminiOpenAIStream(baseURL string) *geminiOpenAIStream {
	return &geminiOpenAIStream{enabled: isGeminiOpenAICompatEndpoint(baseURL), tools: make(map[int64]bool)}
}

func (n *geminiOpenAIStream) normalize(line []byte) []byte {
	if !n.enabled || !bytes.HasPrefix(line, []byte("data:")) {
		return line
	}
	body := bytes.TrimSpace(line[len("data:"):])
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return line
	}
	output := body
	changed := false
	for pos, choice := range gjson.GetBytes(body, "choices").Array() {
		index := choice.Get("index")
		// Bound state even if a malformed stream invents an unlimited index sequence.
		if index.Type != gjson.Number || index.Int() < 0 || index.Int() > 1024 || float64(index.Int()) != index.Float() {
			continue
		}
		key := index.Int()
		if calls := choice.Get("delta.tool_calls"); calls.IsArray() && len(calls.Array()) > 0 {
			n.tools[key] = true
		}
		if n.tools[key] && choice.Get("finish_reason").String() == "stop" {
			if updated, err := sjson.SetBytes(output, fmt.Sprintf("choices.%d.finish_reason", pos), "tool_calls"); err == nil {
				output = updated
				changed = true
			}
		}
	}
	if !changed {
		return line
	}
	return append([]byte("data: "), output...)
}
