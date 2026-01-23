// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package plugin

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	lua "github.com/yuin/gopher-lua"
)

func TestLuaSandbox_RestrictedLibs(t *testing.T) {
	cfg := Config{Enabled: true}
	engine := NewLuaEngine(cfg)
	defer engine.Close()

	L := engine.getState()
	defer engine.putState(L)

	// List of dangerous globals that should be nil (excluding os, which is restricted)
	forbidden := []string{"io", "debug", "dofile", "loadfile"}

	for _, name := range forbidden {
		val := L.GetGlobal(name)
		if val != lua.LNil {
			t.Errorf("Security Breach: Global '%s' should be nil, but found %s", name, val.Type())
		}
	}

	// Logic for os: it should exist but be restricted
	osVal := L.GetGlobal("os")
	if osVal == lua.LNil {
		t.Error("Functionality Bug: Global 'os' should be present (restricted)")
	} else if tbl, ok := osVal.(*lua.LTable); ok {
		// Dangerous os functions must be nil
		dangerousOs := []string{"execute", "exit", "remove", "rename", "tmpname", "getenv"}
		for _, fn := range dangerousOs {
			if L.GetField(tbl, fn) != lua.LNil {
				t.Errorf("Security Breach: os.%s should be nil", fn)
			}
		}
		// Allowed os functions should be present
		allowedOs := []string{"date", "time"}
		for _, fn := range allowedOs {
			if L.GetField(tbl, fn) == lua.LNil {
				t.Errorf("Functionality Bug: os.%s should be available", fn)
			}
		}
	} else {
		t.Errorf("Type Mismatch: Global 'os' should be a table, got %s", osVal.Type())
	}
}

func TestLuaSandbox_AllowedLibs(t *testing.T) {
	cfg := Config{Enabled: true}
	engine := NewLuaEngine(cfg)
	defer engine.Close()

	L := engine.getState()
	defer engine.putState(L)

	// List of allowed globals that should be available
	allowed := []string{"math", "string", "table"}

	for _, name := range allowed {
		val := L.GetGlobal(name)
		if val == lua.LNil {
			t.Errorf("Functionality Bug: Global '%s' should be available", name)
		}
	}
}

func TestLuaSandbox_ExecutionAttempts(t *testing.T) {
	cfg := Config{Enabled: true}
	engine := NewLuaEngine(cfg)
	defer engine.Close()

	// Attempt to use os.execute (should ensure the script fails or os is nil)
	// Since os is nil, 'os.execute' will throw a lua error: "attempt to index a nil value"
	script := `
		function unknown_attack(payload)
			if os == nil then
				return "blocked"
			end
			-- os exists, but execute should be missing
			if os.execute == nil then
				return "blocked"
			end
			os.execute("touch /tmp/hacked")
			return "pwned"
		end
	`

	// Manually load script into state for testing since we aren't using file loading here
	// But we can simulate it by doing LoadString
	L := engine.getState()
	defer engine.putState(L)

	if err := L.DoString(script); err != nil {
		t.Fatalf("Failed to load test script: %v", err)
	}

	attackFn := L.GetGlobal("unknown_attack")
	if attackFn.Type() != lua.LTFunction {
		t.Fatal("Failed to define attack function")
	}

	// Execute
	L.Push(attackFn)
	L.Push(lua.LString("test"))
	if err := L.PCall(1, 1, nil); err != nil {
		// It might fail if we accessed os.execute and os is nil.
		// Actually, our script handles os == nil.
		t.Fatalf("Script execution failed: %v", err)
	}

	result := L.Get(-1)
	L.Pop(1)

	if result.String() != "blocked" {
		t.Errorf("Security Breach: Expected 'blocked', got '%s'", result.String())
	}
}

// Property: Lua Sandbox isolation
// **Validates: Phase 1 Security Requirements for Lua Plugin Sandbox**
func TestProperty_LuaSandboxIsolation(t *testing.T) {
	properties := gopter.NewProperties(nil)

	forbidden := []string{"io", "debug", "dofile", "loadfile"}

	properties.Property("forbidden globals are always nil", prop.ForAll(
		func(dummy string) bool {
			cfg := Config{Enabled: true}
			engine := NewLuaEngine(cfg)
			defer engine.Close()

			L := engine.getState()
			defer engine.putState(L)

			for _, name := range forbidden {
				val := L.GetGlobal(name)
				if val != lua.LNil {
					return false
				}
			}
			return true
		},
		gen.AlphaString(),
	))

	properties.Property("attempting to use forbidden globals results in nil error within script", prop.ForAll(
		func(forbiddenName string) bool {
			cfg := Config{Enabled: true}
			engine := NewLuaEngine(cfg)
			defer engine.Close()

			L := engine.getState()
			defer engine.putState(L)

			// Simple script that returns "nil" if the forbidden global is nil, or its type otherwise
			script := "return type(" + forbiddenName + ")"
			if err := L.DoString(script); err != nil {
				// If the script fails to load, it's fine as long as it's because of syntax or missing global
				return true
			}

			result := L.Get(-1)
			return result.String() == "nil"
		},
		gen.OneConstOf("io", "debug", "dofile", "loadfile"),
	))

	properties.TestingRun(t)
}
