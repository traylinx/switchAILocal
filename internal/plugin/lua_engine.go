// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package plugin provides LUA-based plugin support for extending switchAILocal functionality.
// It allows users to write custom scripts that can intercept and modify API requests/responses.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/intelligence"
	"github.com/traylinx/switchAILocal/internal/intelligence/confidence"
	"github.com/traylinx/switchAILocal/internal/intelligence/embedding"
	"github.com/traylinx/switchAILocal/internal/intelligence/skills"
	"github.com/traylinx/switchAILocal/internal/intelligence/verification"
	"github.com/traylinx/switchAILocal/internal/util"
	"github.com/traylinx/switchAILocal/sdk/config"
	lua "github.com/yuin/gopher-lua"
)

type contextKey string

const (
	// SkipLuaContextKey is a context key used to prevent recursive LUA execution.
	SkipLuaContextKey contextKey = "skip_lua"
)

// Classifier defines the interface for LLM-based intent classification.
// It is used by the Lua engine to delegate classification requests back to the Go host.
type Classifier interface {
	Classify(ctx context.Context, prompt string) (string, error)
}

// IntelligenceService defines the interface for Phase 2 intelligent routing features.
// It provides access to advanced capabilities like discovery, semantic matching, and skill registry.
type IntelligenceService interface {
	IsEnabled() bool
	GetDiscoveryService() intelligence.DiscoveryServiceInterface
	GetMatrixBuilder() intelligence.MatrixBuilderInterface
	IsModelAvailable(modelID string) bool
	GetSkillRegistry() *skills.Registry
	GetEmbeddingEngine() *embedding.Engine
	GetSemanticTier() intelligence.SemanticTierInterface
	GetSemanticCache() intelligence.SemanticCacheInterface
	GetConfidenceScorer() *confidence.Scorer
	GetVerifier() *verification.Verifier
	GetCascadeManager() intelligence.CascadeManagerInterface
	GetFeedbackCollector() intelligence.FeedbackCollectorInterface
}

// Hook types for plugin execution points
const (
	HookOnRequest  = "on_request"
	HookOnResponse = "on_response"
)

type LuaEngine struct {
	pool      sync.Pool
	pluginDir string
	scripts   map[string]*lua.FunctionProto
	scriptsMu sync.RWMutex
	enabled   bool

	intelConfig         config.IntelligenceConfig
	classifier          Classifier
	enabledPlugins      []string
	cache               map[string]string
	cacheMu             sync.RWMutex
	intelligenceService IntelligenceService

	// Internal cache for expensive operations like scanning
	scanCache sync.Map
}

// SkillDefinition represents a parsed SKILL.md
type SkillDefinition struct {
	Name               string `yaml:"name"`
	Description        string `yaml:"description"`
	RequiredCapability string `yaml:"required-capability"`
	Content            string `yaml:"-"` // Full content of SKILL.md
}

type Config struct {
	// Enabled determines if the plugin engine is active
	Enabled bool `yaml:"enabled" json:"enabled"`
	// PluginDir is the directory containing LUA scripts
	PluginDir string `yaml:"plugin-dir" json:"plugin-dir"`
	// Intelligence holds settings for the Cortex routing engine
	Intelligence config.IntelligenceConfig
	// EnabledPlugins specifies a list of plugin IDs to load
	EnabledPlugins []string
}

// NewLuaEngine creates a new LUA plugin engine with the given configuration.
func NewLuaEngine(cfg Config) *LuaEngine {
	if !cfg.Enabled {
		return &LuaEngine{enabled: false}
	}

	engine := &LuaEngine{
		pluginDir:      cfg.PluginDir,
		scripts:        make(map[string]*lua.FunctionProto),
		enabled:        true,
		intelConfig:    cfg.Intelligence,
		enabledPlugins: cfg.EnabledPlugins,
		cache:          make(map[string]string),
	}

	engine.pool = sync.Pool{
		New: func() interface{} {
			// SECURITY: Restrict standard libraries to prevent RCE
			L := lua.NewState(lua.Options{
				SkipOpenLibs: true,
			})

			// Manually load ONLY safe libraries
			lua.OpenBase(L)    // Basic functions (assert, error, pairs, etc.)
			lua.OpenTable(L)   // Table manipulation
			lua.OpenString(L)  // String manipulation
			lua.OpenMath(L)    // Math functions
			lua.OpenPackage(L) // Package require support
			// lua.OpenOS(L)    <-- EXPLICITLY DISABLED (unsafe)

			// Provide a safe subset of OS library (date/time only)
			osTbl := L.NewTable()
			L.SetField(osTbl, "date", L.NewFunction(func(L *lua.LState) int {
				format := L.OptString(1, "%c")
				t := time.Now()
				if L.GetTop() >= 2 {
					t = time.Unix(int64(L.CheckNumber(2)), 0)
				}
				L.Push(lua.LString(t.Format(util.LuaDateFormatToGo(format))))
				return 1
			}))
			L.SetField(osTbl, "time", L.NewFunction(func(L *lua.LState) int {
				L.Push(lua.LNumber(time.Now().Unix()))
				return 1
			}))
			L.SetGlobal("os", osTbl)

			// Additional security: Nuke dangerous global functions that OpenBase might have added
			// if they exist (though SkipOpenLibs usually handles this, we double check)
			L.SetGlobal("dofile", lua.LNil)
			L.SetGlobal("loadfile", lua.LNil)

			// Register switchai global module
			engine.registerSwitchAIModule(L)

			return L
		},
	}

	// Load scripts from plugin directory
	if cfg.PluginDir != "" {
		if err := engine.LoadPlugins(); err != nil {
			log.Warnf("failed to load LUA plugins from %s: %v", cfg.PluginDir, err)
		}
	}

	return engine
}

// IsEnabled returns whether the LUA engine is enabled.
func (e *LuaEngine) IsEnabled() bool {
	return e != nil && e.enabled
}

// getState retrieves a LUA state from the pool.
func (e *LuaEngine) getState() *lua.LState {
	return e.pool.Get().(*lua.LState)
}

// putState returns a LUA state to the pool.
func (e *LuaEngine) putState(L *lua.LState) {
	// Clear the stack before returning to pool
	L.SetTop(0)
	e.pool.Put(L)
}

// LoadPlugins loads all plugin directories from the plugin dir.
func (e *LuaEngine) LoadPlugins() error {
	if e.pluginDir == "" {
		return nil
	}

	// Ensure plugin directory exists
	if _, err := os.Stat(e.pluginDir); os.IsNotExist(err) {
		log.Debugf("plugin directory %s does not exist, skipping", e.pluginDir)
		return nil
	}

	entries, err := os.ReadDir(e.pluginDir)
	if err != nil {
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	if len(e.enabledPlugins) == 0 {
		log.Debug("no plugins explicitly enabled, skipping discovery")
		return nil
	}

	for _, entry := range entries {
		// We now look for DIRECTORIES, not files
		if !entry.IsDir() {
			continue
		}

		pluginID := entry.Name()

		// Validate Plugin ID (Slug only)
		// This regex ensures no spaces, special chars, etc.
		if !util.IsValidPluginID(pluginID) {
			log.Warnf("skipping plugin with invalid directory name '%s' (must be slug-style)", pluginID)
			continue
		}

		// Check if enabled
		enabled := false
		for _, id := range e.enabledPlugins {
			if id == pluginID {
				enabled = true
				break
			}
		}

		if !enabled {
			continue
		}

		if err := e.loadPlugin(pluginID); err != nil {
			log.Warnf("failed to load plugin %s: %v", pluginID, err)
			continue
		}
	}

	return nil
}

// loadPlugin loads a plugin from its directory (schema.lua + handler.lua)
func (e *LuaEngine) loadPlugin(pluginID string) error {
	pluginPath := filepath.Join(e.pluginDir, pluginID)
	schemaPath := filepath.Join(pluginPath, "schema.lua")
	handlerPath := filepath.Join(pluginPath, "handler.lua")

	// 1. Load Schema (Metadata)
	L := e.getState()
	defer e.putState(L)

	// We set package.path so 'require("schema")' works inside handler.lua
	// This adds the plugin directory to the search path for this load
	pathLVar := L.GetField(L.GetGlobal("package"), "path")
	oldPath := ""
	if str, ok := pathLVar.(lua.LString); ok {
		oldPath = string(str)
	}
	newPath := fmt.Sprintf("%s/?.lua;%s", pluginPath, oldPath)
	if err := L.DoString(fmt.Sprintf("package.path = [[%s]]", newPath)); err != nil {
		return fmt.Errorf("failed to set package.path: %w", err)
	}

	// Load Schema
	if err := L.DoFile(schemaPath); err != nil {
		return fmt.Errorf("failed to load schema.lua: %w", err)
	}
	schemaTbl := L.Get(-1) // The return value of schema.lua
	if schemaTbl.Type() != lua.LTTable {
		return fmt.Errorf("schema.lua must return a table")
	}

	// Validate Identity
	nameL := L.GetField(schemaTbl, "name")
	if nameL.String() != pluginID {
		return fmt.Errorf("schema.name ('%s') does not match folder name ('%s')", nameL.String(), pluginID)
	}
	displayName := L.GetField(schemaTbl, "display_name").String()
	log.Infof("loading plugin: %s (%s)", displayName, pluginID)

	// 2. Load Handler (Logic)
	// We compile the handler and store its proto
	handlerContent, err := os.ReadFile(handlerPath)
	if err != nil {
		return fmt.Errorf("failed to read handler.lua: %w", err)
	}

	fn, err := L.LoadString(string(handlerContent))
	if err != nil {
		return fmt.Errorf("failed to compile handler.lua: %w", err)
	}

	e.scriptsMu.Lock()
	defer e.scriptsMu.Unlock()
	e.scripts[pluginID] = fn.Proto

	return nil
}

// RunHook executes a specific hook function across all loaded plugins.
// Returns the modified data or the original if no modifications were made.
func (e *LuaEngine) RunHook(ctx context.Context, hookName string, data map[string]any) (map[string]any, error) {
	if !e.enabled || len(e.scripts) == 0 {
		return data, nil
	}

	e.scriptsMu.RLock()
	defer e.scriptsMu.RUnlock()

	result := data
	for scriptName, proto := range e.scripts {
		modified, err := e.executeHook(ctx, scriptName, proto, hookName, result)
		if err != nil {
			log.Debugf("hook %s in %s returned error: %v", hookName, scriptName, err)
			continue
		}
		if modified != nil {
			result = modified
		}
	}

	return result, nil
}

// executeHook runs a single hook function from a compiled script.
func (e *LuaEngine) executeHook(ctx context.Context, scriptName string, proto *lua.FunctionProto, hookName string, data map[string]any) (map[string]any, error) {
	L := e.getState()
	defer e.putState(L)

	// Set context for script execution immediately
	// This is CRITICAL because the pooled state might hold a stale/canceled context from a previous request.
	L.SetContext(ctx)

	// Set package.path for this specific plugin so it can require() its own files
	pluginPath := filepath.Join(e.pluginDir, scriptName)
	pathLVar := L.GetField(L.GetGlobal("package"), "path")
	oldPath := ""
	if str, ok := pathLVar.(lua.LString); ok {
		oldPath = string(str)
	}
	// Add plugin dir to path
	newPath := fmt.Sprintf("%s/?.lua;%s", pluginPath, oldPath)
	if err := L.DoString(fmt.Sprintf("package.path = [[%s]]", newPath)); err != nil {
		log.Errorf("failed to set package.path in hook: %v", err)
	}

	// Load the compiled script (handler.lua)
	// It returns a table (the Plugin object)
	fn := L.NewFunctionFromProto(proto)
	L.Push(fn)
	// PCall the main chunk to get the Plugin table
	if err := L.PCall(0, 1, nil); err != nil {
		return nil, fmt.Errorf("failed to load handler: %w", err)
	}
	pluginTbl := L.Get(-1)
	L.Pop(1) // Pop the table

	if pluginTbl.Type() != lua.LTTable {
		log.Debugf("plugin %s handler did not return a table, falling back to global scope", scriptName)
	}

	// Look for the hook function
	// 1. Try method on Plugin Table: Plugin:on_request(req)
	//    In Lua: Plugin["on_request"](Plugin, req)
	var hookFn lua.LValue
	if pluginTbl.Type() == lua.LTTable {
		hookFn = L.GetField(pluginTbl, hookName)
	} else {
		// Global fallback
		hookFn = L.GetGlobal(hookName)
	}

	if hookFn == lua.LNil || hookFn.Type() != lua.LTFunction {
		return nil, nil // Hook not implemented
	}

	// Convert Go map to Lua table
	luaData := e.goMapToLuaTable(L, data)

	// Call the function
	L.Push(hookFn)

	nArgs := 1
	if pluginTbl.Type() == lua.LTTable {
		// Pass 'self' (the plugin table) as first argument -> :CallingConvention
		L.Push(pluginTbl)
		L.Push(luaData)
		nArgs = 2
	} else {
		L.Push(luaData)
	}

	if err := L.PCall(nArgs, 1, nil); err != nil {
		return nil, fmt.Errorf("hook %s failed: %w", hookName, err)
	}

	// Get the result
	result := L.Get(-1)
	L.Pop(1)

	if result == lua.LNil {
		return nil, nil
	}

	if tbl, ok := result.(*lua.LTable); ok {
		return e.luaTableToGoMap(tbl), nil
	}

	return nil, nil
}

// goMapToLuaTable converts a Go map to a Lua table.
func (e *LuaEngine) goMapToLuaTable(L *lua.LState, m map[string]any) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range m {
		L.SetField(tbl, k, e.goValueToLua(L, v))
	}
	return tbl
}

// goValueToLua converts a Go value to a Lua value.
func (e *LuaEngine) goValueToLua(L *lua.LState, v any) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case int:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []any:
		tbl := L.NewTable()
		for i, item := range val {
			L.RawSetInt(tbl, i+1, e.goValueToLua(L, item))
		}
		return tbl
	case map[string]any:
		return e.goMapToLuaTable(L, val)
	case json.RawMessage:
		return lua.LString(string(val))
	default:
		// Try JSON encoding for unknown types
		if b, err := json.Marshal(val); err == nil {
			return lua.LString(string(b))
		}
		return lua.LString(fmt.Sprintf("%v", val))
	}
}

// luaTableToGoMap converts a Lua table to a Go map.
func (e *LuaEngine) luaTableToGoMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(key lua.LValue, value lua.LValue) {
		if keyStr, ok := key.(lua.LString); ok {
			result[string(keyStr)] = e.luaValueToGo(value)
		}
	})
	return result
}

// luaValueToGo converts a Lua value to a Go value.
func (e *LuaEngine) luaValueToGo(v lua.LValue) any {
	switch val := v.(type) {
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		// Check if it's an array or map
		isArray := true
		maxIdx := 0
		val.ForEach(func(k, _ lua.LValue) {
			if num, ok := k.(lua.LNumber); ok {
				idx := int(num)
				if idx > maxIdx {
					maxIdx = idx
				}
			} else {
				isArray = false
			}
		})

		if isArray && maxIdx > 0 {
			arr := make([]any, maxIdx)
			val.ForEach(func(k, v lua.LValue) {
				if num, ok := k.(lua.LNumber); ok {
					idx := int(num) - 1
					if idx >= 0 && idx < len(arr) {
						arr[idx] = e.luaValueToGo(v)
					}
				}
			})
			return arr
		}
		return e.luaTableToGoMap(val)
	default:
		return nil
	}
}

// Close shuts down the LUA engine and cleans up resources.
func (e *LuaEngine) Close() {
	// Pool items are garbage collected automatically
	e.scriptsMu.Lock()
	e.scripts = nil
	e.scriptsMu.Unlock()
	e.enabled = false
}

// SetClassifier sets the classifier implementation for the engine.
func (e *LuaEngine) SetClassifier(c Classifier) {
	if e == nil {
		return
	}
	e.classifier = c
}

// SetIntelligenceService sets the intelligence service implementation for the engine.
// This allows Lua plugins to access Phase 2 intelligent routing features.
func (e *LuaEngine) SetIntelligenceService(svc IntelligenceService) {
	if e == nil {
		return
	}
	e.intelligenceService = svc
}

// registerSwitchAIModule registers the 'switchai' global table with host functions.
func (e *LuaEngine) registerSwitchAIModule(L *lua.LState) {
	mod := L.NewTable()

	// switchai.classify(prompt) -> response_json
	L.SetField(mod, "classify", L.NewFunction(func(L *lua.LState) int {
		prompt := L.CheckString(1)
		if e.classifier == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("classifier not configured"))
			return 2
		}

		ctx := L.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		res, err := e.classifier.Classify(ctx, prompt)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LString(res))
		return 1
	}))

	// switchai.log(message)
	L.SetField(mod, "log", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		log.Infof("[LUA] %s", msg)
		return 0
	}))

	// switchai.exec(cmd, args...) -> (output, err)
	L.SetField(mod, "exec", L.NewFunction(func(L *lua.LState) int {
		if L.GetTop() < 1 {
			L.Push(lua.LNil)
			L.Push(lua.LString("missing command"))
			return 2
		}
		cmdName := L.CheckString(1)

		// ALLOWLIST (Strict Sandbox)
		allowed := map[string]bool{
			"ls": true, "cat": true, "echo": true, "date": true,
			"git": true, "whoami": true, "pwd": true, "grep": true,
		}
		if !allowed[cmdName] {
			L.Push(lua.LNil)
			L.Push(lua.LString("command not allowed: " + cmdName))
			return 2
		}

		// Git Safety Check
		var args []string
		for i := 2; i <= L.GetTop(); i++ {
			args = append(args, L.CheckString(i))
		}

		if cmdName == "git" {
			if len(args) > 0 {
				sub := args[0]
				// Only allow read-only git operations
				safeGit := map[string]bool{
					"status": true, "diff": true, "log": true, "show": true,
					"branch": true, "ls-files": true, "grep": true,
				}
				if !safeGit[sub] {
					L.Push(lua.LNil)
					L.Push(lua.LString("git subcommand not allowed: " + sub))
					return 2
				}
			}
		}

		ctx := L.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		// Hard timeout for safety
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, cmdName, args...)

		// Capture combined output
		out, err := cmd.CombinedOutput()
		output := string(out)

		// Truncate if too huge to prevent context explosion
		const maxOutputSize = 8192 // 8KB
		if len(output) > maxOutputSize {
			output = output[:maxOutputSize] + "\n...[truncated (output too large)]"
		}

		L.Push(lua.LString(output))
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 2
		}
		return 1
	}))

	// switchai.get_cache(key) -> value
	L.SetField(mod, "get_cache", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		e.cacheMu.RLock()
		val, ok := e.cache[key]
		e.cacheMu.RUnlock()
		if !ok {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(val))
		}
		return 1
	}))

	// switchai.set_cache(key, value)
	L.SetField(mod, "set_cache", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := L.CheckString(2)
		e.cacheMu.Lock()
		// Simple cap at 1000 items
		if len(e.cache) > 1000 {
			// Clear all (simplest LRU-ish behavior)
			e.cache = make(map[string]string)
		}
		e.cache[key] = val
		e.cacheMu.Unlock()
		return 0
	}))

	// switchai.config
	configTbl := L.NewTable()
	L.SetField(configTbl, "router_model", lua.LString(e.intelConfig.RouterModel))
	L.SetField(configTbl, "router_fallback", lua.LString(e.intelConfig.RouterFallback))
	L.SetField(configTbl, "skills_path", lua.LString(e.intelConfig.SkillsPath))

	matrixTbl := L.NewTable()
	for k, v := range e.intelConfig.Matrix {
		L.SetField(matrixTbl, k, lua.LString(v))
	}
	L.SetField(configTbl, "matrix", matrixTbl)
	L.SetField(mod, "config", configTbl)

	// switchai.scan_skills(path) -> table
	L.SetField(mod, "scan_skills", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		skills, err := e.scanSkills(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		skillsTbl := L.NewTable()
		for _, skill := range skills {
			s := L.NewTable()
			L.SetField(s, "name", lua.LString(skill.Name))
			L.SetField(s, "description", lua.LString(skill.Description))
			L.SetField(s, "required_capability", lua.LString(skill.RequiredCapability))
			L.SetField(s, "content", lua.LString(skill.Content))
			L.SetField(skillsTbl, skill.Name, s)
		}
		L.Push(skillsTbl)
		return 1
	}))

	// switchai.get_skills() -> table or nil, error
	// Returns skills from the enhanced skill registry (Phase 2)
	L.SetField(mod, "get_skills", L.NewFunction(func(L *lua.LState) int {
		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get skill registry
		registry := e.intelligenceService.GetSkillRegistry()
		if registry == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("skill registry not available"))
			return 2
		}

		// Get all skills
		skills := registry.GetAllSkills()

		// Convert to Lua table
		skillsTbl := L.NewTable()
		for _, skill := range skills {
			s := L.NewTable()
			L.SetField(s, "id", lua.LString(skill.GetID()))
			L.SetField(s, "name", lua.LString(skill.GetName()))
			L.SetField(s, "description", lua.LString(skill.GetDescription()))
			L.SetField(s, "required_capability", lua.LString(skill.GetRequiredCapability()))
			L.SetField(s, "system_prompt", lua.LString(skill.GetSystemPrompt()))
			L.SetField(s, "has_embedding", lua.LBool(skill.GetEmbeddingLength() > 0))
			L.SetField(skillsTbl, skill.GetID(), s)
		}

		// Add metadata
		metaTbl := L.NewTable()
		L.SetField(metaTbl, "count", lua.LNumber(registry.GetSkillCount()))
		L.SetField(metaTbl, "embeddings_available", lua.LBool(registry.HasEmbeddings()))
		L.SetField(skillsTbl, "_meta", metaTbl)

		L.Push(skillsTbl)
		return 1
	}))

	// switchai.json_inject(json_str, content_to_inject) -> new_json
	// Safe injection using encoding/json
	L.SetField(mod, "json_inject", L.NewFunction(func(L *lua.LState) int {
		jsonStr := L.CheckString(1)
		injection := L.CheckString(2)

		var payload map[string]interface{}
		// If unmarshal fails, return original string (graceful fallback)
		if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
			log.Warnf("LUA json_inject: invalid JSON, fallback to append: %v", err)
			L.Push(lua.LString(jsonStr + "\n\n" + injection))
			return 1
		}

		// Inject as System Message
		// Check for "messages" array
		if messages, ok := payload["messages"].([]interface{}); ok {
			sysMsg := map[string]interface{}{
				"role":    "system",
				"content": injection,
			}
			// Prepend system message
			newMessages := append([]interface{}{sysMsg}, messages...)
			payload["messages"] = newMessages
		}

		// Marshal back
		newBytes, err := json.Marshal(payload)
		if err != nil {
			L.Push(lua.LString(jsonStr))
			return 1
		}

		L.Push(lua.LString(string(newBytes)))
		return 1
	}))

	// switchai.get_available_models() -> table or nil, error
	// Returns discovered models from the intelligence discovery service
	L.SetField(mod, "get_available_models", L.NewFunction(func(L *lua.LState) int {
		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get discovery service
		discoverySvc := e.intelligenceService.GetDiscoveryService()
		if discoverySvc == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("discovery service not available"))
			return 2
		}

		// Get available models as maps
		models := discoverySvc.GetAvailableModelsAsMap()

		// Convert to Lua table
		modelsTbl := L.NewTable()
		for i, model := range models {
			modelTbl := L.NewTable()
			for k, v := range model {
				switch val := v.(type) {
				case string:
					L.SetField(modelTbl, k, lua.LString(val))
				case bool:
					L.SetField(modelTbl, k, lua.LBool(val))
				case int:
					L.SetField(modelTbl, k, lua.LNumber(val))
				case float64:
					L.SetField(modelTbl, k, lua.LNumber(val))
				case map[string]interface{}:
					// Handle nested capabilities map
					capTbl := L.NewTable()
					for ck, cv := range val {
						switch cval := cv.(type) {
						case string:
							L.SetField(capTbl, ck, lua.LString(cval))
						case bool:
							L.SetField(capTbl, ck, lua.LBool(cval))
						case int:
							L.SetField(capTbl, ck, lua.LNumber(cval))
						case float64:
							L.SetField(capTbl, ck, lua.LNumber(cval))
						}
					}
					L.SetField(modelTbl, k, capTbl)
				}
			}
			L.RawSetInt(modelsTbl, i+1, modelTbl)
		}

		L.Push(modelsTbl)
		return 1
	}))

	// switchai.get_dynamic_matrix() -> table or nil, error
	// Returns the dynamic capability matrix from the intelligence service
	L.SetField(mod, "get_dynamic_matrix", L.NewFunction(func(L *lua.LState) int {
		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get matrix builder
		matrixBuilder := e.intelligenceService.GetMatrixBuilder()
		if matrixBuilder == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("dynamic matrix builder not available"))
			return 2
		}

		// Get current matrix as map
		matrixMap := matrixBuilder.GetCurrentMatrixAsMap()
		if matrixMap == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("no matrix has been built yet"))
			return 2
		}

		// Convert to Lua table
		matrixTbl := L.NewTable()
		for slot, assignment := range matrixMap {
			if assignMap, ok := assignment.(map[string]interface{}); ok {
				slotTbl := L.NewTable()

				// Set primary
				if primary, ok := assignMap["primary"].(string); ok {
					L.SetField(slotTbl, "primary", lua.LString(primary))
				}

				// Set score
				if score, ok := assignMap["score"].(float64); ok {
					L.SetField(slotTbl, "score", lua.LNumber(score))
				}

				// Set reason
				if reason, ok := assignMap["reason"].(string); ok {
					L.SetField(slotTbl, "reason", lua.LString(reason))
				}

				// Set fallbacks as array
				if fallbacks, ok := assignMap["fallbacks"].([]string); ok {
					fallbacksTbl := L.NewTable()
					for i, fb := range fallbacks {
						L.RawSetInt(fallbacksTbl, i+1, lua.LString(fb))
					}
					L.SetField(slotTbl, "fallbacks", fallbacksTbl)
				}

				L.SetField(matrixTbl, slot, slotTbl)
			}
		}

		L.Push(matrixTbl)
		return 1
	}))

	// switchai.is_model_available(model_id) -> bool
	// Checks if a model is in the discovered models
	L.SetField(mod, "is_model_available", L.NewFunction(func(L *lua.LState) int {
		modelID := L.CheckString(1)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LBool(false))
			return 1
		}

		// Check if model is available
		available := e.intelligenceService.IsModelAvailable(modelID)
		L.Push(lua.LBool(available))
		return 1
	}))

	// switchai.embed(text) -> table (embedding) or nil, error
	// Computes a 384-dimensional embedding vector for the given text
	L.SetField(mod, "embed", L.NewFunction(func(L *lua.LState) int {
		text := L.CheckString(1)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get embedding engine
		engine := e.intelligenceService.GetEmbeddingEngine()
		if engine == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("embedding engine not available"))
			return 2
		}

		// Compute embedding
		embeddingVec, err := engine.Embed(text)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Convert to Lua table (array of numbers)
		embeddingTbl := L.NewTable()
		for i, val := range embeddingVec {
			L.RawSetInt(embeddingTbl, i+1, lua.LNumber(val))
		}

		L.Push(embeddingTbl)
		return 1
	}))

	// switchai.cosine_similarity(a, b) -> number
	// Computes the cosine similarity between two embedding vectors
	L.SetField(mod, "cosine_similarity", L.NewFunction(func(L *lua.LState) int {
		aTbl := L.CheckTable(1)
		bTbl := L.CheckTable(2)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNumber(0))
			return 1
		}

		// Get embedding engine
		engine := e.intelligenceService.GetEmbeddingEngine()
		if engine == nil {
			L.Push(lua.LNumber(0))
			return 1
		}

		// Convert Lua tables to float32 slices
		a := make([]float32, 0)
		b := make([]float32, 0)

		aTbl.ForEach(func(_, v lua.LValue) {
			if num, ok := v.(lua.LNumber); ok {
				a = append(a, float32(num))
			}
		})

		bTbl.ForEach(func(_, v lua.LValue) {
			if num, ok := v.(lua.LNumber); ok {
				b = append(b, float32(num))
			}
		})

		// Compute cosine similarity
		similarity := engine.CosineSimilarity(a, b)
		L.Push(lua.LNumber(similarity))
		return 1
	}))

	// switchai.semantic_match_intent(text) -> table {intent, confidence} or nil, error
	// Matches a query to the best matching intent using semantic similarity
	L.SetField(mod, "semantic_match_intent", L.NewFunction(func(L *lua.LState) int {
		text := L.CheckString(1)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get semantic tier
		semanticTier := e.intelligenceService.GetSemanticTier()
		if semanticTier == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic tier not available"))
			return 2
		}

		if !semanticTier.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic tier not initialized"))
			return 2
		}

		// Match intent
		result, err := semanticTier.MatchIntent(text)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// No match above threshold
		if result == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Convert result to Lua table
		resultTbl := L.NewTable()
		L.SetField(resultTbl, "intent", lua.LString(result.Intent))
		L.SetField(resultTbl, "confidence", lua.LNumber(result.Confidence))
		L.SetField(resultTbl, "latency_ms", lua.LNumber(result.LatencyMs))

		L.Push(resultTbl)
		return 1
	}))

	// switchai.match_skill(text) -> table {skill, confidence} or nil, error
	// Matches a query to the best matching skill using semantic similarity
	L.SetField(mod, "match_skill", L.NewFunction(func(L *lua.LState) int {
		text := L.CheckString(1)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get skill registry
		skillRegistry := e.intelligenceService.GetSkillRegistry()
		if skillRegistry == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("skill registry not available"))
			return 2
		}

		// Get embedding engine
		embeddingEngine := e.intelligenceService.GetEmbeddingEngine()
		if embeddingEngine == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("embedding engine not available"))
			return 2
		}

		// Compute query embedding
		queryEmbedding, err := embeddingEngine.Embed(text)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(fmt.Sprintf("failed to compute embedding: %v", err)))
			return 2
		}

		// Match skill
		result, err := skillRegistry.MatchSkill(queryEmbedding)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// No match above threshold
		if result == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Convert result to Lua table
		resultTbl := L.NewTable()

		// Create skill table
		skillTbl := L.NewTable()
		L.SetField(skillTbl, "id", lua.LString(result.Skill.ID))
		L.SetField(skillTbl, "name", lua.LString(result.Skill.Name))
		L.SetField(skillTbl, "description", lua.LString(result.Skill.Description))
		L.SetField(skillTbl, "required_capability", lua.LString(result.Skill.RequiredCapability))
		L.SetField(skillTbl, "system_prompt", lua.LString(result.Skill.SystemPrompt))

		L.SetField(resultTbl, "skill", skillTbl)
		L.SetField(resultTbl, "confidence", lua.LNumber(result.Confidence))

		L.Push(resultTbl)
		return 1
	}))

	// switchai.cache_lookup(query) -> table {decision, metadata} or nil, error
	// Looks up a cached routing decision based on semantic similarity
	L.SetField(mod, "cache_lookup", L.NewFunction(func(L *lua.LState) int {
		query := L.CheckString(1)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get semantic cache
		cacheInterface := e.intelligenceService.GetSemanticCache()
		if cacheInterface == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic cache not available"))
			return 2
		}

		if !cacheInterface.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic cache not initialized"))
			return 2
		}

		// Lookup in cache
		result, err := cacheInterface.Lookup(query)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(fmt.Sprintf("cache lookup failed: %v", err)))
			return 2
		}

		// Cache miss
		if result == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Convert result to Lua table
		// The result is a *cache.CacheEntry, extract fields using reflection
		resultTbl := L.NewTable()

		v := reflect.ValueOf(result)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			// Get Decision field
			if decisionField := v.FieldByName("Decision"); decisionField.IsValid() {
				L.SetField(resultTbl, "decision", lua.LString(decisionField.String()))
			}

			// Get Metadata field
			if metadataField := v.FieldByName("Metadata"); metadataField.IsValid() {
				if metadata, ok := metadataField.Interface().(map[string]interface{}); ok && metadata != nil {
					metadataTbl := L.NewTable()
					for k, val := range metadata {
						switch v := val.(type) {
						case string:
							L.SetField(metadataTbl, k, lua.LString(v))
						case float64:
							L.SetField(metadataTbl, k, lua.LNumber(v))
						case bool:
							L.SetField(metadataTbl, k, lua.LBool(v))
						default:
							jsonBytes, _ := json.Marshal(val)
							L.SetField(metadataTbl, k, lua.LString(string(jsonBytes)))
						}
					}
					L.SetField(resultTbl, "metadata", metadataTbl)
				}
			}
		}

		L.Push(resultTbl)
		return 1
	}))

	// switchai.cache_store(query, decision, metadata) -> nil, error
	// Stores a routing decision in the semantic cache
	L.SetField(mod, "cache_store", L.NewFunction(func(L *lua.LState) int {
		query := L.CheckString(1)
		decision := L.CheckString(2)
		metadataTable := L.OptTable(3, nil)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get semantic cache
		cache := e.intelligenceService.GetSemanticCache()
		if cache == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic cache not available"))
			return 2
		}

		if !cache.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic cache not initialized"))
			return 2
		}

		// Get embedding engine
		embeddingEngine := e.intelligenceService.GetEmbeddingEngine()
		if embeddingEngine == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("embedding engine not available"))
			return 2
		}

		// Compute query embedding
		queryEmbedding, err := embeddingEngine.Embed(query)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(fmt.Sprintf("failed to compute embedding: %v", err)))
			return 2
		}

		// Convert Lua metadata table to Go map
		metadata := make(map[string]interface{})
		if metadataTable != nil {
			metadataTable.ForEach(func(k, v lua.LValue) {
				key := k.String()
				switch val := v.(type) {
				case lua.LString:
					metadata[key] = string(val)
				case lua.LNumber:
					metadata[key] = float64(val)
				case lua.LBool:
					metadata[key] = bool(val)
				default:
					metadata[key] = v.String()
				}
			})
		}

		// Store in cache
		if err := cache.Store(query, queryEmbedding, decision, metadata); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(fmt.Sprintf("cache store failed: %v", err)))
			return 2
		}

		L.Push(lua.LNil)
		return 1
	}))

	// switchai.cache_metrics() -> table {hits, misses, size, hit_rate} or nil, error
	// Returns cache performance metrics
	L.SetField(mod, "cache_metrics", L.NewFunction(func(L *lua.LState) int {
		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get semantic cache
		cache := e.intelligenceService.GetSemanticCache()
		if cache == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic cache not available"))
			return 2
		}

		if !cache.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("semantic cache not initialized"))
			return 2
		}

		// Get metrics
		metrics := cache.GetMetricsAsMap()

		// Convert to Lua table
		metricsTbl := L.NewTable()
		for k, v := range metrics {
			switch val := v.(type) {
			case int64:
				L.SetField(metricsTbl, k, lua.LNumber(val))
			case int:
				L.SetField(metricsTbl, k, lua.LNumber(val))
			case float64:
				L.SetField(metricsTbl, k, lua.LNumber(val))
			case string:
				L.SetField(metricsTbl, k, lua.LString(val))
			default:
				L.SetField(metricsTbl, k, lua.LString(fmt.Sprintf("%v", val)))
			}
		}

		L.Push(metricsTbl)
		return 1
	}))

	// switchai.parse_confidence(json_str) -> table {intent, complexity, confidence} or nil, error
	L.SetField(mod, "parse_confidence", L.NewFunction(func(L *lua.LState) int {
		jsonStr := L.CheckString(1)

		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		scorer := e.intelligenceService.GetConfidenceScorer()
		if scorer == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("confidence scorer not available"))
			return 2
		}

		res, err := scorer.Parse(jsonStr)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		resTbl := L.NewTable()
		L.SetField(resTbl, "intent", lua.LString(res.Intent))
		L.SetField(resTbl, "complexity", lua.LString(res.Complexity))
		L.SetField(resTbl, "confidence", lua.LNumber(res.Confidence))
		L.Push(resTbl)
		return 1
	}))

	// switchai.verify_intent(tier1_intent, tier2_intent) -> bool
	L.SetField(mod, "verify_intent", L.NewFunction(func(L *lua.LState) int {
		t1 := L.CheckString(1)
		t2 := L.CheckString(2)

		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LBool(t1 == t2)) // Fallback to simple comparison
			return 1
		}

		verifier := e.intelligenceService.GetVerifier()
		if verifier == nil {
			L.Push(lua.LBool(t1 == t2))
			return 1
		}

		match := verifier.Verify(t1, t2)
		L.Push(lua.LBool(match))
		return 1
	}))

	// switchai.evaluate_response(response, current_tier) -> table {should_cascade, next_tier, quality_score, reason, signals} or nil, error
	// Evaluates a response for quality and determines if cascade is needed
	L.SetField(mod, "evaluate_response", L.NewFunction(func(L *lua.LState) int {
		response := L.CheckString(1)
		currentTier := L.CheckString(2)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get cascade manager
		cascadeManager := e.intelligenceService.GetCascadeManager()
		if cascadeManager == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("cascade manager not available"))
			return 2
		}

		if !cascadeManager.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("cascade manager not enabled"))
			return 2
		}

		// Evaluate response
		decision := cascadeManager.EvaluateResponse(response, currentTier)
		if decision == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Convert to Lua table
		resultTbl := L.NewTable()
		L.SetField(resultTbl, "should_cascade", lua.LBool(decision.ShouldCascade))
		L.SetField(resultTbl, "current_tier", lua.LString(decision.CurrentTier))
		L.SetField(resultTbl, "next_tier", lua.LString(decision.NextTier))
		L.SetField(resultTbl, "quality_score", lua.LNumber(decision.QualityScore))
		L.SetField(resultTbl, "reason", lua.LString(decision.Reason))

		// Convert signals to Lua table
		signalsTbl := L.NewTable()
		for i, signal := range decision.Signals {
			signalTbl := L.NewTable()
			L.SetField(signalTbl, "type", lua.LString(signal.Type))
			L.SetField(signalTbl, "severity", lua.LNumber(signal.Severity))
			L.SetField(signalTbl, "description", lua.LString(signal.Description))
			L.RawSetInt(signalsTbl, i+1, signalTbl)
		}
		L.SetField(resultTbl, "signals", signalsTbl)

		L.Push(resultTbl)
		return 1
	}))

	// switchai.get_cascade_metrics() -> table or nil, error
	// Returns cascade performance metrics
	L.SetField(mod, "get_cascade_metrics", L.NewFunction(func(L *lua.LState) int {
		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get cascade manager
		cascadeManager := e.intelligenceService.GetCascadeManager()
		if cascadeManager == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("cascade manager not available"))
			return 2
		}

		// Get metrics
		metrics := cascadeManager.GetMetricsAsMap()

		// Convert to Lua table
		metricsTbl := L.NewTable()
		for k, v := range metrics {
			switch val := v.(type) {
			case int64:
				L.SetField(metricsTbl, k, lua.LNumber(val))
			case int:
				L.SetField(metricsTbl, k, lua.LNumber(val))
			case float64:
				L.SetField(metricsTbl, k, lua.LNumber(val))
			case string:
				L.SetField(metricsTbl, k, lua.LString(val))
			case map[string]int64:
				tierTbl := L.NewTable()
				for tk, tv := range val {
					L.SetField(tierTbl, tk, lua.LNumber(tv))
				}
				L.SetField(metricsTbl, k, tierTbl)
			default:
				L.SetField(metricsTbl, k, lua.LString(fmt.Sprintf("%v", val)))
			}
		}

		L.Push(metricsTbl)
		return 1
	}))

	// switchai.get_intelligence_metrics() -> table
	L.SetField(mod, "get_intelligence_metrics", L.NewFunction(func(L *lua.LState) int {
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			return 1
		}

		metrics := L.NewTable()

		if scorer := e.intelligenceService.GetConfidenceScorer(); scorer != nil {
			scorerTbl := L.NewTable()
			for k, v := range scorer.GetMetrics() {
				L.SetField(scorerTbl, k, e.goValueToLua(L, v))
			}
			L.SetField(metrics, "confidence", scorerTbl)
		}

		if verifier := e.intelligenceService.GetVerifier(); verifier != nil {
			verifierTbl := L.NewTable()
			for k, v := range verifier.GetMetrics() {
				L.SetField(verifierTbl, k, e.goValueToLua(L, v))
			}
			L.SetField(metrics, "verification", verifierTbl)
		}

		if cascadeManager := e.intelligenceService.GetCascadeManager(); cascadeManager != nil {
			cascadeTbl := L.NewTable()
			for k, v := range cascadeManager.GetMetricsAsMap() {
				L.SetField(cascadeTbl, k, e.goValueToLua(L, v))
			}
			L.SetField(metrics, "cascade", cascadeTbl)
		}

		L.Push(metrics)
		return 1
	}))

	// switchai.record_feedback(data) -> nil, error
	// Records routing feedback for future learning
	L.SetField(mod, "record_feedback", L.NewFunction(func(L *lua.LState) int {
		dataTable := L.CheckTable(1)

		// Check if intelligence service is enabled
		if e.intelligenceService == nil || !e.intelligenceService.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("intelligence services not enabled"))
			return 2
		}

		// Get feedback collector
		feedbackCollector := e.intelligenceService.GetFeedbackCollector()
		if feedbackCollector == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("feedback collector not available"))
			return 2
		}

		if !feedbackCollector.IsEnabled() {
			L.Push(lua.LNil)
			L.Push(lua.LString("feedback collector not initialized"))
			return 2
		}

		// Convert Lua table to feedback record
		record := &intelligence.FeedbackRecord{}

		// Extract fields from Lua table
		if query := L.GetField(dataTable, "query"); query != lua.LNil {
			record.Query = query.String()
		}
		if intent := L.GetField(dataTable, "intent"); intent != lua.LNil {
			record.Intent = intent.String()
		}
		if model := L.GetField(dataTable, "selected_model"); model != lua.LNil {
			record.SelectedModel = model.String()
		}
		if tier := L.GetField(dataTable, "routing_tier"); tier != lua.LNil {
			record.RoutingTier = tier.String()
		}
		if confidence := L.GetField(dataTable, "confidence"); confidence != lua.LNil {
			if num, ok := confidence.(lua.LNumber); ok {
				record.Confidence = float64(num)
			}
		}
		if skill := L.GetField(dataTable, "matched_skill"); skill != lua.LNil {
			record.MatchedSkill = skill.String()
		}
		if cascade := L.GetField(dataTable, "cascade_occurred"); cascade != lua.LNil {
			if b, ok := cascade.(lua.LBool); ok {
				record.CascadeOccurred = bool(b)
			}
		}
		if quality := L.GetField(dataTable, "response_quality"); quality != lua.LNil {
			if num, ok := quality.(lua.LNumber); ok {
				record.ResponseQuality = float64(num)
			}
		}
		if latency := L.GetField(dataTable, "latency_ms"); latency != lua.LNil {
			if num, ok := latency.(lua.LNumber); ok {
				record.LatencyMs = int64(num)
			}
		}
		if success := L.GetField(dataTable, "success"); success != lua.LNil {
			if b, ok := success.(lua.LBool); ok {
				record.Success = bool(b)
			}
		}
		if errMsg := L.GetField(dataTable, "error_message"); errMsg != lua.LNil {
			record.ErrorMessage = errMsg.String()
		}

		// Extract metadata if present
		if metadataField := L.GetField(dataTable, "metadata"); metadataField != lua.LNil {
			if metadataTable, ok := metadataField.(*lua.LTable); ok {
				metadata := make(map[string]interface{})
				metadataTable.ForEach(func(k, v lua.LValue) {
					key := k.String()
					switch val := v.(type) {
					case lua.LString:
						metadata[key] = string(val)
					case lua.LNumber:
						metadata[key] = float64(val)
					case lua.LBool:
						metadata[key] = bool(val)
					default:
						metadata[key] = v.String()
					}
				})
				record.Metadata = metadata
			}
		}

		// Record feedback
		ctx := L.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		if err := feedbackCollector.Record(ctx, record); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(fmt.Sprintf("failed to record feedback: %v", err)))
			return 2
		}

		L.Push(lua.LNil)
		return 1
	}))

	L.SetGlobal("switchai", mod)
}

// scanSkills walks the directory and parses SKILL.md files (Cached)
func (e *LuaEngine) scanSkills(root string) ([]SkillDefinition, error) {
	// 1. Check Cache
	if val, ok := e.scanCache.Load(root); ok {
		log.Debugf("Returning cached skills for %s", root)
		return val.([]SkillDefinition), nil
	}

	log.Infof("Scanning for skills in %s...", root)

	var newSkills []SkillDefinition
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "SKILL.md") {
			content, err := os.ReadFile(path)
			if err != nil {
				log.Warnf("Failed to read SKILL.md at %s: %v", path, err)
				return nil
			}

			// Parse Frontmatter (YAML)
			parts := strings.SplitN(string(content), "---", 3)
			if len(parts) >= 3 {
				var skill SkillDefinition
				if err := yaml.Unmarshal([]byte(parts[1]), &skill); err == nil {
					if skill.Name == "" {
						skill.Name = filepath.Base(filepath.Dir(path))
					}
					skill.Content = string(content)
					newSkills = append(newSkills, skill)
					count++
				} else {
					log.Warnf("Failed to parse SKILL.md frontmatter at %s: %v", path, err)
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 2. Store in Cache
	e.scanCache.Store(root, newSkills)
	log.Infof("Cached %d skills for %s", count, root)

	return newSkills, nil
}
