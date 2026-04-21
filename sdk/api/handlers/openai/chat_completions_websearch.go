// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

import (
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/traylinx/switchAILocal/internal/buildinfo"
)

// AIL_AUTOINJECT_WEBSEARCH        — master flag; when "true", web_search autoinjection runs.
// AIL_AUTOINJECT_MODELS           — comma-separated allowlist of request-model names
//                                    that receive autoinjection (e.g. "ail-compound,minimax/ail-compound").
// AIL_AUTOINJECT_FORCE_THRESHOLD  — integer; when the caller's `tools` already contains
//                                    ≥ threshold entries with type=="function", the injected
//                                    web_search tool is stamped with force_search:true to
//                                    beat MiniMax M2.7's function-tool preference heuristic
//                                    (agentic callers like OpenClaw send 20+ function tools
//                                    and the model otherwise picks those over native search).
//                                    Default: 5. Set 0 to disable force_search entirely.
// AIL_DEBUG_DUMP                  — when "true", dump each chat-completions raw request body to logs.
// X-Ail-Autoinject: off           — per-request opt-out header.
// X-Ail-Build response header     — emitted as "<commit-sha>-<hostname>" so the 20-call LB probe
//                                    can assert every instance behind nginx is on the new sha.

// webSearchMaxTokensFloor is the minimum max_tokens value required for a MiniMax
// web_search-augmented response. Search-augmented prompts can inflate context by
// 6k–13k tokens (documented in docs/user/api-reference.md:393), so we need at least
// 2000 tokens of output headroom to leave room for reasoning + final answer.
const webSearchMaxTokensFloor = int64(2000)

// defaultForceSearchThreshold is the number of caller-supplied type:"function" tools
// above which switchailocal switches the injected web_search shape from bare
// {"type":"web_search"} to {"type":"web_search","force_search":true}. Rationale in
// docs/WEBSEARCH-AUDIT-2026-04-21.md §4.2 — agentic callers (OpenClaw, LangChain agents)
// present a large function-tool menu and MiniMax M2.7's tool-selection heuristic tends
// to reach for those before native search. force_search sidesteps the heuristic.
const defaultForceSearchThreshold = 5

// autoInjectWebSearch mutates the chat-completions request body to append a
// web_search tool entry when:
//   - AIL_AUTOINJECT_WEBSEARCH=true
//   - modelName is in the AIL_AUTOINJECT_MODELS allowlist
//   - the caller did not send "X-Ail-Autoinject: off"
//   - the caller did not already include a web_search entry in tools
//
// Semantics:
//   - Set-union with dedupe: caller tools are preserved; web_search is appended only if
//     no existing entry with type == "web_search" is present.
//   - Caller wins on parameterized versions (e.g. {type:web_search, max_keyword:5, force_search:true})
//     — the appended bare entry is skipped in that case.
//   - Injected shape depends on the caller's `type:"function"` tool count. When that count
//     is ≥ forceSearchThreshold() (default 5), the injected tool is
//     {"type":"web_search","force_search":true} — this beats MiniMax's function-tool
//     preference heuristic for agentic clients like OpenClaw. Below the threshold we send
//     the bare {"type":"web_search"} form so MiniMax can autonomously skip search when the
//     model already knows the answer (cheap path for simple API callers).
//   - max_tokens floor: when injection fires and max_tokens is absent or < 2000, it is bumped to 2000.
//   - No-op paths (flag off, not allowlisted, opt-out, already has web_search) leave max_tokens untouched.
//
// Returns the (possibly mutated) request body. Errors in sjson mutation are swallowed and the original body
// is returned, matching the fail-open conventions used elsewhere in this package (see openai_openai_request.go:74).
func autoInjectWebSearch(rawJSON []byte, modelName string, optOut bool) []byte {
	if os.Getenv("AIL_AUTOINJECT_WEBSEARCH") != "true" {
		return rawJSON
	}
	if optOut {
		return rawJSON
	}
	if !isAutoinjectAllowlisted(modelName) {
		return rawJSON
	}

	hasWebSearch := false
	functionToolCount := 0
	if tools := gjson.GetBytes(rawJSON, "tools"); tools.Exists() && tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			switch tool.Get("type").String() {
			case "web_search":
				hasWebSearch = true
			case "function":
				functionToolCount++
			}
			return true
		})
	}

	modified := rawJSON
	if !hasWebSearch {
		tools := gjson.GetBytes(modified, "tools")
		idx := 0
		if tools.Exists() && tools.IsArray() {
			idx = len(tools.Array())
		}
		newTool := buildInjectedWebSearchTool(functionToolCount)
		if updated, err := sjson.SetBytes(modified, jsonArrayIndexPath("tools", idx), newTool); err == nil {
			modified = updated
		}
	}

	if mt := gjson.GetBytes(modified, "max_tokens"); !mt.Exists() || mt.Int() < webSearchMaxTokensFloor {
		if updated, err := sjson.SetBytes(modified, "max_tokens", webSearchMaxTokensFloor); err == nil {
			modified = updated
		}
	}

	return modified
}

// buildInjectedWebSearchTool picks between the bare {"type":"web_search"} form
// (autonomous — MiniMax decides per-query whether to search) and the forced
// {"type":"web_search","force_search":true} form for agentic callers whose tool
// arrays contain many function-typed entries. Rationale in §4.2 of the audit doc.
func buildInjectedWebSearchTool(callerFunctionToolCount int) map[string]any {
	threshold := forceSearchThreshold()
	if threshold > 0 && callerFunctionToolCount >= threshold {
		return map[string]any{"type": "web_search", "force_search": true}
	}
	return map[string]any{"type": "web_search"}
}

// forceSearchThreshold reads AIL_AUTOINJECT_FORCE_THRESHOLD as an integer. Unset,
// empty, or unparseable values fall back to defaultForceSearchThreshold (5). A
// value of 0 (set explicitly) disables force_search stamping entirely — autoinject
// then always uses the bare form regardless of how many function tools the caller
// sent.
func forceSearchThreshold() int {
	raw := strings.TrimSpace(os.Getenv("AIL_AUTOINJECT_FORCE_THRESHOLD"))
	if raw == "" {
		return defaultForceSearchThreshold
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultForceSearchThreshold
	}
	return n
}

// jsonArrayIndexPath returns the sjson path for appending at a given array index,
// e.g. jsonArrayIndexPath("tools", 2) -> "tools.2".
func jsonArrayIndexPath(field string, idx int) string {
	return field + "." + strconv.Itoa(idx)
}

// isAutoinjectAllowlisted reports whether modelName appears in the AIL_AUTOINJECT_MODELS
// comma-separated env var. Whitespace around entries is trimmed. An empty/unset env var
// means no models are allowlisted (safe default — injection never fires).
func isAutoinjectAllowlisted(modelName string) bool {
	envList := os.Getenv("AIL_AUTOINJECT_MODELS")
	if envList == "" {
		return false
	}
	for _, m := range strings.Split(envList, ",") {
		if strings.TrimSpace(m) == modelName {
			return true
		}
	}
	return false
}

// debugDumpRequest logs the raw chat-completions request body when AIL_DEBUG_DUMP=true.
// Used in Phase 1 evidence capture to prove the on-wire shape OpenClaw sends. The env
// gate means production droplets default to zero extra logs.
func debugDumpRequest(modelName string, rawJSON []byte) {
	if os.Getenv("AIL_DEBUG_DUMP") != "true" {
		return
	}
	log.WithFields(log.Fields{
		"component": "openai-chat-completions",
		"model":     modelName,
		"bytes":     len(rawJSON),
	}).Infof("AIL_DEBUG_DUMP body=%s", string(rawJSON))
}

// buildIdentity returns "<commit-sha>-<hostname>" for the X-Ail-Build response header.
// Consumed by the Phase 4 verification probe to assert every switchailocal instance
// behind nginx-LB is running the new sha (20-call probe over a 5-instance fleet).
func buildIdentity() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	commit := buildinfo.Commit
	if commit == "" {
		commit = "none"
	}
	return commit + "-" + host
}
