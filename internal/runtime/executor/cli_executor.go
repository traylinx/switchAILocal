// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/cli"
	sdkauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	sdktranslator "github.com/traylinx/switchAILocal/sdk/translator"
)

// LocalCLIExecutor executes prompts by calling a local binary.
type LocalCLIExecutor struct {
	BinaryPath       string
	Args             []string
	Provider         string
	JSONFormatArgs   []string
	StreamFormatArgs []string
	SupportsJSON     bool
	SupportsStream   bool

	// Capability and Flag mapping fields
	SupportsAttachments bool
	AttachmentPrefix    string
	SandboxFlag         string
	AutoApproveFlag     string
	YoloFlag            string
	SessionFlag         string
}

// NewLocalCLIExecutor creates a new executor from a discovered CLI tool.
func NewLocalCLIExecutor(tool cli.DiscoveredTool) *LocalCLIExecutor {
	return &LocalCLIExecutor{
		Provider:            tool.Definition.ProviderKey,
		BinaryPath:          tool.Path,
		Args:                tool.Definition.DefaultArgs,
		JSONFormatArgs:      tool.Definition.JSONFormatArgs,
		StreamFormatArgs:    tool.Definition.StreamFormatArgs,
		SupportsJSON:        tool.Definition.SupportsJSON,
		SupportsStream:      tool.Definition.SupportsStream,
		SupportsAttachments: tool.Definition.SupportsAttachments,
		AttachmentPrefix:    tool.Definition.AttachmentPrefix,
		SandboxFlag:         tool.Definition.SandboxFlag,
		AutoApproveFlag:     tool.Definition.AutoApproveFlag,
		YoloFlag:            tool.Definition.YoloFlag,
		SessionFlag:         tool.Definition.SessionFlag,
	}
}

// NewLocalCLIExecutorSimple creates a basic executor (legacy compatibility).
func NewLocalCLIExecutorSimple(provider, binaryPath string, args []string) *LocalCLIExecutor {
	return &LocalCLIExecutor{
		Provider:   provider,
		BinaryPath: binaryPath,
		Args:       args,
	}
}

func (e *LocalCLIExecutor) Identifier() string {
	return e.Provider
}

// Execute runs the CLI tool with the prompt and returns an OpenAI-formatted response.
func (e *LocalCLIExecutor) Execute(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	prompt, err := extractPrompt(req, opts.SourceFormat)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to extract prompt: %w", err)
	}

	modelName := extractModelName(req)

	// Extract CLI options from extra_body FIRST
	cliOpts, _ := extractCLIOptions(req.Payload)

	// Build control flags from options
	var flags []string
	if cliOpts != nil {
		if cliOpts.Flags.Sandbox && e.SandboxFlag != "" {
			flags = append(flags, e.SandboxFlag)
		}
		if cliOpts.Flags.AutoApprove && e.AutoApproveFlag != "" {
			flags = append(flags, e.AutoApproveFlag)
		}
		if cliOpts.Flags.Yolo && e.YoloFlag != "" {
			flags = append(flags, e.YoloFlag)
		}
		if cliOpts.SessionID != "" && e.SessionFlag != "" {
			if strings.HasSuffix(e.SessionFlag, "=") {
				flags = append(flags, e.SessionFlag+cliOpts.SessionID)
			} else {
				flags = append(flags, e.SessionFlag, cliOpts.SessionID)
			}
		}
	}

	// Phase 0 Quick Fix: Auto-inject --dangerously-skip-permissions for claudecli
	// This prevents silent hangs on permission prompts when running in non-TTY environments
	if e.Provider == "claudecli" && !e.hasTTY() {
		skipPermFlag := "--dangerously-skip-permissions"
		// Only add if not already present
		if !e.containsFlag(flags, skipPermFlag) && !e.containsFlag(e.Args, skipPermFlag) {
			flags = append(flags, skipPermFlag)
			log.Infof("Auto-injected %s for claudecli (no TTY detected)", skipPermFlag)
		}
	}

	// Build command args: [Control Flags] -> [Default Args] -> [Format Args] -> [Prompt]
	// 1. Control flags first (sandbox, auto-approve, session)
	finalArgs := append([]string{}, flags...)

	// 2. Default tool arguments (like -p)
	finalArgs = append(finalArgs, e.Args...)

	// 3. Format arguments (like --output-format=json)
	if e.SupportsJSON && len(e.JSONFormatArgs) > 0 {
		finalArgs = append(finalArgs, e.JSONFormatArgs...)
	}

	// Build attachment prefix (for Gemini/Vibe style @-commands)
	attachmentPrefix, err := e.buildAttachmentPrefix(cliOpts)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("invalid attachment: %w", err)
	}

	// Combine attachments and prompt as the final argument
	finalPrompt := attachmentPrefix + prompt
	finalArgs = append(finalArgs, finalPrompt)

	// Check for remote execution bridge
	if remoteHost := os.Getenv("REMOTE_COMMAND_HOST"); remoteHost != "" {
		return e.executeRemote(ctx, remoteHost, e.BinaryPath, finalArgs, modelName)
	}

	cmd := exec.CommandContext(ctx, e.BinaryPath, finalArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Debugf("Executing CLI: %s %v", e.BinaryPath, finalArgs)

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}

		// Improve error message for known token expiry issues
		if strings.Contains(errMsg, "token expired") || strings.Contains(errMsg, "refresh token is not set") {
			var hint string
			switch e.Provider {
			case "geminicli":
				hint = "Run 'gcloud auth application-default login' to refresh credentials."
			case "vibe":
				hint = "Run 'vibe login' or check your MISTRAL_API_KEY."
			case "claudecli":
				hint = "Run 'claude login' or check your ANTHROPIC_API_KEY."
			case "codex":
				hint = "Run 'codex auth' or check your OPENAI_API_KEY."
			default:
				hint = "Please refresh your CLI tool credentials."
			}
			return switchailocalexecutor.Response{}, fmt.Errorf("local CLI missing valid auth: %s. %s", errMsg, hint)
		}

		// Phase 0 Quick Fix: Extract last 10 lines of stderr for debugging
		// This helps diagnose silent hangs and other CLI issues
		stderrHint := extractErrorHint(stderr.String())
		if stderrHint != "" {
			return switchailocalexecutor.Response{}, fmt.Errorf("CLI execution failed: %s. %s", errMsg, stderrHint)
		}

		return switchailocalexecutor.Response{}, fmt.Errorf("CLI execution failed: %s", errMsg)
	}

	rawOutput := stdout.String()

	// Parse and wrap in OpenAI format
	content := e.extractContent(rawOutput)
	usage := e.extractUsage(rawOutput)

	openAIPayload, err := BuildOpenAIResponse(modelName, content, usage)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to build OpenAI response: %w", err)
	}

	return switchailocalexecutor.Response{Payload: openAIPayload}, nil
}

// extractContent parses CLI output to get the response text.
func (e *LocalCLIExecutor) extractContent(raw string) string {
	if e.SupportsJSON {
		// Parse JSON output: {"response": "...", "result": "..."}
		// Handle leading/trailing noise by finding the JSON block
		jsonBlock := extractJSONBlock(raw)
		if jsonBlock != "" {
			var cliResp struct {
				Response string `json:"response"`
				Result   string `json:"result"`
			}
			if err := json.Unmarshal([]byte(jsonBlock), &cliResp); err == nil {
				if cliResp.Result != "" {
					return cliResp.Result
				}
				if cliResp.Response != "" {
					return cliResp.Response
				}
			}
		}
	}
	// Fallback: return cleaned raw text
	return cleanCLIOutput(raw)
}

// extractUsage parses token usage from CLI JSON output.
func (e *LocalCLIExecutor) extractUsage(raw string) *OpenAIUsage {
	if !e.SupportsJSON {
		return nil
	}

	jsonBlock := extractJSONBlock(raw)
	if jsonBlock == "" {
		return nil
	}

	var cliResp struct {
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Stats *struct {
			Models map[string]struct {
				Tokens struct {
					Prompt int `json:"prompt"`
					Total  int `json:"total"`
				} `json:"tokens"`
			} `json:"models"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(jsonBlock), &cliResp); err != nil {
		return nil
	}

	var prompt, total int

	// 1. Try "usage" field (Claude style)
	if cliResp.Usage != nil {
		prompt = cliResp.Usage.InputTokens
		total = cliResp.Usage.InputTokens + cliResp.Usage.OutputTokens
	} else if cliResp.Stats != nil {
		// 2. Try "stats" field (Standard style)
		for _, m := range cliResp.Stats.Models {
			prompt += m.Tokens.Prompt
			total += m.Tokens.Total
		}
	}

	completion := total - prompt
	if completion < 0 {
		completion = 0
	}

	if total == 0 {
		return nil
	}

	return &OpenAIUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
}

// ExecuteStream runs the CLI tool with streaming and returns OpenAI SSE chunks.
func (e *LocalCLIExecutor) ExecuteStream(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
	// If streaming not supported, fall back to buffered response
	if !e.SupportsStream || len(e.StreamFormatArgs) == 0 {
		return e.executeStreamFallback(ctx, auth, req, opts)
	}

	// Check for remote execution bridge
	if remoteHost := os.Getenv("REMOTE_COMMAND_HOST"); remoteHost != "" {
		// Note: Remote streaming is not yet supported in bridge agent, falling back to buffered
		log.Warn("Remote streaming requested but not supported yet by bridge agent, using buffered remote call")
		resp, err := e.Execute(ctx, auth, req, opts)
		if err != nil {
			return nil, err
		}
		ch := make(chan switchailocalexecutor.StreamChunk, 1)
		ch <- switchailocalexecutor.StreamChunk{Payload: resp.Payload}
		close(ch)
		return ch, nil
	}

	prompt, err := extractPrompt(req, opts.SourceFormat)
	if err != nil {
		return nil, fmt.Errorf("failed to extract prompt: %w", err)
	}

	modelName := extractModelName(req)

	// Extract CLI options from extra_body FIRST
	cliOpts, _ := extractCLIOptions(req.Payload)

	// Build control flags from options
	var flags []string
	if cliOpts != nil {
		if cliOpts.Flags.Sandbox && e.SandboxFlag != "" {
			flags = append(flags, e.SandboxFlag)
		}
		if cliOpts.Flags.AutoApprove && e.AutoApproveFlag != "" {
			flags = append(flags, e.AutoApproveFlag)
		}
		if cliOpts.Flags.Yolo && e.YoloFlag != "" {
			flags = append(flags, e.YoloFlag)
		}
		if cliOpts.SessionID != "" && e.SessionFlag != "" {
			if strings.HasSuffix(e.SessionFlag, "=") {
				flags = append(flags, e.SessionFlag+cliOpts.SessionID)
			} else {
				flags = append(flags, e.SessionFlag, cliOpts.SessionID)
			}
		}
	}

	// Phase 0 Quick Fix: Auto-inject --dangerously-skip-permissions for claudecli
	// This prevents silent hangs on permission prompts when running in non-TTY environments
	if e.Provider == "claudecli" && !e.hasTTY() {
		skipPermFlag := "--dangerously-skip-permissions"
		// Only add if not already present
		if !e.containsFlag(flags, skipPermFlag) && !e.containsFlag(e.Args, skipPermFlag) {
			flags = append(flags, skipPermFlag)
			log.Infof("Auto-injected %s for claudecli (no TTY detected)", skipPermFlag)
		}
	}

	// Build command args: [Control Flags] -> [Default Args] -> [Stream Format Args] -> [Prompt]
	// 1. Control flags first (sandbox, auto-approve, session)
	finalArgs := append([]string{}, flags...)

	// 2. Default tool arguments (like -p)
	finalArgs = append(finalArgs, e.Args...)

	// 3. Stream format arguments
	if e.SupportsStream && len(e.StreamFormatArgs) > 0 {
		finalArgs = append(finalArgs, e.StreamFormatArgs...)
	}

	// Build attachment prefix (for Gemini/Vibe style @-commands)
	attachmentPrefix, err := e.buildAttachmentPrefix(cliOpts)
	if err != nil {
		return nil, fmt.Errorf("invalid attachment: %w", err)
	}

	// Combine attachments and prompt as the final argument
	finalPrompt := attachmentPrefix + prompt
	finalArgs = append(finalArgs, finalPrompt)

	cmd := exec.CommandContext(ctx, e.BinaryPath, finalArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	log.Debugf("Executing CLI (streaming): %s %v", e.BinaryPath, finalArgs)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start CLI: %w", err)
	}

	ch := make(chan switchailocalexecutor.StreamChunk)
	go func() {
		defer close(ch)
		defer func() {
			// Ensure process is killed and waited for cleanup
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
		}()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(nil, 10_485_760) // 10MB buffer
		isFirst := true

		for scanner.Scan() {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()

			// Skip non-JSON lines (startup noise)
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "{") {
				continue
			}

			// Parse CLI stream JSON
			content, ok := parseStreamMessageContent(trimmed)
			if !ok {
				continue
			}

			// Build and send OpenAI SSE chunk
			chunk := BuildOpenAIStreamChunk(modelName, content, isFirst)
			select {
			case ch <- switchailocalexecutor.StreamChunk{Payload: chunk}:
				isFirst = false
			case <-ctx.Done():
				return
			}
		}

		// Log any scanner errors
		if err := scanner.Err(); err != nil {
			log.Errorf("CLI stream scan error: %v", err)
		}

		// Send finish chunk (upstream handler will add [DONE])
		select {
		case ch <- switchailocalexecutor.StreamChunk{Payload: BuildOpenAIStreamFinishChunk(modelName)}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// parseStreamMessageContent extracts content from CLI stream JSON.
// Format: {"type":"message", "content":"...", "delta":true}
func parseStreamMessageContent(line string) (string, bool) {
	var msg struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		Delta   bool   `json:"delta"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", false
	}
	// Only process message chunks with delta=true
	if msg.Type != "message" || !msg.Delta {
		return "", false
	}
	return msg.Content, true
}

// executeStreamFallback wraps non-streaming response as single chunk.
func (e *LocalCLIExecutor) executeStreamFallback(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
	resp, err := e.Execute(ctx, auth, req, opts)
	if err != nil {
		return nil, err
	}
	ch := make(chan switchailocalexecutor.StreamChunk, 1)
	ch <- switchailocalexecutor.StreamChunk{Payload: resp.Payload}
	close(ch)
	return ch, nil
}

// Refresh is a no-op for CLI proxy as auth is handled externally by the tool.
func (e *LocalCLIExecutor) Refresh(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error) {
	return auth, nil
}

// executeRemote forwards execution to a host-side bridge agent.
func (e *LocalCLIExecutor) executeRemote(ctx context.Context, remoteHost, binary string, args []string, modelName string) (switchailocalexecutor.Response, error) {
	log.Infof("Forwarding execution to remote bridge: %s", remoteHost)

	reqBody := struct {
		Binary string   `json:"binary"`
		Args   []string `json:"args"`
	}{
		Binary: binary,
		Args:   args,
	}

	jsonBody, _ := json.Marshal(reqBody)
	url := strings.TrimSuffix(remoteHost, "/") + "/run"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to create bridge request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("bridge request failed: %w. Is the bridge agent running on the host?", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return switchailocalexecutor.Response{}, fmt.Errorf("bridge returned error (%d): %s", resp.StatusCode, string(body))
	}

	var bridgeResp struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
		Error    string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&bridgeResp); err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to decode bridge response: %w", err)
	}

	if bridgeResp.ExitCode != 0 {
		errMsg := strings.TrimSpace(bridgeResp.Stderr)
		if errMsg == "" {
			errMsg = bridgeResp.Error
		}
		return switchailocalexecutor.Response{}, fmt.Errorf("remote CLI execution failed: %s", errMsg)
	}

	// Parse and wrap in OpenAI format
	content := e.extractContent(bridgeResp.Stdout)
	usage := e.extractUsage(bridgeResp.Stdout)

	openAIPayload, err := BuildOpenAIResponse(modelName, content, usage)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("failed to build OpenAI response from remote output: %w", err)
	}

	return switchailocalexecutor.Response{Payload: openAIPayload}, nil
}

// CountTokens returns 0 as CLI tools generally don't expose this via simple one-shot commands.
func (e *LocalCLIExecutor) CountTokens(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	return switchailocalexecutor.Response{
		Payload: []byte(`{"total_tokens": 0}`),
	}, nil
}

// extractPrompt gets the raw prompt string from the request.
func extractPrompt(req switchailocalexecutor.Request, format sdktranslator.Format) (string, error) {
	payload := string(req.Payload)
	if strings.Contains(payload, `"messages"`) {
		return parseMessages(req.Payload)
	}
	return payload, nil
}

// extractModelName gets the model name from the request payload.
func extractModelName(req switchailocalexecutor.Request) string {
	var chatReq struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(req.Payload, &chatReq); err != nil {
		return "unknown"
	}
	if chatReq.Model == "" {
		return "unknown"
	}
	return chatReq.Model
}

// cleanCLIOutput removes noise logs like "[STARTUP] ..." and "Loaded cached credentials".
func cleanCLIOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	var clean []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Filter known noise patterns
		if strings.HasPrefix(trimmed, "[STARTUP]") {
			continue
		}
		if strings.HasPrefix(trimmed, "Loaded cached credentials") {
			continue
		}
		clean = append(clean, line)
	}
	return strings.Join(clean, "\n")
}

// parseMessages extracts messages from OpenAI JSON payload.
// Includes system, user, and assistant messages for full context.
func parseMessages(data []byte) (string, error) {
	type ContentPart struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}

	type Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	type ChatReq struct {
		Messages []Message `json:"messages"`
	}

	var req ChatReq
	if err := json.Unmarshal(data, &req); err != nil {
		return "", err
	}

	if len(req.Messages) == 0 {
		return "", fmt.Errorf("no messages found")
	}

	// Build prompt with context from all message types
	var sb strings.Builder
	for _, msg := range req.Messages {
		var contentStr string

		// Try string first
		var simpleContent string
		if err := json.Unmarshal(msg.Content, &simpleContent); err == nil {
			contentStr = simpleContent
		} else {
			// Try array
			var parts []ContentPart
			if err := json.Unmarshal(msg.Content, &parts); err == nil {
				for _, part := range parts {
					if part.Type == "text" {
						contentStr += part.Text
					}
				}
			}
		}

		switch msg.Role {
		case "system":
			sb.WriteString("[System]: " + contentStr + "\n\n")
		case "user":
			sb.WriteString(contentStr + "\n")
		case "assistant":
			sb.WriteString("[Previous response]: " + contentStr + "\n\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// CLIOptions defines the structure for extra_body.cli options.
type CLIOptions struct {
	Attachments []Attachment `json:"attachments"`
	Flags       CLIFlags     `json:"flags"`
	SessionID   string       `json:"session_id"`
}

// Attachment defines a file or folder to be included in the CLI prompt.
type Attachment struct {
	Type string `json:"type"` // "file", "folder", "glob"
	Path string `json:"path"`
}

// CLIFlags defines provider-agnostic flags to be mapped to CLI-specific arguments.
type CLIFlags struct {
	Sandbox     bool `json:"sandbox"`
	AutoApprove bool `json:"auto_approve"`
	Yolo        bool `json:"yolo"`
}

// extractCLIOptions parses the extra_body.cli object from the request payload.
func extractCLIOptions(payload []byte) (*CLIOptions, error) {
	// Simple check if extra_body.cli exists
	if !strings.Contains(string(payload), "extra_body") || !strings.Contains(string(payload), "cli") {
		return nil, nil
	}

	type ExtraBody struct {
		CLI *CLIOptions `json:"cli"`
	}
	type Request struct {
		ExtraBody *ExtraBody `json:"extra_body"`
	}

	var req Request
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	if req.ExtraBody == nil || req.ExtraBody.CLI == nil {
		return nil, nil
	}

	return req.ExtraBody.CLI, nil
}

// extractJSONBlock finds the first '{' and last '}' to extract a JSON block from potentially noisy output.
func extractJSONBlock(raw string) string {
	start := strings.Index(raw, "{")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(raw, "}")
	if end == -1 || end < start {
		return ""
	}
	return raw[start : end+1]
}

// hasTTY checks if the process has a TTY attached.
// In server/daemon contexts, this will typically return false.
func (e *LocalCLIExecutor) hasTTY() bool {
	// Check if stdin is a terminal
	// In a server context (like switchAILocal), this will be false
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	// Check if stdin is a character device (terminal)
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// containsFlag checks if a flag is present in a slice of arguments.
func (e *LocalCLIExecutor) containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// extractErrorHint extracts the last 10 lines of stderr and provides a helpful hint.
// This is part of Phase 0 Quick Fix to help diagnose CLI issues, particularly silent hangs.
func extractErrorHint(stderr string) string {
	if stderr == "" {
		return ""
	}

	lines := strings.Split(stderr, "\n")

	// Get last 10 non-empty lines
	var lastLines []string
	for i := len(lines) - 1; i >= 0 && len(lastLines) < 10; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			lastLines = append([]string{trimmed}, lastLines...)
		}
	}

	if len(lastLines) == 0 {
		return ""
	}

	// Build hint with last lines
	hint := "CLI may be waiting for input. Last stderr output:\n"
	for _, line := range lastLines {
		hint += "  " + line + "\n"
	}

	return strings.TrimSpace(hint)
}

// buildAttachmentPrefix processes attachments, verifies paths are safe, and returns the prefix string.
func (e *LocalCLIExecutor) buildAttachmentPrefix(cliOpts *CLIOptions) (string, error) {
	if cliOpts == nil || !e.SupportsAttachments {
		return "", nil
	}

	var attachmentPrefix string
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine working directory: %w", err)
	}

	for _, att := range cliOpts.Attachments {
		path := filepath.Clean(att.Path)

		// Security Check: Prevent path traversal
		// Ensure path is within CWD
		var absPath string
		if filepath.IsAbs(path) {
			absPath = path
		} else {
			absPath = filepath.Join(cwd, path)
		}

		rel, err := filepath.Rel(cwd, absPath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve path %s relative to CWD: %w", path, err)
		}

		// Reject paths starting with ".." (traversal up) or equals ".."
		if strings.HasPrefix(rel, "..") || rel == ".." {
			return "", fmt.Errorf("security violation: attachment path %s attempts to traverse outside working directory", att.Path)
		}

		if att.Type == "folder" && !strings.HasSuffix(path, "/") {
			path += "/"
		}
		attachmentPrefix += e.AttachmentPrefix + path + " "
	}
	return attachmentPrefix, nil
}
