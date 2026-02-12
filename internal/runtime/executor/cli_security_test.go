// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"testing"

	"github.com/traylinx/switchAILocal/internal/cli"
)

func TestCLI_ArgumentInjection_Security(t *testing.T) {
	// This test verifies that all known CLI tools are configured with PositionalArgsSeparator
	// to prevent argument injection attacks where a prompt starting with "--" is interpreted as a flag.

	// List of tools that MUST have PositionalArgsSeparator
	toolsToCheck := []string{"Mistral Vibe CLI", "Anthropic Claude CLI", "OpenAI Codex CLI", "Google Gemini CLI"}

	for _, toolName := range toolsToCheck {
		var def *cli.ToolDefinition
		for _, tool := range cli.KnownTools {
			if tool.Name == toolName {
				def = &tool
				break
			}
		}

		if def == nil {
			t.Fatalf("Tool %s not found in KnownTools", toolName)
		}

		// Create executor from definition
		executor := NewLocalCLIExecutor(cli.DiscoveredTool{
			Definition: *def,
			Path:       "/usr/bin/fake",
		})

		// Malicious prompt attempting to inject a flag
		maliciousPrompt := "--dangerous-flag"

		// Build args
		args, err := executor.buildFinalArgs(maliciousPrompt, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build args for %s: %v", toolName, err)
		}

		// If UseStdin is true, the prompt is passed via Stdin, so it's safe from argv injection.
		if def.UseStdin {
			// Verify prompt is NOT in args
			for _, arg := range args {
				if arg == maliciousPrompt {
					t.Errorf("[%s] UseStdin is true but prompt found in args: %s", toolName, arg)
				}
			}
			continue
		}

		// Check if the separator is present before the prompt
		if len(args) < 2 {
			t.Errorf("[%s] Args too short to be secure: %v", toolName, args)
			continue
		}

		separatorIndex := -1
		for i, arg := range args {
			if arg == "--" {
				separatorIndex = i
				break
			}
		}

		// Check prompt position
		promptIndex := len(args) - 1
		if args[promptIndex] != maliciousPrompt {
			t.Errorf("[%s] Expected last arg to be prompt, got %s", toolName, args[promptIndex])
		}

		// Verify security
		if separatorIndex == -1 {
			t.Errorf("[%s] SECURITY FAILURE: Missing PositionalArgsSeparator. Args: %v. Prompt '%s' could be interpreted as a flag.", toolName, args, maliciousPrompt)
		} else if separatorIndex != promptIndex-1 {
			t.Errorf("[%s] Separator found at index %d but prompt is at %d. Args: %v", toolName, separatorIndex, promptIndex, args)
		}
	}
}
