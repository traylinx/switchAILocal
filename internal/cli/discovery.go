// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cli

import (
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/constant"
)

// ToolDefinition represents a supported CLI tool.
type ToolDefinition struct {
	Name             string   // Display name (e.g., "Google Gemini CLI")
	BinaryName       string   // Binary name to look for (e.g., "gemini")
	ProviderKey      string   // The provider key used in SwitchAI (e.g., "geminicli")
	DefaultArgs      []string // Default arguments to prepend (e.g., []string{"-p"})
	JSONFormatArgs   []string // Args to enable JSON output (e.g., ["--output-format=json"])
	StreamFormatArgs []string // Args to enable streaming JSON (e.g., ["--output-format=stream-json"])
	SupportsJSON     bool     // Whether the CLI supports JSON output format
	SupportsStream   bool     // Whether the CLI supports streaming JSON format

	// Capability and Flag mapping fields
	SupportsAttachments bool   // Whether the CLI supports @-command style attachments
	AttachmentPrefix    string // Prefix for attachments (e.g., "@")
	SandboxFlag         string // CLI flag for sandbox mode (e.g., "-s")
	AutoApproveFlag     string // CLI flag for auto-approval (e.g., "-y")
	YoloFlag            string // CLI flag for YOLO mode (e.g., "--yolo")
	SessionFlag         string // CLI flag for sessions (e.g., "--resume=" or "--continue")
	// Security fields
	UseStdin                bool   // Whether to pass the prompt via stdin instead of arguments
	PositionalArgsSeparator string // Separator to use before positional arguments (e.g., "--")
}

// DiscoveredTool represents a tool found on the local system.
type DiscoveredTool struct {
	Definition ToolDefinition
	Path       string
}

// KnownTools lists all CLI tools that SwitchAI supports as proxies.
var KnownTools = []ToolDefinition{
	{
		Name:                    "Google Gemini CLI",
		BinaryName:              "gemini",
		ProviderKey:             constant.GeminiCLI,
		DefaultArgs:             []string{}, // Use positional prompt
		JSONFormatArgs:          []string{"--output-format=json"},
		StreamFormatArgs:        []string{"--output-format=stream-json"},
		SupportsJSON:            true,
		SupportsStream:          true,
		SupportsAttachments:     true,
		AttachmentPrefix:        "@",
		SandboxFlag:             "-s",
		AutoApproveFlag:         "-y",
		YoloFlag:                "--yolo",
		SessionFlag:             "--resume=",
		UseStdin:                true,
		PositionalArgsSeparator: "--",
	},
	{
		Name:                "Mistral Vibe CLI",
		BinaryName:          "vibe",
		ProviderKey:         constant.VibeCLI,
		DefaultArgs:         []string{"-p"},
		SupportsJSON:        false,
		SupportsStream:      false,
		SupportsAttachments: true,
		AttachmentPrefix:    "@",
		AutoApproveFlag:     "--auto-approve",
		YoloFlag:            "--auto-approve",
		SessionFlag:         "--continue",
	},
	{
		Name:                "Anthropic Claude CLI",
		BinaryName:          "claude",
		ProviderKey:         constant.ClaudeCLI,
		DefaultArgs:         []string{"--print"}, // Use --print to be explicit that it's a flag
		JSONFormatArgs:      []string{"--output-format=json"},
		StreamFormatArgs:    []string{"--output-format=stream-json"},
		SupportsJSON:        true,
		SupportsStream:      true,
		SupportsAttachments: false, // Claude Code manages context internally
		AutoApproveFlag:     "--dangerously-skip-permissions",
		YoloFlag:            "--dangerously-skip-permissions",
		SessionFlag:         "--resume",
	},
	{
		Name:                "OpenAI Codex CLI",
		BinaryName:          "codex",
		ProviderKey:         "codex",
		DefaultArgs:         []string{"-p"},
		SupportsJSON:        false,
		SupportsStream:      false,
		SupportsAttachments: false,
		SandboxFlag:         "--sandbox",
		AutoApproveFlag:     "--full-auto",
		YoloFlag:            "--full-auto",
	},
}

// DiscoverInstalledTools scans the system PATH for known CLI tools.
func DiscoverInstalledTools() []DiscoveredTool {
	var found []DiscoveredTool

	for _, tool := range KnownTools {
		path, err := exec.LookPath(tool.BinaryName)
		if err == nil && path != "" {
			absPath, _ := filepath.Abs(path)
			log.Debugf("Discovered local CLI tool: %s at %s", tool.Name, absPath)
			found = append(found, DiscoveredTool{
				Definition: tool,
				Path:       absPath,
			})
		} else {
			log.Debugf("Local CLI tool not found: %s", tool.Name)
		}
	}

	return found
}
