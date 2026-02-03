package memory_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/traylinx/switchAILocal/internal/memory"
)

// ExampleDirectoryStructure_Initialize demonstrates how to initialize the memory system directory structure.
func ExampleDirectoryStructure_Initialize() {
	// Create a temporary directory for this example
	tmpDir, err := os.MkdirTemp("", "memory-example-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create directory structure manager
	baseDir := filepath.Join(tmpDir, ".switchailocal", "memory")
	ds := memory.NewDirectoryStructure(baseDir)

	// Initialize the directory structure
	if err := ds.Initialize(); err != nil {
		log.Fatalf("Failed to initialize memory structure: %v", err)
	}

	// Validate the structure
	if err := ds.Validate(); err != nil {
		log.Fatalf("Validation failed: %v", err)
	}

	fmt.Println("Memory system directory structure initialized successfully")

	// Output:
	// Memory system directory structure initialized successfully
}

// Example_getPaths demonstrates how to get paths to various memory system components.
func Example_getPaths() {
	// Create a temporary directory for this example
	tmpDir, err := os.MkdirTemp("", "memory-example-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and initialize directory structure
	baseDir := filepath.Join(tmpDir, ".switchailocal", "memory")
	ds := memory.NewDirectoryStructure(baseDir)

	if err := ds.Initialize(); err != nil {
		log.Fatal(err)
	}

	// Get paths to various components
	fmt.Println("Routing history:", filepath.Base(ds.GetRoutingHistoryPath()))
	fmt.Println("Provider quirks:", filepath.Base(ds.GetProviderQuirksPath()))
	fmt.Println("User preferences dir:", filepath.Base(ds.GetUserPreferencesDir()))
	fmt.Println("Daily logs dir:", filepath.Base(ds.GetDailyLogsDir()))
	fmt.Println("Analytics dir:", filepath.Base(ds.GetAnalyticsDir()))

	// Output:
	// Routing history: routing-history.jsonl
	// Provider quirks: provider-quirks.md
	// User preferences dir: user-preferences
	// Daily logs dir: daily
	// Analytics dir: analytics
}

// Example_defaultConfig demonstrates the default memory configuration.
func Example_defaultConfig() {
	config := memory.DefaultMemoryConfig()

	fmt.Printf("Enabled: %v\n", config.Enabled)
	fmt.Printf("Base directory: %s\n", config.BaseDir)
	fmt.Printf("Retention days: %d\n", config.RetentionDays)
	fmt.Printf("Max log size (MB): %d\n", config.MaxLogSizeMB)
	fmt.Printf("Compression: %v\n", config.Compression)

	// Output:
	// Enabled: false
	// Base directory: .switchailocal/memory
	// Retention days: 90
	// Max log size (MB): 100
	// Compression: true
}
