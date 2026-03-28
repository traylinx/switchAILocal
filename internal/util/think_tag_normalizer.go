// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import "strings"

// ExtractThinkTags detects <think>...</think> tags embedded in a content string
// and separates the thinking text from the clean content.
//
// Many providers (MiniMax, DeepSeek R1, etc.) embed chain-of-thought reasoning
// inside <think> tags within the content field instead of using the OpenAI-standard
// reasoning_content field. This function normalizes that by splitting the content.
//
// Returns:
//   - thinking: the text inside <think>...</think> tags (may span multiple blocks)
//   - cleaned: the content with all <think>...</think> blocks removed
//   - found: true if at least one <think> tag was found
//
// Edge cases handled:
//   - No <think> tag: returns ("", original, false) — zero overhead path
//   - Unclosed <think> tag: treats everything after <think> as thinking
//   - Multiple <think> blocks: concatenates all thinking text
//   - Empty <think></think>: found=true but thinking="" 
//   - Whitespace around tags and in content is preserved internally but
//     leading/trailing whitespace on both outputs is trimmed
func ExtractThinkTags(content string) (thinking string, cleaned string, found bool) {
	const openTag = "<think>"
	const closeTag = "</think>"

	idx := strings.Index(content, openTag)
	if idx == -1 {
		return "", content, false
	}

	var thinkBuf strings.Builder
	var cleanBuf strings.Builder

	for {
		idx = strings.Index(content, openTag)
		if idx == -1 {
			// No more <think> tags — append remaining content
			cleanBuf.WriteString(content)
			break
		}

		// Append content before <think>
		cleanBuf.WriteString(content[:idx])
		content = content[idx+len(openTag):]

		// Find closing tag
		endIdx := strings.Index(content, closeTag)
		if endIdx == -1 {
			// Unclosed <think> — treat rest as thinking
			thinkBuf.WriteString(content)
			content = ""
			break
		}

		// Extract thinking text between tags
		thinkBuf.WriteString(content[:endIdx])
		content = content[endIdx+len(closeTag):]
	}

	thinking = strings.TrimSpace(thinkBuf.String())
	cleaned = strings.TrimSpace(cleanBuf.String())
	return thinking, cleaned, true
}
