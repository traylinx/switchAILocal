// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// reasoningContentPlaceholder is the non-empty string written into
// assistant.reasoning_content when the field is absent or empty on a
// tool-calling assistant message during history replay. Kimi K2.6
// rejects empty strings as "missing", so the placeholder must be a
// non-empty string. The text is intentionally self-describing so any
// human reading a captured trace immediately understands the mutation.
const reasoningContentPlaceholder = "(reasoning_content elided by gateway during history replay)"

// applyKimiHistoryShim walks messages[] in an OpenAI-shaped chat
// completions body and injects a placeholder reasoning_content on
// every assistant message that has a non-empty tool_calls array but
// is missing or has an empty reasoning_content field.
//
// Why this exists: Kimi K2.6 served from api.kimi.com/coding/v1 has
// thinking-mode default-on. Its assistant responses include a
// reasoning_content string. On subsequent turns where that assistant
// message has tool_calls, Kimi REQUIRES the reasoning_content echoed
// back — missing or empty string returns
// 400 "thinking is enabled but reasoning_content is missing in
// assistant tool call message". OpenClaw and most OpenAI-compat
// agents strip provider-specific fields during history replay, so
// without this shim a Kimi candidate inside a heterogeneous failover
// alias (e.g. ail-compound = MiniMax + Kimi) breaks every multi-turn
// agentic loop. DeepSeek-V4-Pro has the same failure shape.
//
// Stateless: no cache, no GC, idempotent across multi-instance
// deployments. The mutator is a pure transformation on the request
// bytes — safe to call on any OpenAI-compat body. Providers that
// don't require reasoning_content (MiniMax, OpenAI) ignore the
// field; providers that do (Kimi, DeepSeek) accept the placeholder.
//
// Idempotent: a message that already carries a non-empty
// reasoning_content is left untouched. A message with no tool_calls
// is left untouched. A non-array messages field is left untouched.
func applyKimiHistoryShim(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}
	messages := gjson.GetBytes(payload, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return payload
	}

	out := payload
	mutated := 0

	messages.ForEach(func(msgIdx, msg gjson.Result) bool {
		if msg.Get("role").String() != "assistant" {
			return true
		}
		toolCalls := msg.Get("tool_calls")
		if !toolCalls.Exists() || !toolCalls.IsArray() || len(toolCalls.Array()) == 0 {
			return true
		}
		rc := msg.Get("reasoning_content")
		// Treat string-typed reasoning_content with non-empty trimmed
		// value as "already valid"; nil / missing / empty / non-string
		// types fall through to the placeholder injection.
		if rc.Exists() && rc.Type == gjson.String && rc.String() != "" {
			return true
		}
		path := buildMessagePath(msgIdx.Int(), "reasoning_content")
		newOut, err := sjson.SetBytes(out, path, reasoningContentPlaceholder)
		if err != nil {
			// sjson errors here are pathological (malformed JSON,
			// path build mismatch) — skip the mutation rather than
			// corrupt the body. Loud log so a fleet diff surfaces it.
			log.Warnf("kimi history shim: sjson.SetBytes failed at %s: %v", path, err)
			return true
		}
		out = newOut
		mutated++
		return true
	})

	if mutated > 0 {
		log.Debugf("kimi history shim: injected reasoning_content placeholder on %d assistant tool-call messages", mutated)
	}
	return out
}

// buildMessagePath returns the sjson path for a top-level field on
// messages[idx]. Kept tiny and dependency-free so the test file can
// assert on path construction directly.
func buildMessagePath(idx int64, field string) string {
	return "messages." + itoa(idx) + "." + field
}
