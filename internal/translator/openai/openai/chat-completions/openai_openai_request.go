// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package openai provides request translation functionality for OpenAI to Gemini CLI API compatibility.
// It converts OpenAI Chat Completions requests into Gemini CLI compatible JSON using gjson/sjson only.
package chat_completions

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// sanitizeJSONString extracts the first complete JSON object from a string
// that might contain multiple concatenated JSON objects (e.g. {"a":1}{"b":2}).
// This fixes parsing errors caused by improperly merged tool calls in history.
func sanitizeJSONString(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return s
	}
	depth := 0
	inQuote := false
	escape := false
	for i, c := range s {
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					return s[:i+1]
				}
			}
		}
	}
	return s
}

// ConvertOpenAIRequestToOpenAI converts an OpenAI Chat Completions request (raw JSON)
// into a complete Gemini CLI request JSON. All JSON construction uses sjson and lookups use gjson.
//
// Parameters:
//   - modelName: The name of the model to use for the request
//   - rawJSON: The raw JSON request data from the OpenAI API
//   - stream: A boolean indicating if the request is for a streaming response (unused in current implementation)
//
// Returns:
//   - []byte: The transformed request data in Gemini CLI API format
func ConvertOpenAIRequestToOpenAI(modelName string, inputRawJSON []byte, _ bool) []byte {
	// Update the "model" field in the JSON payload with the provided modelName
	// The sjson.SetBytes function returns a new byte slice with the updated JSON.
	updatedJSON, err := sjson.SetBytes(inputRawJSON, "model", modelName)
	if err != nil {
		// If there's an error, return the original JSON or handle the error appropriately.
		// For now, we'll return the original, but in a real scenario, logging or a more robust error
		// handling mechanism would be needed.
		return bytes.Clone(inputRawJSON)
	}
	// Sanitize corrupted tool_calls in the history
	if msgs := gjson.GetBytes(updatedJSON, "messages"); msgs.Exists() && msgs.IsArray() {
		msgs.ForEach(func(i, msg gjson.Result) bool {
			if tcs := msg.Get("tool_calls"); tcs.Exists() && tcs.IsArray() {
				tcs.ForEach(func(j, tc gjson.Result) bool {
					if args := tc.Get("function.arguments"); args.Exists() {
						s := sanitizeJSONString(args.String())
						if s != args.String() {
							updatedJSON, _ = sjson.SetBytes(updatedJSON, fmt.Sprintf("messages.%d.tool_calls.%d.function.arguments", i.Int(), j.Int()), s)
						}
					}
					return true
				})
			}
			if fc := msg.Get("function_call.arguments"); fc.Exists() {
				s := sanitizeJSONString(fc.String())
				if s != fc.String() {
					updatedJSON, _ = sjson.SetBytes(updatedJSON, fmt.Sprintf("messages.%d.function_call.arguments", i.Int()), s)
				}
			}
			return true
		})
	}

	return updatedJSON
}
