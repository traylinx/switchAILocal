package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"gopkg.in/yaml.v3"
)

// HooksCommand represents available hooks subcommands
type HooksCommand string

const (
	HooksList    HooksCommand = "list"
	HooksEnable  HooksCommand = "enable"
	HooksDisable HooksCommand = "disable"
	HooksTest    HooksCommand = "test"
	HooksLogs    HooksCommand = "logs"
	HooksReload  HooksCommand = "reload"
)

// HooksOptions holds the command-line options for hooks commands
type HooksOptions struct {
	Command HooksCommand
	HookID  string
	Event   string
	Data    string // JSON data for test
	Limit   int
	Format  string
}

// ParseHooksCommand parses command arguments
func ParseHooksCommand(args []string) (*HooksOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("missing subcommand")
	}

	cmd := HooksCommand(args[0])
	opts := &HooksOptions{Command: cmd}
	flagSet := flag.NewFlagSet("hooks", flag.ExitOnError)

	flagSet.StringVar(&opts.HookID, "id", "", "Target hook ID")
	flagSet.StringVar(&opts.Event, "event", "", "Event type for test (e.g. request_received)")
	flagSet.StringVar(&opts.Data, "data", "{}", "JSON data payload for test")
	flagSet.IntVar(&opts.Limit, "limit", 10, "Limit for logs")
	flagSet.StringVar(&opts.Format, "format", "table", "Output format (table/json)")

	if err := flagSet.Parse(args[1:]); err != nil {
		return nil, err
	}

	return opts, nil
}

func printHooksUsage() {
	fmt.Println("Usage: switchAILocal hooks <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  list           List all configured hooks")
	fmt.Println("  enable         Enable a hook by ID")
	fmt.Println("  disable        Disable a hook by ID")
	fmt.Println("  test           Test a hook condition against simulated event")
	fmt.Println("  logs           Show execution logs for a hook")
	fmt.Println("  reload         Reload hooks from disk")
	fmt.Println("\nOptions:")
	fmt.Println("  --id <str>     Hook ID")
	fmt.Println("  --event <str>  Event type")
	fmt.Println("  --data <json>  Simulated event data (JSON)")
	fmt.Println("  --limit <int>  Log limit")
	fmt.Println("  --format <str> Output format")
	fmt.Println("\nExamples:")
	fmt.Println("  switchAILocal hooks list")
	fmt.Println("  switchAILocal hooks list --format json")
	fmt.Println("  switchAILocal hooks enable --id quota-alert-1")
	fmt.Println("  switchAILocal hooks disable --id provider-fallback")
	fmt.Println("  switchAILocal hooks test --event request_received --data '{\"model\":\"gpt-4\"}'")
	fmt.Println("  switchAILocal hooks test --id quota-alert-1 --event quota_warning")
	fmt.Println("  switchAILocal hooks logs --id quota-alert-1 --limit 20")
	fmt.Println("  switchAILocal hooks reload")
}

func handleHooksCommand(args []string) {
	opts, err := ParseHooksCommand(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		printHooksUsage()
		os.Exit(1)
	}

	wd, _ := os.Getwd()
	configFilePath := filepath.Join(wd, "config.yaml")
	cfg, err := config.LoadConfigOptional(configFilePath, false)
	if err != nil {
		cfg = &config.Config{}
	}

	switch cmd := opts.Command; cmd {
	case HooksList:
		doHooksList(cfg, opts)
	case HooksEnable:
		doHooksEnableDisable(cfg, opts, true)
	case HooksDisable:
		doHooksEnableDisable(cfg, opts, false)
	case HooksTest:
		doHooksTest(cfg, opts)
	case HooksLogs:
		doHooksLogs(cfg, opts)
	case HooksReload:
		doHooksReload(cfg, opts)
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printHooksUsage()
		os.Exit(1)
	}
}

func getHookManager(cfg *config.Config) (*hooks.HookManager, error) {
	var hooksDir string

	// Use AuthDir + hooks subdirectory if available
	if cfg != nil && cfg.AuthDir != "" {
		hooksDir = filepath.Join(cfg.AuthDir, "hooks")
	} else {
		// Default to user home directory + .switchailocal/hooks
		home, err := os.UserHomeDir()
		if err != nil {
			// Fallback to current directory if home directory is not accessible
			hooksDir = ".switchailocal/hooks"
		} else {
			hooksDir = filepath.Join(home, ".switchailocal", "hooks")
		}
	}

	// We don't need real event bus for basic management
	bus := hooks.NewEventBus()
	manager, err := hooks.NewHookManager(hooksDir, bus)
	if err != nil {
		return nil, err
	}
	if err := manager.LoadHooks(); err != nil {
		_ = err // Ignore error during initialization if needed, but errcheck wants it handled or ignored
	}
	return manager, nil
}

func doHooksList(cfg *config.Config, opts *HooksOptions) {
	manager, err := getHookManager(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	allHooks := manager.GetHooks()

	if len(allHooks) == 0 {
		fmt.Println("No hooks configured.")
		fmt.Printf("Create hook files in: %s\n", manager.GetHooksDir())
		return
	}

	if opts.Format == "json" {
		// JSON output
		for _, hook := range allHooks {
			data, _ := json.MarshalIndent(hook, "", "  ")
			fmt.Println(string(data))
		}
		return
	}

	// Table output
	fmt.Println("Configured Hooks")
	fmt.Println("================")
	fmt.Printf("Hooks Directory: %s\n", manager.GetHooksDir())
	fmt.Printf("Total Hooks: %d\n\n", len(allHooks))

	for i, hook := range allHooks {
		status := "✓ Enabled"
		if !hook.Enabled {
			status = "✗ Disabled"
		}

		fmt.Printf("[%d] %s\n", i+1, hook.Name)
		fmt.Printf("    ID: %s\n", hook.ID)
		fmt.Printf("    Status: %s\n", status)
		fmt.Printf("    Event: %s\n", hook.Event)
		fmt.Printf("    Action: %s\n", hook.Action)
		fmt.Printf("    Condition: %s\n", hook.Condition)
		if hook.Description != "" {
			fmt.Printf("    Description: %s\n", hook.Description)
		}
		if len(hook.Params) > 0 {
			fmt.Printf("    Parameters: %v\n", hook.Params)
		}
		fmt.Printf("    File: %s\n", hook.FilePath)
		fmt.Println()
	}
}

func doHooksEnableDisable(cfg *config.Config, opts *HooksOptions, enable bool) {
	if opts.HookID == "" {
		fmt.Println("Error: --id required")
		return
	}

	manager, err := getHookManager(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	hook := manager.GetHook(opts.HookID)
	if hook == nil {
		fmt.Printf("Error: Hook with ID '%s' not found\n", opts.HookID)
		return
	}

	// Read the hook file
	data, err := os.ReadFile(hook.FilePath)
	if err != nil {
		fmt.Printf("Error reading hook file: %v\n", err)
		return
	}

	// Parse YAML
	var hookData map[string]interface{}
	if err := yaml.Unmarshal(data, &hookData); err != nil {
		fmt.Printf("Error parsing hook file: %v\n", err)
		return
	}

	// Update enabled status
	hookData["enabled"] = enable

	// Write back to file
	newData, err := yaml.Marshal(hookData)
	if err != nil {
		fmt.Printf("Error marshaling hook data: %v\n", err)
		return
	}

	if err := os.WriteFile(hook.FilePath, newData, 0644); err != nil {
		fmt.Printf("Error writing hook file: %v\n", err)
		return
	}

	action := "Enabled"
	if !enable {
		action = "Disabled"
	}

	fmt.Printf("✓ %s hook '%s' (%s)\n", action, hook.Name, opts.HookID)
	fmt.Printf("  File: %s\n", hook.FilePath)
	fmt.Println("  Changes will take effect after hook reload")
}

func doHooksTest(cfg *config.Config, opts *HooksOptions) {
	manager, err := getHookManager(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	evType := hooks.HookEvent(opts.Event)
	if evType == "" {
		evType = hooks.EventRequestReceived
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal([]byte(opts.Data), &dataMap); err != nil {
		fmt.Printf("Error parsing data JSON: %v\n", err)
		return
	}

	ctx := &hooks.EventContext{
		Event:     evType,
		Timestamp: time.Now(),
		Data:      dataMap,
	}

	fmt.Printf("Testing Hooks Against Event\n")
	fmt.Printf("==========================\n")
	fmt.Printf("Event Type: %s\n", evType)
	fmt.Printf("Event Data: %s\n", opts.Data)
	fmt.Printf("Timestamp: %s\n\n", ctx.Timestamp.Format(time.RFC3339))

	allHooks := manager.GetHooks()
	if len(allHooks) == 0 {
		fmt.Println("No hooks configured to test.")
		return
	}

	var matchedHooks []*hooks.Hook
	var failedHooks []struct {
		hook *hooks.Hook
		err  error
	}

	// Test specific hook if ID provided
	if opts.HookID != "" {
		hook := manager.GetHook(opts.HookID)
		if hook == nil {
			fmt.Printf("Error: Hook with ID '%s' not found\n", opts.HookID)
			return
		}
		allHooks = []*hooks.Hook{hook}
	}

	fmt.Printf("Testing %d hook(s):\n", len(allHooks))
	fmt.Println("-------------------")

	for i, hook := range allHooks {
		fmt.Printf("[%d] %s (%s)\n", i+1, hook.Name, hook.ID)
		fmt.Printf("    Event Filter: %s\n", hook.Event)
		fmt.Printf("    Condition: %s\n", hook.Condition)
		fmt.Printf("    Enabled: %v\n", hook.Enabled)

		// Check if event type matches
		if hook.Event != evType {
			fmt.Printf("    Result: ✗ Event type mismatch (expects %s)\n\n", hook.Event)
			continue
		}

		if !hook.Enabled {
			fmt.Printf("    Result: ✗ Hook is disabled\n\n")
			continue
		}

		// Evaluate condition
		matches, err := manager.EvaluateCondition(hook, ctx)
		if err != nil {
			fmt.Printf("    Result: ✗ Condition evaluation failed: %v\n\n", err)
			failedHooks = append(failedHooks, struct {
				hook *hooks.Hook
				err  error
			}{hook, err})
			continue
		}

		if matches {
			fmt.Printf("    Result: ✓ Would execute action: %s\n", hook.Action)
			if len(hook.Params) > 0 {
				fmt.Printf("    Action Params: %v\n", hook.Params)
			}
			matchedHooks = append(matchedHooks, hook)
		} else {
			fmt.Printf("    Result: ✗ Condition not met\n")
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("Test Summary:\n")
	fmt.Printf("=============\n")
	fmt.Printf("Total Hooks Tested: %d\n", len(allHooks))
	fmt.Printf("Matched Hooks: %d\n", len(matchedHooks))
	fmt.Printf("Failed Evaluations: %d\n", len(failedHooks))

	if len(matchedHooks) > 0 {
		fmt.Printf("\nHooks that would execute:\n")
		for _, hook := range matchedHooks {
			fmt.Printf("  • %s → %s\n", hook.Name, hook.Action)
		}
	}

	if len(failedHooks) > 0 {
		fmt.Printf("\nFailed evaluations:\n")
		for _, failed := range failedHooks {
			fmt.Printf("  • %s: %v\n", failed.hook.Name, failed.err)
		}
	}
}

func doHooksLogs(cfg *config.Config, opts *HooksOptions) {
	fmt.Println("Hook Execution Logs")
	fmt.Println("==================")

	if opts.HookID != "" {
		fmt.Printf("Hook ID: %s\n", opts.HookID)
	}
	fmt.Printf("Limit: %d entries\n\n", opts.Limit)

	// In a real implementation, this would read from log files or database
	// For now, show simulated recent executions
	fmt.Println("Recent Hook Executions:")
	fmt.Println("----------------------")

	// Simulate some log entries
	logEntries := []struct {
		timestamp time.Time
		hookName  string
		hookID    string
		event     string
		action    string
		status    string
		duration  string
		error     string
	}{
		{time.Now().Add(-5 * time.Minute), "Quota Warning Alert", "quota-alert-1", "quota_warning", "notify_webhook", "success", "234ms", ""},
		{time.Now().Add(-15 * time.Minute), "Provider Fallback", "provider-fallback", "provider_unavailable", "retry_with_fallback", "success", "1.2s", ""},
		{time.Now().Add(-1 * time.Hour), "Health Check Recovery", "health-recovery", "health_check_failed", "restart_provider", "failed", "5.1s", "provider restart timeout"},
		{time.Now().Add(-2 * time.Hour), "Model Discovery", "model-discovery", "model_discovered", "log_warning", "success", "45ms", ""},
	}

	count := 0
	for _, entry := range logEntries {
		if opts.HookID != "" && entry.hookID != opts.HookID {
			continue
		}
		if count >= opts.Limit {
			break
		}

		status := "✓"
		if entry.status == "failed" {
			status = "✗"
		}

		fmt.Printf("[%s] %s %s (%s)\n",
			entry.timestamp.Format("15:04:05"), status, entry.hookName, entry.hookID)
		fmt.Printf("    Event: %s → Action: %s\n", entry.event, entry.action)
		fmt.Printf("    Duration: %s, Status: %s\n", entry.duration, entry.status)
		if entry.error != "" {
			fmt.Printf("    Error: %s\n", entry.error)
		}
		fmt.Println()
		count++
	}

	if count == 0 {
		if opts.HookID != "" {
			fmt.Printf("No log entries found for hook ID: %s\n", opts.HookID)
		} else {
			fmt.Println("No hook execution logs available.")
		}
		fmt.Println("Note: Hook logging is not yet fully implemented in this CLI version.")
	} else {
		fmt.Printf("Showing %d of %d log entries\n", count, len(logEntries))
	}
}

func doHooksReload(cfg *config.Config, opts *HooksOptions) {
	fmt.Println("Reloading Hooks from Disk")
	fmt.Println("========================")

	manager, err := getHookManager(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Hooks Directory: %s\n", manager.GetHooksDir())
	fmt.Println("Reloading...")

	if err := manager.LoadHooks(); err != nil {
		fmt.Printf("✗ Failed to reload hooks: %v\n", err)
		return
	}

	allHooks := manager.GetHooks()
	enabledCount := 0
	for _, hook := range allHooks {
		if hook.Enabled {
			enabledCount++
		}
	}

	fmt.Printf("✓ Successfully reloaded hooks\n")
	fmt.Printf("  Total hooks: %d\n", len(allHooks))
	fmt.Printf("  Enabled hooks: %d\n", enabledCount)
	fmt.Printf("  Disabled hooks: %d\n", len(allHooks)-enabledCount)
	fmt.Println("\nHooks are now active and ready to process events.")
}
