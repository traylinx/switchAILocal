package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/learning"
	"github.com/traylinx/switchAILocal/internal/memory"
)

// LearningCommand represents the available learning subcommands
type LearningCommand string

const (
	LearningStatus  LearningCommand = "status"
	LearningAnalyze LearningCommand = "analyze"
	LearningApply   LearningCommand = "apply"
	LearningReset   LearningCommand = "reset"
)

// LearningOptions holds command-line options
type LearningOptions struct {
	Command  LearningCommand
	UserHash string
}

func handleLearningCommand(args []string) {
	if len(args) == 0 {
		printLearningUsage()
		os.Exit(1)
	}

	command := LearningCommand(args[0])

	// Flags for subcommands
	var userHash string

	fs := flag.NewFlagSet("learning", flag.ExitOnError)
	fs.StringVar(&userHash, "user", "", "User API Key Hash for analysis")

	// Parse flags starting from args[1] if available
	if len(args) > 1 {
		if err := fs.Parse(args[1:]); err != nil {
			_ = err // ExitOnError is set
		}
	}

	opts := &LearningOptions{
		Command:  command,
		UserHash: userHash,
	}

	// Load config similar to memory command
	cfg := loadMinimalConfig()

	DoLearningCommand(cfg, opts)
}

func printLearningUsage() {
	fmt.Println("Usage: switchAILocal learning <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  status             Show learning engine status")
	fmt.Println("  analyze --user <id>  Run analysis for a specific user")
	fmt.Println("  apply --user <id>    Apply learned preferences for a user")
	fmt.Println("  reset --user <id>    Reset learned preferences for a user")
}

func DoLearningCommand(cfg *config.Config, opts *LearningOptions) {
	switch opts.Command {
	case LearningStatus:
		doLearningStatus(cfg)
	case LearningAnalyze:
		doLearningAnalyze(cfg, opts)
	case LearningApply:
		doLearningApply(cfg, opts)
	case LearningReset:
		doLearningReset(cfg, opts)
	default:
		printLearningUsage()
		os.Exit(1)
	}
}

func doLearningApply(cfg *config.Config, opts *LearningOptions) {
	if opts.UserHash == "" {
		fmt.Println("Error: --user is required for apply")
		os.Exit(1)
	}

	engine, mem := initEngine(cfg)
	defer mem.Close()

	fmt.Printf("Analyzing history for user: %s\n", opts.UserHash)
	result, err := engine.AnalyzeUser(context.Background(), opts.UserHash)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	if result.NewPreferences == nil {
		fmt.Println("No new preferences generated (insufficient data or no patterns).")
		return
	}

	fmt.Println("Applying learned preferences...")
	if err := engine.ApplyPreferences(result.NewPreferences); err != nil {
		log.Fatalf("Failed to apply preferences: %v", err)
	}
	fmt.Println("✓ Successfully applied learned preferences.")
}

func doLearningReset(cfg *config.Config, opts *LearningOptions) {
	if opts.UserHash == "" {
		fmt.Println("Error: --user is required for reset")
		os.Exit(1)
	}

	_, mem := initEngine(cfg)
	defer mem.Close()

	fmt.Printf("Resetting preferences for user: %s\n", opts.UserHash)
	if err := mem.DeleteUserPreferences(opts.UserHash); err != nil {
		log.Fatalf("Failed to reset preferences: %v", err)
	}
	fmt.Println("✓ Successfully reset preferences.")
}

func doLearningStatus(cfg *config.Config) {
	fmt.Println("Learning Engine Status")
	fmt.Println("======================")

	if cfg.SDKConfig.Intelligence.Learning.Enabled {
		fmt.Println("Status: ENABLED")
	} else {
		fmt.Println("Status: DISABLED")
	}

	fmt.Printf("Min Sample Size: %d\n", cfg.SDKConfig.Intelligence.Learning.MinSampleSize)
	fmt.Printf("Confidence Threshold: %.2f\n", cfg.SDKConfig.Intelligence.Learning.ConfidenceThreshold)
}

func doLearningAnalyze(cfg *config.Config, opts *LearningOptions) {
	if opts.UserHash == "" {
		fmt.Println("Error: --user is required for analysis")
		os.Exit(1)
	}

	engine, mem := initEngine(cfg)
	defer mem.Close()

	fmt.Printf("Analyzing history for user: %s\n", opts.UserHash)
	result, err := engine.AnalyzeUser(context.Background(), opts.UserHash)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	fmt.Printf("Analyzed %d requests.\n", result.RequestsAnalyzed)

	if result.NewPreferences != nil {
		fmt.Println("\nLearned Preferences:")
		for intent, pref := range result.NewPreferences.ModelPreferences {
			fmt.Printf("- Intent '%s': Prefers %s (Conf: %.2f, Success: %.0f%%)\n",
				intent, pref.Model, pref.Confidence, pref.SuccessRate*100)
		}

		fmt.Println("\nComputed Bias:")
		for provider, bias := range result.NewPreferences.ProviderBias {
			fmt.Printf("- %s: %.2f\n", provider, bias)
		}
	}

	if len(result.Suggestions) > 0 {
		fmt.Println("\nSuggestions:")
		for _, s := range result.Suggestions {
			fmt.Printf("- %s\n", s)
		}
	} else {
		fmt.Println("\nNo suggestions generated.")
	}
}

func initEngine(cfg *config.Config) (*learning.LearningEngine, memory.MemoryManager) {
	memConfig := &memory.MemoryConfig{
		Enabled: true,
		BaseDir: getMemoryBaseDir(cfg),
	}

	mem, err := memory.NewMemoryManager(memConfig)
	if err != nil {
		log.Fatalf("Failed to init memory: %v", err)
	}

	engine, err := learning.NewLearningEngine(&cfg.SDKConfig.Intelligence.Learning, mem)
	if err != nil {
		mem.Close()
		log.Fatalf("Failed to init learning engine: %v", err)
	}
	return engine, mem
}

func loadMinimalConfig() *config.Config {
	wd, _ := os.Getwd()
	configFilePath := filepath.Join(wd, "config.yaml")
	cfg, err := config.LoadConfigOptional(configFilePath, false)
	if err != nil || cfg == nil {
		return &config.Config{}
	}
	return cfg
}
