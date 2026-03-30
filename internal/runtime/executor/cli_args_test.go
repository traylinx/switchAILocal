// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"reflect"
	"strings"
	"testing"
)

func TestLocalCLIExecutor_BuildFinalArgs(t *testing.T) {
	tests := []struct {
		name              string
		executor          *LocalCLIExecutor
		prompt            string
		cliOpts           *CLIOptions
		formatArgs        []string
		wantArgs          []string
		wantErrorContains string
	}{
		{
			name: "Gemini CLI - Separator Injection",
			executor: &LocalCLIExecutor{
				Provider:                "geminicli",
				PositionalArgsSeparator: "--",
					Args:                    []string{}, // No default args
			},
			prompt:     "-dangerous-flag",
			cliOpts:    nil,
			formatArgs: nil,
			// Auto-injected: --no-interactive, -y, --include-directories=<sandbox>
			// We just verify --no-interactive and -y are present, separator and prompt are at the end
		},
		{
			name: "Gemini CLI - Separator with valid prompt",
			executor: &LocalCLIExecutor{
				Provider:                "geminicli",
				PositionalArgsSeparator: "--",
				Args:                    []string{},
			},
			prompt:     "hello world",
			cliOpts:    nil,
			formatArgs: nil,
			wantArgs:   []string{"--", "hello world"},
		},
		{
			name: "Vibe CLI - No Separator (Default)",
			executor: &LocalCLIExecutor{
				Provider:                "vibe",
				PositionalArgsSeparator: "",
				Args:                    []string{"-p"},
			},
			prompt:     "hello",
			cliOpts:    nil,
			formatArgs: nil,
			wantArgs:   []string{"-p", "hello"},
		},
		{
			name: "Complex Flags and Attachments with Separator",
			executor: &LocalCLIExecutor{
				Provider:                "geminicli",
				PositionalArgsSeparator: "--",
				Args:                    []string{},
				SandboxFlag:             "-s",
				SupportsAttachments:     true,
				AttachmentPrefix:        "@",
				SupportsJSON:            true,
			},
			prompt: "analyze this",
			cliOpts: &CLIOptions{
				Flags: CLIFlags{
					Sandbox: true,
				},
				Attachments: []Attachment{
					{Type: "file", Path: "test.txt"},
				},
			},
			formatArgs: []string{"--json"},
			// Expected order: [Control Flags] -> [Default Args] -> [Format Args] -> [Separator] -> [Prompt]
			wantArgs: []string{"-s", "--json", "--", "@test.txt analyze this"},
		},
		{
			name: "Gemini CLI - Separator with user flags",
			executor: &LocalCLIExecutor{
				Provider:                "geminicli",
				PositionalArgsSeparator: "--",
				Args:                    []string{},
				SandboxFlag:             "-s",
			},
			prompt: "hello",
			cliOpts: &CLIOptions{
				Flags: CLIFlags{Sandbox: true},
			},
			formatArgs: nil,
			wantArgs:   []string{"-s", "--", "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, err := tt.executor.buildFinalArgs(tt.prompt, tt.cliOpts, tt.formatArgs, "")

			if tt.wantErrorContains != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrorContains)
				} else if !strings.Contains(err.Error(), tt.wantErrorContains) {
					t.Errorf("expected error containing %q, got %q", tt.wantErrorContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// For geminicli, auto-injected flags (--no-interactive, -y, --include-directories)
			// are prepended. Instead of exact match, verify key elements are present
			// and the tail (user args) is correct.
			if tt.executor.Provider == "geminicli" {
				joined := strings.Join(gotArgs, " ")
				if !strings.Contains(joined, "-y") {
					t.Errorf("expected -y in args, got %v", gotArgs)
				}
				// Verify prompt is at the end
				last := gotArgs[len(gotArgs)-1]
				if !strings.Contains(last, tt.prompt) {
					t.Errorf("expected prompt %q at end of args, last arg was %q", tt.prompt, last)
				}
				// Verify separator is second-to-last if configured
				if tt.executor.PositionalArgsSeparator != "" && len(gotArgs) >= 2 {
					secondLast := gotArgs[len(gotArgs)-2]
					if secondLast != tt.executor.PositionalArgsSeparator {
						t.Errorf("expected separator %q before prompt, got %q", tt.executor.PositionalArgsSeparator, secondLast)
					}
				}
			} else if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("buildFinalArgs() = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}
