package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// HeartbeatCommand represents the available heartbeat subcommands
type HeartbeatCommand string

const (
	HeartbeatStatus   HeartbeatCommand = "status"
	HeartbeatCheck    HeartbeatCommand = "check"
	HeartbeatQuota    HeartbeatCommand = "quota"
	HeartbeatDiscover HeartbeatCommand = "discover"
)

// HeartbeatOptions holds the command-line options for heartbeat commands
type HeartbeatOptions struct {
	Command  HeartbeatCommand
	Provider string
	Format   string
}

// ParseHeartbeatCommand parses command arguments
func ParseHeartbeatCommand(args []string) (*HeartbeatOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("missing subcommand")
	}

	cmd := HeartbeatCommand(args[0])
	opts := &HeartbeatOptions{Command: cmd}
	flagSet := flag.NewFlagSet("heartbeat", flag.ExitOnError)

	switch cmd {
	case HeartbeatCheck:
		if len(args) < 2 {
			return nil, fmt.Errorf("check command requires a provider name")
		}
		opts.Provider = args[1]
		// skip provider name in parsing
		if err := flagSet.Parse(args[2:]); err != nil {
			return nil, err
		}
	default:
		if err := flagSet.Parse(args[1:]); err != nil {
			return nil, err
		}
	}

	// handle remaining flags if any specific ones are added later
	return opts, nil
}

func printHeartbeatUsage() {
	fmt.Println("Usage: switchAILocal heartbeat <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  status      Show all provider health")
	fmt.Println("  check <p>   Check specific provider health")
	fmt.Println("  quota       Show quota usage for all providers")
	fmt.Println("  discover    Force model discovery")
}

// handleHeartbeatCommand processes heartbeat subcommands
func handleHeartbeatCommand(args []string) {
	opts, err := ParseHeartbeatCommand(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		printHeartbeatUsage()
		os.Exit(1)
	}

	// Load minimal config
	wd, _ := os.Getwd()
	configFilePath := filepath.Join(wd, "config.yaml")
	cfg, err := config.LoadConfigOptional(configFilePath, false)
	if err != nil {
		// If config.yaml is not found or invalid, proceed with a default empty config.
		// This allows CLI commands to run without a full server config.
		cfg = &config.Config{}
	}

	// Initialize simplified memory manager for heartbeat (needed for quotas/discovery)
	// We use the basic config just to get paths right
	home, err := os.UserHomeDir()
	var memoryBaseDir string
	if err != nil {
		// Fallback to current directory if home directory is not accessible
		memoryBaseDir = filepath.Join(wd, ".switchailocal", "memory")
	} else {
		memoryBaseDir = filepath.Join(home, ".switchailocal", "memory")
	}

	memConfig := &memory.MemoryConfig{
		Enabled:       true,
		BaseDir:       memoryBaseDir,
		RetentionDays: 90,
		MaxLogSizeMB:  100,
	}
	memManager, err := memory.NewMemoryManager(memConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Failed to initialize memory system: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Quotas and discovery features will be limited.\n")
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Run 'switchAILocal memory init' to initialize the memory system.\n\n")
	} else {
		defer memManager.Close()
	}

	// Initialize Heartbeat Monitor
	// Since we are running in CLI mode, we create a temporary monitor instance to run checks.
	// NOTE: This runs checks LOCALLY. It does not query the running background server.
	// If the server is managing state (like rate limits), this local check might not see specific server-state.
	// But it verifies connectivity and current status.

	monitorConfig := heartbeat.DefaultHeartbeatConfig()
	// Update with config from file if strictly needed

	monitor := heartbeat.NewHeartbeatMonitor(monitorConfig)

	// Register Checkers
	registerCheckers(monitor, cfg)

	switch opts.Command {
	case HeartbeatStatus:
		doHeartbeatStatus(monitor)
	case HeartbeatCheck:
		doHeartbeatCheck(monitor, opts.Provider)
	case HeartbeatQuota:
		doHeartbeatQuota(monitor)
	case HeartbeatDiscover:
		doHeartbeatDiscover(monitor, memManager)
	default:
		fmt.Printf("Unknown command: %s\n", opts.Command)
		printHeartbeatUsage()
		os.Exit(1)
	}
}

func registerCheckers(monitor heartbeat.HeartbeatMonitor, cfg *config.Config) {
	// Register Ollama checker if enabled
	if cfg != nil && cfg.Ollama.Enabled {
		_ = monitor.RegisterChecker(heartbeat.NewOllamaHealthChecker(cfg.Ollama.BaseURL))
	} else {
		// Fallback to default Ollama
		_ = monitor.RegisterChecker(heartbeat.NewOllamaHealthChecker("http://localhost:11434"))
	}

	// Register CLI checkers (always available)
	_ = monitor.RegisterChecker(heartbeat.NewGeminiCLIHealthChecker("gemini"))
	_ = monitor.RegisterChecker(heartbeat.NewClaudeCLIHealthChecker("claude"))

	// Register API checkers from config
	if cfg != nil {
		// SwitchAI API checkers
		if len(cfg.SwitchAIKey) > 0 {
			for _, switchaiCfg := range cfg.SwitchAIKey {
				if switchaiCfg.APIKey != "" && !strings.Contains(switchaiCfg.APIKey, "placeholder") {
					_ = monitor.RegisterChecker(heartbeat.NewSwitchAIHealthChecker(switchaiCfg.APIKey, switchaiCfg.BaseURL))
				}
			}
		}

		// Gemini API checkers
		if len(cfg.GeminiKey) > 0 {
			for _, geminiCfg := range cfg.GeminiKey {
				if geminiCfg.APIKey != "" && !strings.Contains(geminiCfg.APIKey, "placeholder") {
					_ = monitor.RegisterChecker(heartbeat.NewGeminiAPIHealthChecker(geminiCfg.APIKey))
				}
			}
		}

		// Claude API checkers
		if len(cfg.ClaudeKey) > 0 {
			for _, claudeCfg := range cfg.ClaudeKey {
				if claudeCfg.APIKey != "" && !strings.Contains(claudeCfg.APIKey, "placeholder") {
					_ = monitor.RegisterChecker(heartbeat.NewClaudeAPIHealthChecker(claudeCfg.APIKey))
				}
			}
		}

		// OpenAI compatibility providers (including Groq)
		if len(cfg.OpenAICompatibility) > 0 {
			for _, provider := range cfg.OpenAICompatibility {
				if len(provider.APIKeyEntries) > 0 {
					for _, keyEntry := range provider.APIKeyEntries {
						if keyEntry.APIKey != "" && !strings.Contains(keyEntry.APIKey, "placeholder") {
							if provider.Name == "groq" {
								_ = monitor.RegisterChecker(heartbeat.NewGroqHealthChecker(keyEntry.APIKey, provider.BaseURL))
							} else {
								_ = monitor.RegisterChecker(heartbeat.NewOpenAICompatibilityHealthChecker(provider.Name, provider.BaseURL, keyEntry.APIKey))
							}
						}
					}
				}
			}
		}
	}

	// Fallback: Register API checkers from environment variables if not in config
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		_ = monitor.RegisterChecker(heartbeat.NewOpenAIHealthChecker(key))
	}

	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		_ = monitor.RegisterChecker(heartbeat.NewGeminiAPIHealthChecker(key))
	}

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		_ = monitor.RegisterChecker(heartbeat.NewClaudeAPIHealthChecker(key))
	}
}

func doHeartbeatStatus(monitor heartbeat.HeartbeatMonitor) {
	fmt.Println("Checking provider status...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := monitor.CheckAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: Failed to check provider status: %v\n", err)
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Ensure providers are configured and accessible.\n")
		os.Exit(1)
	}

	statuses := monitor.GetAllStatuses()

	if len(statuses) == 0 {
		fmt.Println("No providers registered for monitoring.")
		fmt.Println("\n💡 Tip: Providers are auto-registered on first use.")
		fmt.Println("   Start the server and make some requests to register providers.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tSTATUS\tLATENCY\tMODELS\tMESSAGE")
	fmt.Fprintln(w, "--------\t------\t-------\t------\t-------")

	for _, s := range statuses {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", s.Provider, s.Status, s.ResponseTime, s.ModelsCount, s.ErrorMessage)
	}
	w.Flush()
}

func doHeartbeatCheck(monitor heartbeat.HeartbeatMonitor, provider string) {
	fmt.Printf("Checking %s...\n", provider)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := monitor.CheckProvider(ctx, provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: Failed to check provider '%s': %v\n", provider, err)
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Verify the provider name and ensure it's configured.\n")
		fmt.Fprintf(os.Stderr, "   Available providers: gemini, claude, openai, ollama\n")
		os.Exit(1)
	}

	fmt.Printf("Status: %s\n", status.Status)
	fmt.Printf("Latency: %s\n", status.ResponseTime)
	if status.ErrorMessage != "" {
		fmt.Printf("Error: %s\n", status.ErrorMessage)
	}
	if len(status.Metadata) > 0 {
		fmt.Println("Metadata:")
		for k, v := range status.Metadata {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
}

func doHeartbeatQuota(monitor heartbeat.HeartbeatMonitor) {
	fmt.Println("Checking quotas...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Force check to update quotas (since quotas come from headers during checks mostly)
	if err := monitor.CheckAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: Failed to check quotas: %v\n", err)
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Ensure providers are configured with valid API keys.\n")
		os.Exit(1)
	}

	statuses := monitor.GetAllStatuses()

	if len(statuses) == 0 {
		fmt.Println("No providers registered for quota monitoring.")
		fmt.Println("\n💡 Tip: Providers are auto-registered on first use.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tQUOTA USED\tLIMIT\tRESET")
	fmt.Fprintln(w, "--------\t----------\t-----\t-----")

	hasQuotaInfo := false
	for _, s := range statuses {
		if s.QuotaLimit > 0 {
			fmt.Fprintf(w, "%s\t%.0f\t%.0f\t-\n", s.Provider, s.QuotaUsed, s.QuotaLimit)
			hasQuotaInfo = true
		} else {
			fmt.Fprintf(w, "%s\tN/A\tN/A\t-\n", s.Provider)
		}
	}
	w.Flush()

	if !hasQuotaInfo {
		fmt.Println("\n💡 Note: Quota information is only available for API-based providers.")
		fmt.Println("   CLI-based providers (gemini-cli, claude-cli) don't report quotas.")
	}
}

func doHeartbeatDiscover(monitor heartbeat.HeartbeatMonitor, mem memory.MemoryManager) {
	if mem == nil {
		fmt.Fprintf(os.Stderr, "❌ Error: Memory system not initialized\n")
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Run 'switchAILocal memory init' first to enable model discovery.\n")
		os.Exit(1)
	}

	fmt.Println("Discovering models...")
	discovery := heartbeat.NewModelDiscovery(mem)

	// We can hook a callback to print as we find them
	discovery.SetEventCallback(func(event *heartbeat.HeartbeatEvent) {
		if event.Type == heartbeat.EventModelDiscovered {
			fmt.Printf("✓ Discovered %d models for %s\n", event.Data["count"], event.Provider)
		}
	})

	// Currently only Ollama supported in discovery tool
	models, err := discovery.DiscoverOllamaModels("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: Failed to discover Ollama models: %v\n", err)
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Ensure Ollama is running on http://localhost:11434\n")
		fmt.Fprintf(os.Stderr, "   Start Ollama with: ollama serve\n")
		os.Exit(1)
	}

	if len(models) == 0 {
		fmt.Println("No models found.")
		fmt.Println("\n💡 Tip: Pull some models first:")
		fmt.Println("   ollama pull llama2")
		fmt.Println("   ollama pull codellama")
		return
	}

	fmt.Printf("\nFound %d models:\n", len(models))
	for _, m := range models {
		fmt.Printf("  - %s (%s)\n", m.ModelID, formatBytes(m.Size))
	}
}
