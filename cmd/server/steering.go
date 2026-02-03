package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/steering"
)

// SteeringCommand represents available steering subcommands
type SteeringCommand string

const (
	SteeringList     SteeringCommand = "list"
	SteeringTest     SteeringCommand = "test"
	SteeringReload   SteeringCommand = "reload"
	SteeringValidate SteeringCommand = "validate"
)

// SteeringOptions holds the command-line options for steering commands
type SteeringOptions struct {
	Command  SteeringCommand
	RuleFile string
	Intent   string
	User     string
	Hour     int
	Format   string
}

// ParseSteeringCommand parses command arguments
func ParseSteeringCommand(args []string) (*SteeringOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("missing subcommand")
	}

	cmd := SteeringCommand(args[0])
	opts := &SteeringOptions{Command: cmd}
	flagSet := flag.NewFlagSet("steering", flag.ExitOnError)

	flagSet.StringVar(&opts.RuleFile, "file", "", "Specific rule file to test/validate")
	flagSet.StringVar(&opts.Intent, "intent", "", "Intent to test against")
	flagSet.StringVar(&opts.User, "user", "", "User hash to test against")
	flagSet.IntVar(&opts.Hour, "hour", time.Now().Hour(), "Hour to test against")
	flagSet.StringVar(&opts.Format, "format", "table", "Output format (table/json)")

	if err := flagSet.Parse(args[1:]); err != nil {
		return nil, err
	}

	return opts, nil
}

func printSteeringUsage() {
	fmt.Println("Usage: switchAILocal steering <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  list           List all active steering rules")
	fmt.Println("  test           Test rule matching against a simulated context")
	fmt.Println("  reload         Force reload of all rules (if server is running)")
	fmt.Println("  validate       Validate specific rule file or all rules")
	fmt.Println("\nOptions:")
	fmt.Println("  --file <path>  Target specific rule file")
	fmt.Println("  --intent <str> Simulated intent for test")
	fmt.Println("  --user <str>   Simulated user hash for test")
	fmt.Println("  --hour <int>   Simulated hour (0-23) for test")
	fmt.Println("  --format <str> Output format: table (default) or json")
}

// handleSteeringCommand processes steering subcommands
func handleSteeringCommand(args []string) {
	opts, err := ParseSteeringCommand(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		printSteeringUsage()
		os.Exit(1)
	}

	wd, _ := os.Getwd()
	configFilePath := filepath.Join(wd, "config.yaml")
	cfg, err := config.LoadConfigOptional(configFilePath, false)
	if err != nil {
		cfg = &config.Config{}
	}

	switch cmd := opts.Command; cmd {
	case SteeringList:
		doSteeringList(cfg, opts)
	case SteeringTest:
		doSteeringTest(cfg, opts)
	case SteeringReload:
		doSteeringReload(cfg, opts)
	case SteeringValidate:
		doSteeringValidate(cfg, opts)
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printSteeringUsage()
		os.Exit(1)
	}
}

func getSteeringDir(cfg *config.Config) string {
	// Use AuthDir + steering subdirectory if available
	if cfg != nil && cfg.AuthDir != "" {
		return filepath.Join(cfg.AuthDir, "steering")
	}

	// Default to user home directory + .switchailocal/steering
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home directory is not accessible
		return ".switchailocal/steering"
	}
	return filepath.Join(home, ".switchailocal", "steering")
}

func doSteeringList(cfg *config.Config, opts *SteeringOptions) {
	engine, err := steering.NewSteeringEngine(getSteeringDir(cfg))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := engine.LoadRules(); err != nil {
		fmt.Printf("Error loading rules: %v\n", err)
		return
	}

	rules := engine.GetRules()
	if opts.Format == "json" {
		data, _ := json.MarshalIndent(rules, "", "  ")
		fmt.Println(string(data))
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPRIORITY\tCONDITION\tPRIMARY MODEL\tFILE")
	for _, r := range rules {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			r.Name, r.Activation.Priority, r.Activation.Condition, r.Preferences.PrimaryModel, filepath.Base(r.FilePath))
	}
	w.Flush()
}

func doSteeringTest(cfg *config.Config, opts *SteeringOptions) {
	engine, err := steering.NewSteeringEngine(getSteeringDir(cfg))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := engine.LoadRules(); err != nil {
		fmt.Printf("Error loading rules: %v\n", err)
		return
	}

	ctx := &steering.RoutingContext{
		Intent:    opts.Intent,
		Hour:      opts.Hour,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
	if opts.User != "" {
		ctx.APIKeyHash = opts.User
	}

	matches, err := engine.FindMatchingRules(ctx)
	if err != nil {
		fmt.Printf("Error finding matches: %v\n", err)
		return
	}

	fmt.Printf("Simulated Context: Intent=%s, Hour=%d, User=%s\n", ctx.Intent, ctx.Hour, ctx.APIKeyHash)
	fmt.Printf("Found %d matching rules:\n\n", len(matches))

	if len(matches) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PRIORITY\tNAME\tCONDITION\tACTION")
		for _, r := range matches {
			action := "Inject Context"
			if r.Preferences.PrimaryModel != "" {
				action = "Route to " + r.Preferences.PrimaryModel
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.Activation.Priority, r.Name, r.Activation.Condition, action)
		}
		w.Flush()

		// Show final applied decision
		model, messages, _ := engine.ApplySteering(ctx, nil, nil, matches)
		fmt.Printf("\nFinal Decision:\n")
		fmt.Printf("  Selected Model: %s\n", model)
		if len(messages) > 0 {
			fmt.Printf("  Injected System Prompt: %s\n", messages[0]["content"])
		}
	}
}

func doSteeringReload(cfg *config.Config, opts *SteeringOptions) {
	// In a real CLI, this might send a signal to a running process or just verify paths
	fmt.Println("Reloading rules requires a running server process. Use 'switchAILocal steering list' to verify currently loaded files.")
}

func doSteeringValidate(cfg *config.Config, opts *SteeringOptions) {
	// Similar to load, but specifically check for syntax errors
	fmt.Println("Validating rules...")
	engine, err := steering.NewSteeringEngine(getSteeringDir(cfg))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := engine.LoadRules(); err != nil {
		fmt.Printf("Validation FAILED: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("All rules in %s are valid.\n", getSteeringDir(cfg))
}
