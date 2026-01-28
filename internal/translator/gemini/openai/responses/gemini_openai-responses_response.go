// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package responses

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"
	"github.com/tidwall/gjson"
)

type geminiToResponsesState struct {
	Seq        int
	ResponseID string
	CreatedAt  int64
	Started    bool

	// message aggregation
	MsgOpened    bool
	MsgIndex     int
	CurrentMsgID string
	TextBuf      strings.Builder

	// reasoning aggregation
	ReasoningOpened bool
	ReasoningIndex  int
	ReasoningItemID string
	ReasoningBuf    strings.Builder
	ReasoningClosed bool

	// function call aggregation (keyed by output_index)
	NextIndex   int
	FuncArgsBuf map[int]*strings.Builder
	FuncNames   map[int]string
	FuncCallIDs map[int]string
}

// responseIDCounter provides a process-wide unique counter for synthesized response identifiers.
var responseIDCounter uint64

// funcCallIDCounter provides a process-wide unique counter for function call identifiers.
var funcCallIDCounter uint64

func emitEvent(event string, payload string) string {
	return fmt.Sprintf("event: %s\ndata: %s", event, payload)
}

func marshalEvent(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ConvertGeminiResponseToOpenAIResponses converts Gemini SSE chunks into OpenAI Responses SSE events.
func ConvertGeminiResponseToOpenAIResponses(_ context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []string {
	if *param == nil {
		*param = &geminiToResponsesState{
			FuncArgsBuf: make(map[int]*strings.Builder),
			FuncNames:   make(map[int]string),
			FuncCallIDs: make(map[int]string),
		}
	}
	st := (*param).(*geminiToResponsesState)

	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	root := gjson.ParseBytes(rawJSON)
	if !root.Exists() {
		return []string{}
	}

	var out []string
	nextSeq := func() int { st.Seq++; return st.Seq }

	// Helper to finalize reasoning summary events in correct order.
	finalizeReasoning := func() {
		if !st.ReasoningOpened || st.ReasoningClosed {
			return
		}
		full := st.ReasoningBuf.String()

		// response.reasoning_summary_text.done
		textDone := ResponseReasoningSummaryTextDone{
			Type:           "response.reasoning_summary_text.done",
			SequenceNumber: nextSeq(),
			ItemID:         st.ReasoningItemID,
			OutputIndex:    st.ReasoningIndex,
			SummaryIndex:   0,
			Text:           full,
		}
		out = append(out, emitEvent(textDone.Type, marshalEvent(textDone)))

		// response.reasoning_summary_part.done
		partDone := ResponseReasoningSummaryPartDone{
			Type:           "response.reasoning_summary_part.done",
			SequenceNumber: nextSeq(),
			ItemID:         st.ReasoningItemID,
			OutputIndex:    st.ReasoningIndex,
			SummaryIndex:   0,
			Part: SummaryPart{
				Type: "summary_text",
				Text: full,
			},
		}
		out = append(out, emitEvent(partDone.Type, marshalEvent(partDone)))

		// response.output_item.done
		itemDone := ResponseOutputItemDone{
			Type:           "response.output_item.done",
			SequenceNumber: nextSeq(),
			OutputIndex:    st.ReasoningIndex,
			Item: OutputItem{
				ID:   st.ReasoningItemID,
				Type: "reasoning",
				Summary: []SummaryPart{{
					Type: "summary_text",
					Text: full,
				}},
			},
		}
		out = append(out, emitEvent(itemDone.Type, marshalEvent(itemDone)))

		st.ReasoningClosed = true
	}

	// Initialize per-response fields and emit created/in_progress once
	if !st.Started {
		if v := root.Get("responseId"); v.Exists() {
			st.ResponseID = v.String()
		}
		if v := root.Get("createTime"); v.Exists() {
			if t, err := time.Parse(time.RFC3339Nano, v.String()); err == nil {
				st.CreatedAt = t.Unix()
			}
		}
		if st.CreatedAt == 0 {
			st.CreatedAt = time.Now().Unix()
		}

		created := ResponseCreated{
			Type:           "response.created",
			SequenceNumber: nextSeq(),
			Response: ResponseInfo{
				ID:         st.ResponseID,
				Object:     "response",
				CreatedAt:  st.CreatedAt,
				Status:     "in_progress",
				Background: false,
				Output:     &[]any{},
			},
		}
		out = append(out, emitEvent(created.Type, marshalEvent(created)))

		inprog := ResponseInProgress{
			Type:           "response.in_progress",
			SequenceNumber: nextSeq(),
			Response: ResponseInfo{
				ID:        st.ResponseID,
				Object:    "response",
				CreatedAt: st.CreatedAt,
				Status:    "in_progress",
			},
		}
		out = append(out, emitEvent(inprog.Type, marshalEvent(inprog)))

		st.Started = true
		st.NextIndex = 0
	}

	// Handle parts (text/thought/functionCall)
	if parts := root.Get("candidates.0.content.parts"); parts.Exists() && parts.IsArray() {
		parts.ForEach(func(_, part gjson.Result) bool {
			// Reasoning text
			if part.Get("thought").Bool() {
				if st.ReasoningClosed {
					return true
				}
				if !st.ReasoningOpened {
					st.ReasoningOpened = true
					st.ReasoningIndex = st.NextIndex
					st.NextIndex++
					st.ReasoningItemID = fmt.Sprintf("rs_%s_%d", st.ResponseID, st.ReasoningIndex)

					item := OutputItemAdded{
						Type:           "response.output_item.added",
						SequenceNumber: nextSeq(),
						OutputIndex:    st.ReasoningIndex,
						Item: OutputItem{
							ID:      st.ReasoningItemID,
							Type:    "reasoning",
							Status:  "in_progress",
							Summary: []SummaryPart{},
						},
					}
					out = append(out, emitEvent(item.Type, marshalEvent(item)))

					partAdded := ReasoningSummaryPartAdded{
						Type:           "response.reasoning_summary_part.added",
						SequenceNumber: nextSeq(),
						ItemID:         st.ReasoningItemID,
						OutputIndex:    st.ReasoningIndex,
						SummaryIndex:   0,
						Part: SummaryPart{
							Type: "summary_text",
							Text: "",
						},
					}
					out = append(out, emitEvent(partAdded.Type, marshalEvent(partAdded)))
				}
				if t := part.Get("text"); t.Exists() && t.String() != "" {
					st.ReasoningBuf.WriteString(t.String())

					msg := ReasoningSummaryTextDelta{
						Type:           "response.reasoning_summary_text.delta",
						SequenceNumber: nextSeq(),
						ItemID:         st.ReasoningItemID,
						OutputIndex:    st.ReasoningIndex,
						SummaryIndex:   0,
						Delta:          t.String(),
					}
					out = append(out, emitEvent(msg.Type, marshalEvent(msg)))
				}
				return true
			}

			// Assistant visible text
			if t := part.Get("text"); t.Exists() && t.String() != "" {
				finalizeReasoning()
				if !st.MsgOpened {
					st.MsgOpened = true
					st.MsgIndex = st.NextIndex
					st.NextIndex++
					st.CurrentMsgID = fmt.Sprintf("msg_%s_0", st.ResponseID)

					item := OutputItemAdded{
						Type:           "response.output_item.added",
						SequenceNumber: nextSeq(),
						OutputIndex:    st.MsgIndex,
						Item: OutputItem{
							ID:      st.CurrentMsgID,
							Type:    "message",
							Status:  "in_progress",
							Content: []ContentPart{},
							Role:    "assistant",
						},
					}
					out = append(out, emitEvent(item.Type, marshalEvent(item)))

					partAdded := ContentPartAdded{
						Type:           "response.content_part.added",
						SequenceNumber: nextSeq(),
						ItemID:         st.CurrentMsgID,
						OutputIndex:    st.MsgIndex,
						ContentIndex:   0,
						Part: ContentPart{
							Type:        "output_text",
							Annotations: []any{},
							Logprobs:    []any{},
							Text:        "",
						},
					}
					out = append(out, emitEvent(partAdded.Type, marshalEvent(partAdded)))
				}
				st.TextBuf.WriteString(t.String())

				msg := OutputTextDelta{
					Type:           "response.output_text.delta",
					SequenceNumber: nextSeq(),
					ItemID:         st.CurrentMsgID,
					OutputIndex:    st.MsgIndex,
					ContentIndex:   0,
					Delta:          t.String(),
					Logprobs:       []any{},
				}
				out = append(out, emitEvent(msg.Type, marshalEvent(msg)))
				return true
			}

			// Function call
			if fc := part.Get("functionCall"); fc.Exists() {
				finalizeReasoning()
				name := fc.Get("name").String()
				idx := st.NextIndex
				st.NextIndex++
				if st.FuncArgsBuf[idx] == nil {
					st.FuncArgsBuf[idx] = &strings.Builder{}
				}
				if st.FuncCallIDs[idx] == "" {
					st.FuncCallIDs[idx] = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&funcCallIDCounter, 1))
				}
				st.FuncNames[idx] = name

				item := OutputItemAdded{
					Type:           "response.output_item.added",
					SequenceNumber: nextSeq(),
					OutputIndex:    idx,
					Item: OutputItem{
						ID:        fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]),
						Type:      "function_call",
						Status:    "in_progress",
						Arguments: "",
						CallID:    st.FuncCallIDs[idx],
						Name:      name,
					},
				}
				out = append(out, emitEvent(item.Type, marshalEvent(item)))

				if args := fc.Get("args"); args.Exists() {
					argsJSON := args.Raw
					st.FuncArgsBuf[idx].WriteString(argsJSON)

					ad := FunctionCallArgumentsDelta{
						Type:           "response.function_call_arguments.delta",
						SequenceNumber: nextSeq(),
						ItemID:         fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]),
						OutputIndex:    idx,
						Delta:          argsJSON,
					}
					out = append(out, emitEvent(ad.Type, marshalEvent(ad)))
				}

				return true
			}

			return true
		})
	}

	// Finalization on finishReason
	if fr := root.Get("candidates.0.finishReason"); fr.Exists() && fr.String() != "" {
		finalizeReasoning()
		if st.MsgOpened {
			done := ResponseOutputTextDone{
				Type:           "response.output_text.done",
				SequenceNumber: nextSeq(),
				ItemID:         st.CurrentMsgID,
				OutputIndex:    st.MsgIndex,
				ContentIndex:   0,
				Text:           "",
				Logprobs:       []any{},
			}
			out = append(out, emitEvent(done.Type, marshalEvent(done)))

			partDone := ResponseContentPartDone{
				Type:           "response.content_part.done",
				SequenceNumber: nextSeq(),
				ItemID:         st.CurrentMsgID,
				OutputIndex:    st.MsgIndex,
				ContentIndex:   0,
				Part: ContentPart{
					Type:        "output_text",
					Annotations: []any{},
					Logprobs:    []any{},
					Text:        "",
				},
			}
			out = append(out, emitEvent(partDone.Type, marshalEvent(partDone)))

			final := ResponseOutputItemDone{
				Type:           "response.output_item.done",
				SequenceNumber: nextSeq(),
				OutputIndex:    st.MsgIndex,
				Item: OutputItem{
					ID:     st.CurrentMsgID,
					Type:   "message",
					Status: "completed",
					Content: []ContentPart{{
						Type: "output_text",
						Text: "",
					}},
					Role: "assistant",
				},
			}
			out = append(out, emitEvent(final.Type, marshalEvent(final)))
		}

		if len(st.FuncArgsBuf) > 0 {
			idxs := make([]int, 0, len(st.FuncArgsBuf))
			for idx := range st.FuncArgsBuf {
				idxs = append(idxs, idx)
			}
			for i := 0; i < len(idxs); i++ {
				for j := i + 1; j < len(idxs); j++ {
					if idxs[j] < idxs[i] {
						idxs[i], idxs[j] = idxs[j], idxs[i]
					}
				}
			}

			for _, idx := range idxs {
				args := "{}"
				if b := st.FuncArgsBuf[idx]; b != nil && b.Len() > 0 {
					args = b.String()
				}

				fcDone := ResponseFunctionCallArgumentsDone{
					Type:           "response.function_call_arguments.done",
					SequenceNumber: nextSeq(),
					ItemID:         fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]),
					OutputIndex:    idx,
					Arguments:      args,
				}
				out = append(out, emitEvent(fcDone.Type, marshalEvent(fcDone)))

				itemDone := ResponseOutputItemDone{
					Type:           "response.output_item.done",
					SequenceNumber: nextSeq(),
					OutputIndex:    idx,
					Item: OutputItem{
						ID:        fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]),
						Type:      "function_call",
						Status:    "completed",
						Arguments: args,
						CallID:    st.FuncCallIDs[idx],
						Name:      st.FuncNames[idx],
					},
				}
				out = append(out, emitEvent(itemDone.Type, marshalEvent(itemDone)))
			}
		}

		// Response Completed
		completed := ResponseCompleted{
			Type:           "response.completed",
			SequenceNumber: nextSeq(),
			Response: ResponseInfo{
				ID:         st.ResponseID,
				Object:     "response",
				CreatedAt:  st.CreatedAt,
				Status:     "completed",
				Background: false,
				Error:      nil,
			},
		}

		if requestRawJSON != nil {
			req := gjson.ParseBytes(requestRawJSON)
			if v := req.Get("instructions"); v.Exists() {
				completed.Response.Instructions = v.String()
			}
			if v := req.Get("max_output_tokens"); v.Exists() {
				completed.Response.MaxOutputTokens = v.Int()
			}
			if v := req.Get("max_tool_calls"); v.Exists() {
				completed.Response.MaxToolCalls = v.Int()
			}
			if v := req.Get("model"); v.Exists() {
				completed.Response.Model = v.String()
			}
			if v := req.Get("parallel_tool_calls"); v.Exists() {
				val := v.Bool()
				completed.Response.ParallelToolCalls = &val
			}
			if v := req.Get("previous_response_id"); v.Exists() {
				completed.Response.PreviousResponseID = v.String()
			}
			if v := req.Get("prompt_cache_key"); v.Exists() {
				completed.Response.PromptCacheKey = v.String()
			}
			if v := req.Get("reasoning"); v.Exists() {
				completed.Response.Reasoning = v.Value()
			}
			if v := req.Get("safety_identifier"); v.Exists() {
				completed.Response.SafetyIdentifier = v.String()
			}
			if v := req.Get("service_tier"); v.Exists() {
				completed.Response.ServiceTier = v.String()
			}
			if v := req.Get("store"); v.Exists() {
				val := v.Bool()
				completed.Response.Store = &val
			}
			if v := req.Get("temperature"); v.Exists() {
				val := v.Float()
				completed.Response.Temperature = &val
			}
			if v := req.Get("text"); v.Exists() {
				completed.Response.Text = v.Value()
			}
			if v := req.Get("tool_choice"); v.Exists() {
				completed.Response.ToolChoice = v.Value()
			}
			if v := req.Get("tools"); v.Exists() {
				completed.Response.Tools = v.Value()
			}
			if v := req.Get("top_logprobs"); v.Exists() {
				completed.Response.TopLogprobs = v.Int()
			}
			if v := req.Get("top_p"); v.Exists() {
				val := v.Float()
				completed.Response.TopP = &val
			}
			if v := req.Get("truncation"); v.Exists() {
				completed.Response.Truncation = v.String()
			}
			if v := req.Get("user"); v.Exists() {
				completed.Response.User = v.Value()
			}
			if v := req.Get("metadata"); v.Exists() {
				completed.Response.Metadata = v.Value()
			}
		}

		// Compose outputs
		outputs := make([]any, 0)
		if st.ReasoningOpened {
			outputs = append(outputs, OutputItem{
				ID:      st.ReasoningItemID,
				Type:    "reasoning",
				Summary: []SummaryPart{{Type: "summary_text", Text: st.ReasoningBuf.String()}},
			})
		}
		if st.MsgOpened {
			outputs = append(outputs, OutputItem{
				ID:      st.CurrentMsgID,
				Type:    "message",
				Status:  "completed",
				Content: []ContentPart{{Type: "output_text", Annotations: []any{}, Logprobs: []any{}, Text: st.TextBuf.String()}},
				Role:    "assistant",
			})
		}
		if len(st.FuncArgsBuf) > 0 {
			idxs := make([]int, 0, len(st.FuncArgsBuf))
			for idx := range st.FuncArgsBuf {
				idxs = append(idxs, idx)
			}
			for i := 0; i < len(idxs); i++ {
				for j := i + 1; j < len(idxs); j++ {
					if idxs[j] < idxs[i] {
						idxs[i], idxs[j] = idxs[j], idxs[i]
					}
				}
			}
			for _, idx := range idxs {
				args := ""
				if b := st.FuncArgsBuf[idx]; b != nil {
					args = b.String()
				}
				outputs = append(outputs, OutputItem{
					ID:        fmt.Sprintf("fc_%s", st.FuncCallIDs[idx]),
					Type:      "function_call",
					Status:    "completed",
					Arguments: args,
					CallID:    st.FuncCallIDs[idx],
					Name:      st.FuncNames[idx],
				})
			}
		}

		if len(outputs) > 0 {
			completed.Response.Output = &outputs
		}

		// usage mapping
		if um := root.Get("usageMetadata"); um.Exists() {
			// input tokens = prompt + thoughts
			input := um.Get("promptTokenCount").Int() + um.Get("thoughtsTokenCount").Int()

			usage := &ResponseUsage{
				InputTokens:        input,
				InputTokensDetails: &InputTokensDetails{CachedTokens: 0},
			}

			// output tokens
			if v := um.Get("candidatesTokenCount"); v.Exists() {
				usage.OutputTokens = v.Int()
			}

			var reasoningTokens int64
			if v := um.Get("thoughtsTokenCount"); v.Exists() {
				reasoningTokens = v.Int()
			}
			usage.OutputTokensDetails = &OutputTokensDetails{ReasoningTokens: reasoningTokens}

			if v := um.Get("totalTokenCount"); v.Exists() {
				usage.TotalTokens = v.Int()
			}

			completed.Response.Usage = usage
		}

		out = append(out, emitEvent(completed.Type, marshalEvent(completed)))
	}

	return out
}

// ConvertGeminiResponseToOpenAIResponsesNonStream aggregates Gemini response JSON into a single OpenAI Responses JSON object.
func ConvertGeminiResponseToOpenAIResponsesNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) string {
	root := gjson.ParseBytes(rawJSON)

	// Base response scaffold
	resp := ResponseInfo{
		Object:     "response",
		Status:     "completed",
		Background: false,
	}

	// id: prefer provider responseId, otherwise synthesize
	id := root.Get("responseId").String()
	if id == "" {
		id = fmt.Sprintf("resp_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&responseIDCounter, 1))
	}
	// Normalize to response-style id (prefix resp_ if missing)
	if !strings.HasPrefix(id, "resp_") {
		id = fmt.Sprintf("resp_%s", id)
	}
	resp.ID = id

	// created_at: map from createTime if available
	createdAt := time.Now().Unix()
	if v := root.Get("createTime"); v.Exists() {
		if t, err := time.Parse(time.RFC3339Nano, v.String()); err == nil {
			createdAt = t.Unix()
		}
	}
	resp.CreatedAt = createdAt

	// Echo request fields when present; fallback model from response modelVersion
	if len(requestRawJSON) > 0 {
		req := gjson.ParseBytes(requestRawJSON)
		if v := req.Get("instructions"); v.Exists() {
			resp.Instructions = v.String()
		}
		if v := req.Get("max_output_tokens"); v.Exists() {
			resp.MaxOutputTokens = v.Int()
		}
		if v := req.Get("max_tool_calls"); v.Exists() {
			resp.MaxToolCalls = v.Int()
		}
		if v := req.Get("model"); v.Exists() {
			resp.Model = v.String()
		} else if v = root.Get("modelVersion"); v.Exists() {
			resp.Model = v.String()
		}
		if v := req.Get("parallel_tool_calls"); v.Exists() {
			val := v.Bool()
			resp.ParallelToolCalls = &val
		}
		if v := req.Get("previous_response_id"); v.Exists() {
			resp.PreviousResponseID = v.String()
		}
		if v := req.Get("prompt_cache_key"); v.Exists() {
			resp.PromptCacheKey = v.String()
		}
		if v := req.Get("reasoning"); v.Exists() {
			resp.Reasoning = v.Value()
		}
		if v := req.Get("safety_identifier"); v.Exists() {
			resp.SafetyIdentifier = v.String()
		}
		if v := req.Get("service_tier"); v.Exists() {
			resp.ServiceTier = v.String()
		}
		if v := req.Get("store"); v.Exists() {
			val := v.Bool()
			resp.Store = &val
		}
		if v := req.Get("temperature"); v.Exists() {
			val := v.Float()
			resp.Temperature = &val
		}
		if v := req.Get("text"); v.Exists() {
			resp.Text = v.Value()
		}
		if v := req.Get("tool_choice"); v.Exists() {
			resp.ToolChoice = v.Value()
		}
		if v := req.Get("tools"); v.Exists() {
			resp.Tools = v.Value()
		}
		if v := req.Get("top_logprobs"); v.Exists() {
			resp.TopLogprobs = v.Int()
		}
		if v := req.Get("top_p"); v.Exists() {
			val := v.Float()
			resp.TopP = &val
		}
		if v := req.Get("truncation"); v.Exists() {
			resp.Truncation = v.String()
		}
		if v := req.Get("user"); v.Exists() {
			resp.User = v.Value()
		}
		if v := req.Get("metadata"); v.Exists() {
			resp.Metadata = v.Value()
		}
	} else if v := root.Get("modelVersion"); v.Exists() {
		resp.Model = v.String()
	}

	// Build outputs from candidates[0].content.parts
	var reasoningText strings.Builder
	var reasoningEncrypted string
	var messageText strings.Builder
	var haveMessage bool
	var outputs []any

	if parts := root.Get("candidates.0.content.parts"); parts.Exists() && parts.IsArray() {
		parts.ForEach(func(_, p gjson.Result) bool {
			if p.Get("thought").Bool() {
				if t := p.Get("text"); t.Exists() {
					reasoningText.WriteString(t.String())
				}
				if sig := p.Get("thoughtSignature"); sig.Exists() && sig.String() != "" {
					reasoningEncrypted = sig.String()
				}
				return true
			}
			if t := p.Get("text"); t.Exists() && t.String() != "" {
				messageText.WriteString(t.String())
				haveMessage = true
				return true
			}
			if fc := p.Get("functionCall"); fc.Exists() {
				name := fc.Get("name").String()
				args := fc.Get("args")
				callID := fmt.Sprintf("call_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&funcCallIDCounter, 1))

				item := OutputItem{
					ID:     fmt.Sprintf("fc_%s", callID),
					Type:   "function_call",
					Status: "completed",
					CallID: callID,
					Name:   name,
				}

				if args.Exists() {
					item.Arguments = args.Raw
				}
				outputs = append(outputs, item)
				return true
			}
			return true
		})
	}

	// Reasoning output item
	if reasoningText.Len() > 0 || reasoningEncrypted != "" {
		rid := strings.TrimPrefix(id, "resp_")
		item := OutputItem{
			ID:               fmt.Sprintf("rs_%s", rid),
			Type:             "reasoning",
			EncryptedContent: reasoningEncrypted,
		}

		if reasoningText.Len() > 0 {
			item.Summary = []SummaryPart{{
				Type: "summary_text",
				Text: reasoningText.String(),
			}}
		}
		outputs = append(outputs, item)
	}

	// Assistant message output item
	if haveMessage {
		item := OutputItem{
			ID:     fmt.Sprintf("msg_%s_0", strings.TrimPrefix(id, "resp_")),
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []ContentPart{{
				Type:        "output_text",
				Annotations: []any{},
				Logprobs:    []any{},
				Text:        messageText.String(),
			}},
		}
		outputs = append(outputs, item)
	}

	if len(outputs) > 0 {
		resp.Output = &outputs
	}

	// usage mapping
	if um := root.Get("usageMetadata"); um.Exists() {
		// input tokens = prompt + thoughts
		input := um.Get("promptTokenCount").Int() + um.Get("thoughtsTokenCount").Int()

		usage := &ResponseUsage{
			InputTokens:        input,
			InputTokensDetails: &InputTokensDetails{CachedTokens: 0},
		}

		// output tokens
		if v := um.Get("candidatesTokenCount"); v.Exists() {
			usage.OutputTokens = v.Int()
		}

		var reasoningTokens int64
		if v := um.Get("thoughtsTokenCount"); v.Exists() {
			reasoningTokens = v.Int()
		}
		usage.OutputTokensDetails = &OutputTokensDetails{ReasoningTokens: reasoningTokens}

		if v := um.Get("totalTokenCount"); v.Exists() {
			usage.TotalTokens = v.Int()
		}

		resp.Usage = usage
	}

	b, _ := json.Marshal(resp)
	return string(b)
}
