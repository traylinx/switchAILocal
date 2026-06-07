// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package virtualmodels implements public virtual model pools such as
// ail-compound. It deliberately lives below internal/ so SDK clients only see
// the stable config shape, not selector internals.
package virtualmodels

import (
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
	"sync"

	json "github.com/goccy/go-json"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/registry"
)

const (
	ClassChatText               = "chat_text"
	ClassChatTextTools          = "chat_text_tools"
	ClassChatMultiturnTools     = "chat_multiturn_tools"
	ClassChatImageUnderstanding = "chat_image_understanding"
	ClassChatAudioUnderstanding = "chat_audio_understanding"
	ClassAudioTranscription     = "audio_transcription"
	ClassImageGeneration        = "image_generation"
	ClassSpeechGeneration       = "speech_generation"
	ClassMusicGeneration        = "music_generation"
	ClassLyricsGeneration       = "lyrics_generation"
	ClassEmbeddings             = "embeddings"
)

// Requirements captures the capability contract required by a request.
type Requirements struct {
	Class          string
	InputText      bool
	InputImage     bool
	InputAudio     bool
	NeedsTools     bool
	HasToolHistory bool
	OutputText     bool
	OutputImage    bool
	OutputAudio    bool
	MinContext     int
	Endpoint       string
}

// Route is the concrete provider/model choice for one request.
type Route struct {
	PublicModel  string
	Provider     string
	NativeModel  string
	MemberID     string
	Requirements Requirements
}

// NoEligibleBackendError reports explicit capability mismatch without trying an
// upstream provider.
type NoEligibleBackendError struct {
	Model        string
	Requirements Requirements
}

func (e NoEligibleBackendError) Error() string {
	class := e.Requirements.Class
	if class == "" {
		class = "unknown"
	}
	return fmt.Sprintf("no eligible backend for %s request: requires %s", e.Model, class)
}

func (e NoEligibleBackendError) Code() string { return "no_eligible_backend" }

// Router keeps process-local weighted round-robin cursors.
type Router struct {
	mu      sync.Mutex
	cursors map[string]int
	seed    int
}

func NewRouter() *Router {
	return &Router{cursors: make(map[string]int), seed: instanceSeed()}
}

func instanceSeed() int {
	id := firstNonEmpty(os.Getenv("INSTANCE_ID"), os.Getenv("AIL_INSTANCE_ID"), os.Getenv("HOSTNAME"))
	if id == "" {
		if host, err := os.Hostname(); err == nil {
			id = host
		}
	}
	if id == "" {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32())
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// IsVirtualModel reports whether model maps to a configured virtual pool.
func IsVirtualModel(cfg *config.SDKConfig, model string) bool {
	_, ok := getPool(cfg, model)
	return ok
}

func getPool(cfg *config.SDKConfig, model string) (config.VirtualModelConfig, bool) {
	if cfg == nil || len(cfg.VirtualModels) == 0 {
		return config.VirtualModelConfig{}, false
	}
	for id, pool := range cfg.VirtualModels {
		if strings.EqualFold(strings.TrimSpace(id), strings.TrimSpace(model)) {
			return pool, true
		}
	}
	return config.VirtualModelConfig{}, false
}

// Select chooses one eligible member for a virtual model.
func (r *Router) Select(cfg *config.SDKConfig, model string, rawJSON []byte, endpoint string) (Route, error) {
	return r.SelectExcluding(cfg, model, rawJSON, endpoint, nil)
}

// SelectExcluding chooses an eligible member while skipping member IDs in exclude.
func (r *Router) SelectExcluding(cfg *config.SDKConfig, model string, rawJSON []byte, endpoint string, exclude map[string]struct{}) (Route, error) {
	pool, ok := getPool(cfg, model)
	if !ok {
		return Route{}, fmt.Errorf("model %q is not virtual", model)
	}
	req := DetectRequirements(rawJSON, endpoint)
	eligible := eligibleMembers(pool, req)
	if len(exclude) > 0 {
		filtered := eligible[:0]
		for _, member := range eligible {
			if _, skip := exclude[member.ID]; skip {
				continue
			}
			filtered = append(filtered, member)
		}
		eligible = filtered
	}
	if len(eligible) == 0 {
		return Route{}, NoEligibleBackendError{Model: model, Requirements: req}
	}
	member := r.pick(model, req.Class, eligible)
	return Route{PublicModel: model, Provider: member.Provider, NativeModel: member.Model, MemberID: member.ID, Requirements: req}, nil
}

// FallbackEnabled reports whether a virtual pool should try another eligible
// member on recoverable upstream failures.
func FallbackEnabled(cfg *config.SDKConfig, model string) bool {
	pool, ok := getPool(cfg, model)
	return ok && pool.Fallback
}

func (r *Router) pick(model, class string, eligible []config.VirtualModelMemberConfig) config.VirtualModelMemberConfig {
	expanded := make([]config.VirtualModelMemberConfig, 0, len(eligible))
	for _, member := range eligible {
		weight := member.Weight
		if weight <= 0 {
			weight = 1
		}
		for i := 0; i < weight; i++ {
			expanded = append(expanded, member)
		}
	}
	sort.SliceStable(expanded, func(i, j int) bool {
		if strings.EqualFold(expanded[i].ID, expanded[j].ID) {
			return expanded[i].Model < expanded[j].Model
		}
		return expanded[i].ID < expanded[j].ID
	})
	key := strings.ToLower(model + "|" + class)
	r.mu.Lock()
	defer r.mu.Unlock()
	cursor, ok := r.cursors[key]
	if !ok {
		cursor = r.seed % len(expanded)
	}
	member := expanded[cursor%len(expanded)]
	r.cursors[key] = (cursor + 1) % len(expanded)
	return member
}

func eligibleMembers(pool config.VirtualModelConfig, req Requirements) []config.VirtualModelMemberConfig {
	out := make([]config.VirtualModelMemberConfig, 0, len(pool.Members))
	for _, member := range pool.Members {
		if member.Enabled != nil && !*member.Enabled {
			continue
		}
		if member.Provider == "" || member.Model == "" {
			continue
		}
		if member.Weight < 0 {
			continue
		}
		if capabilitiesSatisfy(member.Capabilities, req) {
			out = append(out, member)
		}
	}
	return out
}

func capabilitiesSatisfy(c config.VirtualModelCapabilitiesConfig, req Requirements) bool {
	ops := normalizeSet(c.Operations)
	inputs := normalizeSet(c.Input)
	outputs := normalizeSet(c.Output)
	if len(outputs) == 0 {
		outputs["text"] = struct{}{}
	}
	if !classAllowed(ops, req.Class) {
		return false
	}
	if req.InputText && !has(inputs, "text") {
		return false
	}
	if req.InputImage && !has(inputs, "image") {
		return false
	}
	if req.InputAudio && !has(inputs, "audio") {
		return false
	}
	if req.OutputText && !has(outputs, "text") {
		return false
	}
	if req.OutputImage && !has(outputs, "image") {
		return false
	}
	if req.OutputAudio && !has(outputs, "audio") {
		return false
	}
	if req.NeedsTools && !c.Tools {
		return false
	}
	if req.HasToolHistory && !c.AgenticSafe && !c.ToolHistoryReplay {
		return false
	}
	if req.MinContext > 0 && c.Context > 0 && c.Context < req.MinContext {
		return false
	}
	return true
}

func classAllowed(ops map[string]struct{}, class string) bool {
	if len(ops) == 0 {
		// Strict default: only text chat if operator omitted operations.
		return class == ClassChatText
	}
	if has(ops, class) {
		return true
	}
	switch class {
	case ClassChatText:
		return has(ops, "chat") || has(ops, "text")
	case ClassChatTextTools:
		return has(ops, "chat_text_tools") || has(ops, "chat_tools") || has(ops, "chat")
	case ClassChatMultiturnTools:
		return has(ops, "chat_multiturn_tools")
	case ClassChatImageUnderstanding:
		return has(ops, "chat_image_understanding") || has(ops, "vision")
	case ClassChatAudioUnderstanding:
		return has(ops, "chat_audio_understanding") || has(ops, "audio_understanding")
	case ClassAudioTranscription:
		return has(ops, "audio_transcription") || has(ops, "transcription")
	case ClassImageGeneration:
		return has(ops, "image_generation") || has(ops, "image_gen")
	case ClassSpeechGeneration:
		return has(ops, "speech_generation") || has(ops, "speech")
	case ClassMusicGeneration:
		return has(ops, "music_generation") || has(ops, "music")
	case ClassLyricsGeneration:
		return has(ops, "lyrics_generation") || has(ops, "lyrics")
	case ClassEmbeddings:
		return has(ops, "embeddings") || has(ops, "embedding")
	}
	return false
}

func normalizeSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func has(set map[string]struct{}, value string) bool {
	_, ok := set[strings.ToLower(value)]
	return ok
}

// DetectRequirements classifies OpenAI-compatible requests by endpoint and body.
func DetectRequirements(rawJSON []byte, endpoint string) Requirements {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	req := Requirements{Endpoint: endpoint, InputText: true, OutputText: true}
	switch endpoint {
	case "embeddings":
		req.Class = ClassEmbeddings
		return req
	case "images_generations", "image_generation", "image_gen":
		req.Class = ClassImageGeneration
		req.OutputText = false
		req.OutputImage = true
		return req
	case "audio_speech", "speech_generation", "speech":
		req.Class = ClassSpeechGeneration
		req.OutputText = false
		req.OutputAudio = true
		return req
	case "audio_transcriptions", "audio_translation", "transcription":
		req.Class = ClassAudioTranscription
		req.InputAudio = true
		return req
	case "music_generation", "music_generations":
		req.Class = ClassMusicGeneration
		req.OutputAudio = true
		return req
	case "lyrics_generation", "music_lyrics":
		req.Class = ClassLyricsGeneration
		return req
	}

	if hasImageContent(rawJSON) {
		req.Class = ClassChatImageUnderstanding
		req.InputImage = true
		req.NeedsTools = requestHasTools(rawJSON)
		return req
	}
	if hasAudioContent(rawJSON) {
		req.Class = ClassChatAudioUnderstanding
		req.InputAudio = true
		req.NeedsTools = requestHasTools(rawJSON)
		return req
	}
	req.NeedsTools = requestHasTools(rawJSON)
	req.HasToolHistory = hasToolHistory(rawJSON)
	switch {
	case req.HasToolHistory:
		req.Class = ClassChatMultiturnTools
		req.NeedsTools = true
	case req.NeedsTools:
		req.Class = ClassChatTextTools
	default:
		req.Class = ClassChatText
	}
	return req
}

func requestHasTools(rawJSON []byte) bool {
	tools := gjson.GetBytes(rawJSON, "tools")
	if tools.Exists() && tools.Type != gjson.Null {
		if tools.IsArray() {
			return len(tools.Array()) > 0
		}
		return true
	}
	toolChoice := gjson.GetBytes(rawJSON, "tool_choice")
	return toolChoice.Exists() && toolChoice.Type != gjson.Null && toolChoice.String() != "none"
}

func hasToolHistory(rawJSON []byte) bool {
	for _, msg := range gjson.GetBytes(rawJSON, "messages").Array() {
		if strings.EqualFold(msg.Get("role").String(), "tool") {
			return true
		}
		if msg.Get("tool_call_id").Exists() {
			return true
		}
		if strings.EqualFold(msg.Get("role").String(), "assistant") {
			calls := msg.Get("tool_calls")
			if calls.Exists() && calls.IsArray() && len(calls.Array()) > 0 {
				return true
			}
		}
	}
	return false
}

func hasImageContent(rawJSON []byte) bool {
	found := false
	gjson.GetBytes(rawJSON, "messages").ForEach(func(_, msg gjson.Result) bool {
		if contentHasType(msg.Get("content"), "image_url", "image") {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasAudioContent(rawJSON []byte) bool {
	found := false
	gjson.GetBytes(rawJSON, "messages").ForEach(func(_, msg gjson.Result) bool {
		if contentHasType(msg.Get("content"), "input_audio", "audio", "audio_url") {
			found = true
			return false
		}
		return true
	})
	return found
}

func contentHasType(content gjson.Result, types ...string) bool {
	if !content.Exists() {
		return false
	}
	targets := normalizeSet(types)
	if content.IsArray() {
		for _, part := range content.Array() {
			if has(targets, part.Get("type").String()) || part.Get("image_url").Exists() || part.Get("input_audio").Exists() {
				if part.Get("image_url").Exists() && has(targets, "image_url") {
					return true
				}
				if part.Get("input_audio").Exists() && has(targets, "input_audio") {
					return true
				}
				if has(targets, part.Get("type").String()) {
					return true
				}
			}
		}
	}
	return false
}

// PublicCatalog returns provider-neutral public virtual models for /v1/models.
func PublicCatalog(cfg *config.SDKConfig) []map[string]any {
	if cfg == nil || len(cfg.VirtualModels) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cfg.VirtualModels))
	for id := range cfg.VirtualModels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		pool := cfg.VirtualModels[id]
		if !pool.Expose {
			continue
		}
		info := registry.ModelInfo{
			ID:           id,
			Object:       "model",
			OwnedBy:      "switchai",
			Type:         "virtual",
			Description:  pool.Description,
			Capabilities: catalogCapabilities(pool),
		}
		model := map[string]any{"id": info.ID, "object": "model", "owned_by": info.OwnedBy, "type": info.Type}
		if info.Description != "" {
			model["description"] = info.Description
		}
		if info.Capabilities != nil {
			model["attachment"] = info.Capabilities.Attachment
			model["tool_call"] = info.Capabilities.ToolCall
			model["reasoning"] = info.Capabilities.Reasoning
			model["modalities"] = info.Capabilities.Modalities
			if contains(info.Capabilities.Modalities.Input, "image") {
				model["vision"] = true
			}
			if contains(info.Capabilities.Modalities.Input, "audio") {
				model["audio"] = true
			}
		}
		// Aggregate context length from enabled pool members (max-of-pool = the
		// headline context for the alias). switchAI clients (pi-switchai-provider,
		// Codex, OpenCode, etc.) read context_length from /v1/models to display
		// the right value without hardcoding per-pool.
		var maxContext int
		for _, member := range pool.Members {
			if member.Enabled != nil && !*member.Enabled {
				continue
			}
			if member.Capabilities.Context > maxContext {
				maxContext = member.Capabilities.Context
			}
		}
		if maxContext > 0 {
			model["context_length"] = maxContext
		}
		out = append(out, model)
	}
	return out
}

func catalogCapabilities(pool config.VirtualModelConfig) *registry.ModelCapabilities {
	input := map[string]struct{}{}
	output := map[string]struct{}{}
	tool := false
	for _, member := range pool.Members {
		if member.Enabled != nil && !*member.Enabled {
			continue
		}
		for _, v := range member.Capabilities.Input {
			input[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
		}
		for _, v := range member.Capabilities.Output {
			output[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
		}
		if member.Capabilities.Tools {
			tool = true
		}
	}
	if len(input) == 0 {
		input["text"] = struct{}{}
	}
	if len(output) == 0 {
		output["text"] = struct{}{}
	}
	in := setToSortedSlice(input)
	out := setToSortedSlice(output)
	return &registry.ModelCapabilities{
		Attachment: contains(in, "image") || contains(in, "audio") || contains(in, "pdf"),
		ToolCall:   tool,
		Modalities: registry.ModelModalities{Input: in, Output: out},
	}
}

func setToSortedSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for item := range set {
		if item != "" {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

// RewriteModelField rewrites a JSON object model field when present. It also
// supports raw SSE payload text; data: [DONE] is preserved.
func RewriteModelField(payload []byte, publicModel string) []byte {
	if strings.TrimSpace(publicModel) == "" || len(payload) == 0 {
		return payload
	}
	text := string(payload)
	if strings.Contains(text, "data:") {
		lines := strings.Split(text, "\n")
		changed := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data == "" || data == "[DONE]" || !json.Valid([]byte(data)) || !gjson.Get(data, "model").Exists() {
				continue
			}
			rewritten, err := sjson.Set(data, "model", publicModel)
			if err == nil {
				prefix := line[:strings.Index(line, "data:")+len("data:")]
				lines[i] = prefix + " " + rewritten
				changed = true
			}
		}
		if changed {
			return []byte(strings.Join(lines, "\n"))
		}
		return payload
	}
	trimmed := strings.TrimSpace(text)
	if !json.Valid([]byte(trimmed)) || !gjson.Get(trimmed, "model").Exists() {
		return payload
	}
	rewritten, err := sjson.Set(trimmed, "model", publicModel)
	if err != nil {
		return payload
	}
	return []byte(rewritten)
}
