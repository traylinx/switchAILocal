// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package virtualmodels implements public virtual model pools such as
// ail-compound. It deliberately lives below internal/ so SDK clients only see
// the stable config shape, not selector internals.
package virtualmodels

import (
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	json "github.com/goccy/go-json"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/registry"
	"github.com/traylinx/switchAILocal/internal/util"
)

const (
	ClassChatText                    = "chat_text"
	ClassChatTextTools               = "chat_text_tools"
	ClassChatMultiturnTools          = "chat_multiturn_tools"
	ClassChatImageUnderstanding      = "chat_image_understanding"
	ClassChatAudioUnderstanding      = "chat_audio_understanding"
	ClassChatMultimodalUnderstanding = "chat_multimodal_understanding"
	ClassAudioTranscription          = "audio_transcription"
	ClassImageGeneration             = "image_generation"
	ClassSpeechGeneration            = "speech_generation"
	ClassMusicGeneration             = "music_generation"
	ClassLyricsGeneration            = "lyrics_generation"
	ClassEmbeddings                  = "embeddings"

	routerStateSchemaVersion = 2
	routerAlgorithm          = "smooth-weighted-round-robin/v1"
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

// RouterState is the persisted, operator-observable routing state for virtual
// model pools. Pool keys start with lower-case "<public-model>|<request-class>"
// and may include media/tool/context qualifiers that affect eligible members.
type RouterState struct {
	SchemaVersion int                        `json:"schema_version"`
	Algorithm     string                     `json:"algorithm"`
	Pools         map[string]RouterPoolState `json:"pools"`
}

// RouterPoolState stores the smooth weighted round-robin accumulators for one
// virtual model routing bucket.
type RouterPoolState struct {
	ConfigHash   string         `json:"config_hash"`
	Current      map[string]int `json:"current"`
	Counts       map[string]int `json:"counts"`
	LastSelected string         `json:"last_selected,omitempty"`
}

// Router keeps smooth weighted round-robin accumulators for virtual model pools.
// State is persisted best-effort so local process restarts continue proportional
// routing without changing the public OpenAI-compatible API contract.
type Router struct {
	mu            sync.Mutex
	seed          int
	stateFilePath string
	state         RouterState
	stateLoaded   bool
}

func NewRouter() *Router {
	return NewRouterWithStatePath("")
}

func NewRouterWithStatePath(path string) *Router {
	return &Router{
		seed:          instanceSeed(),
		stateFilePath: path,
		state:         newRouterState(),
	}
}

func newRouterState() RouterState {
	return RouterState{
		SchemaVersion: routerStateSchemaVersion,
		Algorithm:     routerAlgorithm,
		Pools:         make(map[string]RouterPoolState),
	}
}

func (s *RouterState) ensure() {
	s.SchemaVersion = routerStateSchemaVersion
	s.Algorithm = routerAlgorithm
	if s.Pools == nil {
		s.Pools = make(map[string]RouterPoolState)
	}
	for key, pool := range s.Pools {
		pool.ensure()
		s.Pools[key] = pool
	}
}

func (s RouterState) clone() RouterState {
	out := newRouterState()
	out.SchemaVersion = s.SchemaVersion
	out.Algorithm = s.Algorithm
	for k, v := range s.Pools {
		out.Pools[k] = v.clone()
	}
	return out
}

func (s *RouterPoolState) ensure() {
	if s.Current == nil {
		s.Current = make(map[string]int)
	}
	if s.Counts == nil {
		s.Counts = make(map[string]int)
	}
}

func (s RouterPoolState) clone() RouterPoolState {
	out := RouterPoolState{
		ConfigHash:   s.ConfigHash,
		Current:      make(map[string]int, len(s.Current)),
		Counts:       make(map[string]int, len(s.Counts)),
		LastSelected: s.LastSelected,
	}
	for k, v := range s.Current {
		out.Current[k] = v
	}
	for k, v := range s.Counts {
		out.Counts[k] = v
	}
	return out
}

func (r *Router) getStateFilePath() string {
	if r.stateFilePath != "" {
		return r.stateFilePath
	}
	if sb, err := util.NewStateBox(); err == nil {
		return filepath.Join(sb.RootPath(), "virtual_models_state.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".switchailocal", "virtual_models_state.json")
}

// Snapshot returns a copy of the current router state for tests and
// observability. It best-effort loads persisted state before returning.
func (r *Router) Snapshot() RouterState {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadStateLocked()
	return r.state.clone()
}

func (r *Router) loadStateLocked() {
	if r.stateLoaded {
		r.state.ensure()
		return
	}
	r.state.ensure()
	statePath := r.getStateFilePath()
	if data, err := os.ReadFile(statePath); err == nil {
		var disk RouterState
		if err := json.Unmarshal(data, &disk); err == nil {
			if disk.SchemaVersion == routerStateSchemaVersion && disk.Algorithm == routerAlgorithm {
				disk.ensure()
				r.state = disk
			} else {
				log.WithFields(log.Fields{
					"schema_version": disk.SchemaVersion,
					"algorithm":      disk.Algorithm,
				}).Debug("ignored incompatible virtual model router state")
			}
		} else {
			log.WithError(err).Debug("ignored invalid virtual model router state")
		}
	}
	r.stateLoaded = true
}

func (r *Router) persistStateLocked() {
	r.state.ensure()
	statePath := r.getStateFilePath()
	sb, _ := util.NewStateBox()
	if err := util.SecureWriteJSON(sb, statePath, &r.state, nil); err != nil {
		// Routing must never fail because the state file cannot be written.
		log.WithError(err).Debug("failed to persist virtual model router state")
	}
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
	mutateState := len(exclude) == 0
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
	member := r.pick(model, req, eligible, mutateState)
	return Route{PublicModel: model, Provider: member.Provider, NativeModel: member.Model, MemberID: member.ID, Requirements: req}, nil
}

// FallbackEnabled reports whether a virtual pool should try another eligible
// member on recoverable upstream failures.
func FallbackEnabled(cfg *config.SDKConfig, model string) bool {
	pool, ok := getPool(cfg, model)
	return ok && pool.Fallback
}

type weightedMember struct {
	member config.VirtualModelMemberConfig
	id     string
	weight int
}

func (r *Router) pick(model string, req Requirements, eligible []config.VirtualModelMemberConfig, mutateState bool) config.VirtualModelMemberConfig {
	members := sortedWeightedMembers(eligible)
	if len(members) == 0 {
		return config.VirtualModelMemberConfig{}
	}
	key := routingStateKey(model, req)
	configHash := routingConfigHash(model, key, members)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadStateLocked()

	if !mutateState {
		return r.pickWithoutMutationLocked(key, members).member
	}

	poolState := r.poolStateForLocked(key, configHash, members)
	selected := smoothPick(&poolState, members)
	r.state.Pools[key] = poolState

	log.WithFields(log.Fields{
		"virtual_model": model,
		"request_class": req.Class,
		"state_key":     key,
		"member_id":     selected.id,
		"algorithm":     routerAlgorithm,
		"total_routed":  poolState.Counts[selected.id],
	}).Info("routed virtual model member")

	r.persistStateLocked()
	return selected.member
}

func sortedWeightedMembers(eligible []config.VirtualModelMemberConfig) []weightedMember {
	members := make([]weightedMember, 0, len(eligible))
	for _, member := range eligible {
		id := strings.TrimSpace(member.ID)
		if id == "" {
			continue
		}
		members = append(members, weightedMember{member: member, id: id, weight: effectiveWeight(member)})
	}
	sort.SliceStable(members, func(i, j int) bool {
		leftID := strings.ToLower(members[i].id)
		rightID := strings.ToLower(members[j].id)
		if leftID == rightID {
			return members[i].member.Model < members[j].member.Model
		}
		return leftID < rightID
	})
	return members
}

func effectiveWeight(member config.VirtualModelMemberConfig) int {
	if member.Weight <= 0 {
		return 1
	}
	return member.Weight
}

func (r *Router) poolStateForLocked(key, configHash string, members []weightedMember) RouterPoolState {
	poolState, ok := r.state.Pools[key]
	if !ok || poolState.ConfigHash != configHash {
		return newRouterPoolState(configHash, members)
	}
	poolState.ensure()
	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberSet[member.id] = struct{}{}
		if _, ok := poolState.Current[member.id]; !ok {
			poolState.Current[member.id] = 0
		}
		if _, ok := poolState.Counts[member.id]; !ok {
			poolState.Counts[member.id] = 0
		}
	}
	for id := range poolState.Current {
		if _, ok := memberSet[id]; !ok {
			delete(poolState.Current, id)
		}
	}
	for id := range poolState.Counts {
		if _, ok := memberSet[id]; !ok {
			delete(poolState.Counts, id)
		}
	}
	return poolState
}

func newRouterPoolState(configHash string, members []weightedMember) RouterPoolState {
	poolState := RouterPoolState{
		ConfigHash: configHash,
		Current:    make(map[string]int, len(members)),
		Counts:     make(map[string]int, len(members)),
	}
	for _, member := range members {
		poolState.Current[member.id] = 0
		poolState.Counts[member.id] = 0
	}
	return poolState
}

func smoothPick(poolState *RouterPoolState, members []weightedMember) weightedMember {
	poolState.ensure()
	totalWeight := 0
	selectedIdx := 0
	selectedSet := false
	selectedScore := 0
	for i, member := range members {
		totalWeight += member.weight
		poolState.Current[member.id] += member.weight
		score := poolState.Current[member.id]
		if !selectedSet || score > selectedScore {
			selectedIdx = i
			selectedScore = score
			selectedSet = true
		}
	}
	selected := members[selectedIdx]
	poolState.Current[selected.id] -= totalWeight
	poolState.Counts[selected.id]++
	poolState.LastSelected = selected.id
	return selected
}

func (r *Router) pickWithoutMutationLocked(key string, members []weightedMember) weightedMember {
	if poolState, ok := r.state.Pools[key]; ok {
		poolState = poolState.clone()
		return smoothPick(&poolState, members)
	}
	idx := 0
	if len(members) > 0 {
		idx = r.seed % len(members)
	}
	if idx < 0 {
		idx = 0
	}
	return members[idx]
}

func routingStateKey(model string, req Requirements) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(model)),
		strings.ToLower(strings.TrimSpace(req.Class)),
	}
	switch req.Class {
	case ClassChatImageUnderstanding, ClassChatAudioUnderstanding, ClassChatMultimodalUnderstanding:
		if req.NeedsTools {
			parts = append(parts, "tools")
		}
		if req.HasToolHistory {
			parts = append(parts, "tool_history")
		}
	}
	if req.MinContext > 0 {
		parts = append(parts, fmt.Sprintf("ctx_%d", req.MinContext))
	}
	return strings.Join(parts, "|")
}

type routingHashInput struct {
	PublicModel  string              `json:"public_model"`
	RequestClass string              `json:"request_class"`
	Members      []routingHashMember `json:"members"`
}

type routingHashMember struct {
	ID           string                `json:"id"`
	Weight       int                   `json:"weight"`
	Capabilities routingHashCapability `json:"capabilities"`
}

type routingHashCapability struct {
	Operations        []string `json:"operations,omitempty"`
	Input             []string `json:"input,omitempty"`
	Output            []string `json:"output,omitempty"`
	Tools             bool     `json:"tools,omitempty"`
	Context           int      `json:"context,omitempty"`
	AgenticSafe       bool     `json:"agentic_safe,omitempty"`
	ToolHistoryReplay bool     `json:"tool_history_replay,omitempty"`
}

func routingConfigHash(model, stateKey string, members []weightedMember) string {
	input := routingHashInput{
		PublicModel:  strings.ToLower(strings.TrimSpace(model)),
		RequestClass: strings.ToLower(strings.TrimSpace(stateKey)),
		Members:      make([]routingHashMember, 0, len(members)),
	}
	for _, member := range members {
		caps := member.member.Capabilities
		input.Members = append(input.Members, routingHashMember{
			ID:     member.id,
			Weight: member.weight,
			Capabilities: routingHashCapability{
				Operations:        normalizedHashList(caps.Operations),
				Input:             normalizedHashList(caps.Input),
				Output:            normalizedHashList(caps.Output),
				Tools:             caps.Tools,
				Context:           caps.Context,
				AgenticSafe:       caps.AgenticSafe,
				ToolHistoryReplay: caps.ToolHistoryReplay,
			},
		})
	}
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:])
}

func normalizedHashList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func eligibleMembers(pool config.VirtualModelConfig, req Requirements) []config.VirtualModelMemberConfig {
	out := make([]config.VirtualModelMemberConfig, 0, len(pool.Members))
	for _, member := range pool.Members {
		if member.Enabled != nil && !*member.Enabled {
			continue
		}
		if strings.TrimSpace(member.ID) == "" {
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
	case ClassChatMultimodalUnderstanding:
		return has(ops, "chat_multimodal_understanding") || has(ops, "multimodal") || (classAllowed(ops, ClassChatImageUnderstanding) && classAllowed(ops, ClassChatAudioUnderstanding))
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

	req.NeedsTools = requestHasTools(rawJSON)
	req.HasToolHistory = hasToolHistory(rawJSON)

	inputImage := hasImageContent(rawJSON)
	inputAudio := hasAudioContent(rawJSON)
	if inputImage && inputAudio {
		req.Class = ClassChatMultimodalUnderstanding
		req.InputImage = true
		req.InputAudio = true
		req.NeedsTools = req.NeedsTools || req.HasToolHistory
		return req
	}
	if inputImage {
		req.Class = ClassChatImageUnderstanding
		req.InputImage = true
		req.NeedsTools = req.NeedsTools || req.HasToolHistory
		return req
	}
	if inputAudio {
		req.Class = ClassChatAudioUnderstanding
		req.InputAudio = true
		req.NeedsTools = req.NeedsTools || req.HasToolHistory
		return req
	}
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
	return responsesInputHasToolHistory(gjson.GetBytes(rawJSON, "input"))
}

func hasImageContent(rawJSON []byte) bool {
	found := false
	gjson.GetBytes(rawJSON, "messages").ForEach(func(_, msg gjson.Result) bool {
		if contentHasType(msg.Get("content"), "image_url", "image", "input_image") {
			found = true
			return false
		}
		return true
	})
	if found {
		return true
	}
	return responsesInputHasContentType(gjson.GetBytes(rawJSON, "input"), "image_url", "image", "input_image")
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
	if found {
		return true
	}
	return responsesInputHasContentType(gjson.GetBytes(rawJSON, "input"), "input_audio", "audio", "audio_url")
}

func responsesInputHasToolHistory(input gjson.Result) bool {
	if !input.Exists() {
		return false
	}
	if input.IsArray() {
		for _, item := range input.Array() {
			if responseInputItemHasToolHistory(item) {
				return true
			}
		}
	}
	return false
}

func responseInputItemHasToolHistory(item gjson.Result) bool {
	if strings.EqualFold(item.Get("role").String(), "tool") {
		return true
	}
	itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	switch itemType {
	case "function_call", "function_call_output", "tool_call", "tool_result", "computer_call", "computer_call_output":
		return true
	}
	if item.Get("tool_call_id").Exists() {
		return true
	}
	if item.Get("call_id").Exists() && strings.Contains(itemType, "call") {
		return true
	}
	content := item.Get("content")
	if content.IsArray() {
		for _, part := range content.Array() {
			partType := strings.ToLower(strings.TrimSpace(part.Get("type").String()))
			if strings.Contains(partType, "function_call") || strings.Contains(partType, "tool_") {
				return true
			}
		}
	}
	return false
}

func responsesInputHasContentType(input gjson.Result, types ...string) bool {
	if !input.Exists() {
		return false
	}
	if input.IsArray() {
		for _, item := range input.Array() {
			if resultHasType(item, types...) || contentHasType(item.Get("content"), types...) {
				return true
			}
		}
	}
	return false
}

func contentHasType(content gjson.Result, types ...string) bool {
	if !content.Exists() {
		return false
	}
	if resultHasType(content, types...) {
		return true
	}
	if content.IsArray() {
		for _, part := range content.Array() {
			if resultHasType(part, types...) {
				return true
			}
		}
	}
	return false
}

func resultHasType(result gjson.Result, types ...string) bool {
	targets := normalizeSet(types)
	if has(targets, result.Get("type").String()) {
		return true
	}
	if result.Get("image_url").Exists() && has(targets, "image_url") {
		return true
	}
	if result.Get("input_image").Exists() && has(targets, "input_image") {
		return true
	}
	if result.Get("input_audio").Exists() && has(targets, "input_audio") {
		return true
	}
	if result.Get("audio_url").Exists() && has(targets, "audio_url") {
		return true
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
