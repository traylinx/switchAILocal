// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bytes"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// emptyTextPlaceholder replaces an empty-text content array when there is
// no other content left after stripping. The text is intentionally
// self-describing so a captured trace surfaces the gateway mutation.
const emptyTextPlaceholder = "(empty content elided by gateway)"

// stripEmptyTextContent removes content array elements of the shape
// {"type":"text","text":""} from every messages[].content[].
//
// Why: OpenClaw 2026.4.25 began emitting empty-text placeholder elements
// during history replay when an assistant turn errored before producing
// content. Both Moonshot/Kimi and MiniMax permanently 400 such requests
// with "text content is empty" / "invalid params". Once an empty-text
// element is persisted to a session jsonl, every subsequent turn replays
// the corrupt history and the entire conversation deadlocks — the
// 2026-04-27 Telegram outage on pod 02. Stripping the element at the
// gateway boundary makes the whole pipeline robust to any client that
// emits empty content, present or future.
//
// Idempotent: messages without an array content, without empty-text
// elements, or with non-empty trimmed text pass through untouched. If
// stripping leaves the array empty, a single placeholder text element is
// inserted so the message remains a valid OpenAI-shape chat-completions
// content block (every upstream provider rejects an empty content array
// the same way it rejects empty text).
//
// Pure transformation: no I/O, no config dependency, no provider
// awareness. Always applied. Empty text content is universally invalid
// in chat completions; there is no scenario where forwarding it is
// preferable to stripping it.
func stripEmptyTextContent(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}
	messages := gjson.GetBytes(payload, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return payload
	}

	out := payload
	stripped := 0
	rewritten := 0

	messages.ForEach(func(msgIdx, msg gjson.Result) bool {
		content := msg.Get("content")
		if !content.Exists() || !content.IsArray() {
			return true
		}

		elems := content.Array()
		kept := make([][]byte, 0, len(elems))
		droppedHere := 0
		for _, el := range elems {
			if el.Get("type").String() == "text" && el.Get("text").String() == "" {
				droppedHere++
				continue
			}
			kept = append(kept, []byte(el.Raw))
		}
		if droppedHere == 0 {
			return true
		}
		stripped += droppedHere
		rewritten++

		path := buildMessagePath(msgIdx.Int(), "content")
		var newOut []byte
		var err error
		if len(kept) == 0 {
			// Don't leave an empty content array — every upstream rejects
			// it. Replace with a single non-empty placeholder text block.
			placeholder := []byte(`[{"type":"text","text":"` + emptyTextPlaceholder + `"}]`)
			newOut, err = sjson.SetRawBytes(out, path, placeholder)
		} else {
			var buf bytes.Buffer
			buf.WriteByte('[')
			for i, raw := range kept {
				if i > 0 {
					buf.WriteByte(',')
				}
				buf.Write(raw)
			}
			buf.WriteByte(']')
			newOut, err = sjson.SetRawBytes(out, path, buf.Bytes())
		}
		if err != nil {
			log.Warnf("empty-text shim: sjson.SetRawBytes failed at %s: %v", path, err)
			return true
		}
		out = newOut
		return true
	})

	if stripped > 0 {
		log.Debugf("empty-text shim: stripped %d empty-text content element(s) across %d message(s)", stripped, rewritten)
	}
	return out
}
