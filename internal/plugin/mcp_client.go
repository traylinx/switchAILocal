package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type mcpJSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpJSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

func executeMCPToolCall(ctx context.Context, command string, args []string, env map[string]string, toolName string, toolArgs map[string]any) (string, error) {
	// 30 seconds max for MCP execution involving network calls like Vision or Search
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	allowedCmds := map[string]bool{
		"uvx":     true,
		"npx":     true,
		"node":    true,
		"python3": true,
	}
	if !allowedCmds[command] {
		return "", fmt.Errorf("MCP command '%s' is not in the security whitelist", command)
	}

	cmd := exec.CommandContext(ctx, command, args...)
	
	// Set up environment safely
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// We only log stderr to console to aid debugging without corrupting JSON-RPC on stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start MCP process: %w", err)
	}

	defer func() {
		_ = cmd.Process.Kill()
	}()

	// JSON-RPC helpers
	sendMessage := func(msg interface{}) error {
		b, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		b = append(b, '\n')
		_, err = stdin.Write(b)
		return err
	}

	decoder := json.NewDecoder(stdout)

	readResponse := func(expectedID int) (*mcpJSONRPCResponse, error) {
		for {
			var resp mcpJSONRPCResponse
			if err := decoder.Decode(&resp); err != nil {
				return nil, fmt.Errorf("JSON-RPC decoding error: %w", err)
			}
			
			if resp.ID == expectedID {
				return &resp, nil
			}
		}
	}

	// 1. Initialize
	initReq := mcpJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "switchAILocal",
				"version": "1.0",
			},
		},
	}
	if err := sendMessage(initReq); err != nil {
		return "", fmt.Errorf("failed to send initialize: %w", err)
	}

	initResp, err := readResponse(1)
	if err != nil {
		return "", fmt.Errorf("failed during initialize handshake: %w", err)
	}
	if initResp.Error != nil {
		return "", fmt.Errorf("MCP initialize error: %s", string(initResp.Error))
	}

	// 2. Notifications/initialized
	initializedNotif := mcpJSONRPCRequest{
		JSONRPC: "2.0",
		// No ID for notifications
		Method: "notifications/initialized",
	}
	if err := sendMessage(initializedNotif); err != nil {
		return "", fmt.Errorf("failed to send initialized notification: %w", err)
	}

	// 3. Call tool
	toolReq := mcpJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": toolArgs,
		},
	}
	if err := sendMessage(toolReq); err != nil {
		return "", fmt.Errorf("failed to send tools/call: %w", err)
	}

	toolResp, err := readResponse(2)
	if err != nil {
		return "", fmt.Errorf("failed to read tools/call response: %w", err)
	}
	if toolResp.Error != nil {
		return "", fmt.Errorf("MCP tool error: %s", string(toolResp.Error))
	}

	// Parse the tool result
	// The result format is usually {"content": [{"type":"text","text":"..."}]}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}

	if err := json.Unmarshal(toolResp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse tool result: %w\nRaw: %s", err, string(toolResp.Result))
	}

	if result.IsError {
		// Try to extract text anyway
		var errText string
		if len(result.Content) > 0 {
			errText = result.Content[0].Text
		}
		return "", fmt.Errorf("tool returned error: %s", errText)
	}

	// Aggregate text
	var output string
	for _, c := range result.Content {
		if c.Type == "text" {
			output += c.Text + "\n"
		}
	}

	// Close stdin to prompt MCP to shut down cleanly if it supports it
	_ = stdin.Close()
	return output, nil
}
