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
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/util"
	lua "github.com/yuin/gopher-lua"
)

// Hook types for plugin execution points
const (
	HookOnRequest  = "on_request"
	HookOnResponse = "on_response"
)

// LuaEngine manages LUA script execution with a pool of LUA states for concurrency.
type LuaEngine struct {
	pool      sync.Pool
	pluginDir string
	scripts   map[string]*lua.FunctionProto
	scriptsMu sync.RWMutex
	enabled   bool
}

// Config holds configuration for the LUA plugin engine.
type Config struct {
	// Enabled determines if the plugin engine is active
	Enabled bool `yaml:"enabled" json:"enabled"`
	// PluginDir is the directory containing LUA scripts
	PluginDir string `yaml:"plugin-dir" json:"plugin-dir"`
}

// NewLuaEngine creates a new LUA plugin engine with the given configuration.
func NewLuaEngine(cfg Config) *LuaEngine {
	if !cfg.Enabled {
		return &LuaEngine{enabled: false}
	}

	engine := &LuaEngine{
		pluginDir: cfg.PluginDir,
		scripts:   make(map[string]*lua.FunctionProto),
		enabled:   true,
	}

	engine.pool = sync.Pool{
		New: func() interface{} {
			// SECURITY: Restrict standard libraries to prevent RCE
			L := lua.NewState(lua.Options{
				SkipOpenLibs: true,
			})

			// Manually load ONLY safe libraries
			lua.OpenBase(L)   // Basic functions (assert, error, pairs, etc.)
			lua.OpenTable(L)  // Table manipulation
			lua.OpenString(L) // String manipulation
			lua.OpenMath(L)   // Math functions
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

// LoadPlugins loads all .lua files from the plugin directory.
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

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".lua" {
			continue
		}

		scriptPath := filepath.Join(e.pluginDir, entry.Name())
		if err := e.loadScript(scriptPath); err != nil {
			log.Warnf("failed to load script %s: %v", entry.Name(), err)
			continue
		}
		log.Infof("loaded LUA plugin: %s", entry.Name())
	}

	return nil
}

// loadScript compiles and caches a LUA script.
func (e *LuaEngine) loadScript(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read script: %w", err)
	}

	L := e.getState()
	defer e.putState(L)

	// Compile the script
	fn, err := L.LoadString(string(content))
	if err != nil {
		return fmt.Errorf("failed to compile script: %w", err)
	}

	e.scriptsMu.Lock()
	defer e.scriptsMu.Unlock()
	e.scripts[filepath.Base(path)] = fn.Proto
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

	// Load the compiled script
	fn := L.NewFunctionFromProto(proto)
	L.Push(fn)

	// Set context for script execution to support timeouts
	L.SetContext(ctx)

	if err := L.PCall(0, 0, nil); err != nil {
		return nil, fmt.Errorf("failed to execute script: %w", err)
	}

	// Get the hook function
	hookFn := L.GetGlobal(hookName)
	if hookFn == lua.LNil || hookFn.Type() != lua.LTFunction {
		return nil, nil // Hook not defined in this script
	}

	// Convert Go map to Lua table
	luaData := e.goMapToLuaTable(L, data)

	// Call the hook function
	L.Push(hookFn)
	L.Push(luaData)
	if err := L.PCall(1, 1, nil); err != nil {
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
