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
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	log "github.com/sirupsen/logrus"
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

	intelConfig    config.IntelligenceConfig
	classifier     Classifier
	enabledPlugins []string
	cache          map[string]string
	cacheMu        sync.RWMutex

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

	// Set context for script execution
	L.SetContext(ctx)

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
