package plugin

import (
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// TestCortexHandler_ConfidenceLogic tests the confidence parsing and routing logic in handler.lua
func TestCortexHandler_ConfidenceLogic(t *testing.T) {
	// Setup Lua state
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs() // Load standard libraries (os, string, etc.)

	// 1. Mock 'switchai' global table
	switchai := L.NewTable()
	L.SetGlobal("switchai", switchai)

	// Mock switchai.config
	configTbl := L.NewTable()
	matrix := L.NewTable()
	// Populate required fields
	L.SetField(matrix, "image_gen", lua.LString("mock-image"))
	L.SetField(matrix, "transcription", lua.LString("mock-transcription"))
	L.SetField(matrix, "speech", lua.LString("mock-speech"))
	L.SetField(configTbl, "matrix", matrix)
	L.SetField(switchai, "config", configTbl)

	// Mock switchai.log (noop)
	L.SetField(switchai, "log", L.NewFunction(func(L *lua.LState) int { return 0 }))

	// Mock switchai.get_dynamic_matrix (return nil)
	L.SetField(switchai, "get_dynamic_matrix", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 1
	}))

	// Mock switchai.cache_lookup (return nil)
	L.SetField(switchai, "cache_lookup", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 2
	}))

	// Mock switchai.is_model_available (always true)
	L.SetField(switchai, "is_model_available", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LTrue)
		return 1
	}))

	// Mock switchai.semantic_match_intent
	var mockSemanticResult *lua.LTable
	L.SetField(switchai, "semantic_match_intent", L.NewFunction(func(L *lua.LState) int {
		if mockSemanticResult != nil {
			L.Push(mockSemanticResult)
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNil)
			L.Push(lua.LString("no match"))
		}
		return 2
	}))

	// Mock switchai.classify (Crucial for this test)
	var mockClassifyResponse string
	L.SetField(switchai, "classify", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(mockClassifyResponse))
		L.Push(lua.LNil)
		return 2
	}))

	// Mock switchai.parse_confidence
	L.SetField(switchai, "parse_confidence", L.NewFunction(func(L *lua.LState) int {
		jsonStr := L.CheckString(1)
		// Simple manual parse for testing
		// In reality, this calls Go's Scorer.Parse
		resTbl := L.NewTable()
		intent := ""
		if i := 0; i == 0 {
			// Very naive mock parsing
			if string(jsonStr[0]) == "{" {
				// We'll just use a simple regex-like extraction if we wanted to be fancy,
				// but let's just use the mockClassifyResponse logic or hardcode for now.
				// Actually, the test cases pass valid JSON, so we can just unmarshal it here!
				// But we are in Lua. Let's just return what we expect.
				// To stay robust, we'll use regex matching like the old Lua helper.

				// Re-implement the old Lua logic here for the mock
				compMatch := ""
				confMatch := 0.5

				// (Simplified manual extraction for the mock)
				if i := 0; i == 0 {
					// We'll use a hack: just use the tc logic by looking at the mockClassifyResponse
					// But this mock doesn't have access to tc.
					// So let's just do real string matching.

					// Intent
					if j := 0; j == 0 {
						if k := 0; k == 0 {
							// Find "intent": "..."
							if i := 0; i == 0 {
								// NAIVE MOCK:
								if jsonStr == `{"intent": "coding", "complexity": "complex", "confidence": 0.95}` {
									intent = "coding"
									compMatch = "complex"
									confMatch = 0.95
								} else if jsonStr == `{"intent": "coding", "complexity": "complex", "confidence": 0.40}` {
									intent = "coding"
									compMatch = "complex"
									confMatch = 0.40
								} else if jsonStr == `{"intent": "chat", "complexity": "simple", "confidence": 0.70}` {
									intent = "chat"
									compMatch = "simple"
									confMatch = 0.70
								} else if jsonStr == `{"intent": "coding", "complexity": "complex", "confidence": 0.90}` {
									intent = "coding"
									compMatch = "complex"
									confMatch = 0.90
								} else if jsonStr == `{"intent": "chat", "complexity": "simple", "confidence": 0.80}` {
									intent = "chat"
									compMatch = "simple"
									confMatch = 0.80
								}
							}
						}
					}
				}
				L.SetField(resTbl, "intent", lua.LString(intent))
				L.SetField(resTbl, "complexity", lua.LString(compMatch))
				L.SetField(resTbl, "confidence", lua.LNumber(confMatch))
				L.Push(resTbl)
				return 1
			}
		}
		L.Push(lua.LNil)
		L.Push(lua.LString("parse error"))
		return 2
	}))

	// Mock switchai.verify_intent
	L.SetField(switchai, "verify_intent", L.NewFunction(func(L *lua.LState) int {
		t1 := L.CheckString(1)
		t2 := L.CheckString(2)
		L.Push(lua.LBool(t1 == t2))
		return 1
	}))

	// Mock switchai.match_skill
	L.SetField(switchai, "match_skill", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 1
	}))

	// Mock switchai.cache_lookup
	L.SetField(switchai, "cache_lookup", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil) // No cache hit
		L.Push(lua.LNil) // No error
		return 2
	}))

	// Mock switchai.json_inject
	L.SetField(switchai, "json_inject", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(L.CheckString(1)))
		return 1
	}))

	// Mock switchai.cache_lookup (Phase 2)
	L.SetField(switchai, "cache_lookup", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil) // No cache hit
		L.Push(lua.LNil) // No error
		return 2
	}))

	// Mock switchai.cache_store (Phase 2)
	L.SetField(switchai, "cache_store", L.NewFunction(func(L *lua.LState) int {
		return 0
	}))

	// Mock switchai.record_feedback (Phase 2)
	L.SetField(switchai, "record_feedback", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 2
	}))

	// Mock switchai.evaluate_response (Phase 2)
	L.SetField(switchai, "evaluate_response", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 2
	}))


	// 2. Load handler.lua
	// We need to resolve the path. Assuming test is running from internal/plugin/
	// and handler is in plugins/cortex-router/handler.lua
	handlerPath, _ := filepath.Abs("../../plugins/cortex-router/handler.lua")

	// Preload 'schema' module since handler requires it
	L.PreloadModule("schema", func(L *lua.LState) int {
		// Return a dummy table
		L.Push(L.NewTable())
		return 1
	})

	if err := L.DoFile(handlerPath); err != nil {
		t.Fatalf("Failed to load handler.lua: %v", err)
	}

	// The script returns a table (Plugin)
	pluginTbl := L.Get(-1)
	if pluginTbl.Type() != lua.LTTable {
		t.Fatalf("handler.lua did not return a table, got %s", pluginTbl.Type())
	}

	// 3. Define Test Cases
	tests := []struct {
		name           string
		classifyJSON   string
		semanticIntent string
		semanticConf   float64
		expectModel    string
		expectTier     string
		expectIntent   string
		expectConf     float64
		expectVerify   string // "match", "mismatch", or "" (none)
	}{
		{
			name:         "High Confidence Coding (No Semantic)",
			classifyJSON: `{"intent": "coding", "complexity": "complex", "confidence": 0.95}`,
			expectModel:  "switchai-chat",
			expectTier:   "cognitive",
			expectIntent: "coding",
			expectConf:   0.95,
		},
		{
			name:         "Low Confidence -> Escalate to Reasoning",
			classifyJSON: `{"intent": "coding", "complexity": "complex", "confidence": 0.40}`,
			expectModel:  "switchai-reasoner",
			expectTier:   "cognitive",
			expectIntent: "coding",
			expectConf:   0.40,
		},
		{
			name:           "Verification Mismatch -> Escalate",
			classifyJSON:   `{"intent": "chat", "complexity": "simple", "confidence": 0.70}`,
			semanticIntent: "coding",
			semanticConf:   0.75,                // > 0.60
			expectModel:    "switchai-reasoner", // Mismatch -> Escalate
			expectTier:     "cognitive",
			expectVerify:   "mismatch",
			expectConf:     0.70,
		},
		{
			name:           "Verification Consensus",
			classifyJSON:   `{"intent": "coding", "complexity": "complex", "confidence": 0.90}`,
			semanticIntent: "coding",
			semanticConf:   0.70, // > 0.60
			expectModel:    "switchai-chat",
			expectTier:     "cognitive", // Still cognitive routing, but verified
			expectVerify:   "match",
			expectConf:     0.90,
		},
		{
			name:           "Ignore Low Confidence Semantic",
			classifyJSON:   `{"intent": "chat", "complexity": "simple", "confidence": 0.80}`,
			semanticIntent: "coding",
			semanticConf:   0.30,            // < 0.60, should be ignored
			expectModel:    "switchai-fast", // Chat -> fast
			expectTier:     "cognitive",
			expectVerify:   "", // No verification done
			expectConf:     0.80,
		},
	}

	// 4. Run Tests
	onRequest := L.GetField(pluginTbl, "on_request")
	if onRequest.Type() != lua.LTFunction {
		t.Fatal("Plugin:on_request is not a function")
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockClassifyResponse = tc.classifyJSON

			if tc.semanticIntent != "" {
				tbl := L.NewTable()
				L.SetField(tbl, "intent", lua.LString(tc.semanticIntent))
				L.SetField(tbl, "confidence", lua.LNumber(tc.semanticConf))
				mockSemanticResult = tbl
			} else {
				mockSemanticResult = nil
			}

			// Prepare request table
			req := L.NewTable()
			L.SetField(req, "model", lua.LString("auto")) // Trigger routing
			L.SetField(req, "body", lua.LString("test prompt"))
			L.SetField(req, "metadata", L.NewTable())

			// Call Plugin:on_request(req) -> req (colon syntax passes self)
			L.Push(onRequest)
			L.Push(pluginTbl) // self
			L.Push(req)
			if err := L.PCall(2, 1, nil); err != nil {
				t.Fatalf("Lua execution failed: %v", err)
			}

			// Check result
			res := L.Get(-1)
			L.Pop(1) // Pop result

			if res.Type() != lua.LTTable {
				t.Errorf("Expected table result, got %s", res.Type())
				return
			}
			resTbl := res.(*lua.LTable)

			// Check Model
			model := L.GetField(resTbl, "model")
			if model.String() != tc.expectModel {
				t.Errorf("Expected model %s, got %s", tc.expectModel, model.String())
			}

			// Check Metadata
			meta := L.GetField(resTbl, "metadata")
			if meta.Type() == lua.LTTable {
				metaTbl := meta.(*lua.LTable)

				// Check Tier
				tier := L.GetField(metaTbl, "routing_tier")
				if tier.String() != tc.expectTier {
					t.Errorf("Expected tier %s, got %s", tc.expectTier, tier.String())
				}

				// Check Verification
				if tc.expectVerify == "mismatch" {
					if L.GetField(metaTbl, "verification_mismatch") != lua.LTrue {
						t.Error("Expected verification_mismatch=true")
					}
					action := L.GetField(metaTbl, "verification_action")
					if action.String() != "escalate" {
						t.Errorf("Expected verification_action='escalate', got %s", action.String())
					}
				} else if tc.expectVerify == "match" {
					if L.GetField(metaTbl, "verification_match") != lua.LTrue {
						t.Error("Expected verification_match=true")
					}
				} else if tc.expectVerify == "" {
					if L.GetField(metaTbl, "verification_mismatch") != lua.LNil {
						t.Error("Expected no verification mismatch")
					}
				}

				// Check Confidence if cognitive
				if tc.expectTier == "cognitive" {
					conf := L.GetField(metaTbl, "cognitive_confidence")
					if lua.LVAsNumber(conf) != lua.LNumber(tc.expectConf) {
						t.Errorf("Expected confidence %v, got %v", tc.expectConf, conf)
					}
				}
			}
		})
	}
}
