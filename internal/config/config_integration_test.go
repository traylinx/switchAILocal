package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMemoryConfigDefaults verifies that Memory configuration defaults are applied correctly.
func TestMemoryConfigDefaults(t *testing.T) {
	// Create a minimal config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
host: "127.0.0.1"
port: 8080
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify Memory defaults
	if cfg.Memory.Enabled {
		t.Error("Memory should be disabled by default")
	}
	if cfg.Memory.BaseDir != ".switchailocal/memory" {
		t.Errorf("Expected BaseDir '.switchailocal/memory', got '%s'", cfg.Memory.BaseDir)
	}
	if cfg.Memory.RetentionDays != 90 {
		t.Errorf("Expected RetentionDays 90, got %d", cfg.Memory.RetentionDays)
	}
	if cfg.Memory.MaxLogSizeMB != 100 {
		t.Errorf("Expected MaxLogSizeMB 100, got %d", cfg.Memory.MaxLogSizeMB)
	}
	if !cfg.Memory.Compression {
		t.Error("Compression should be enabled by default")
	}
}

// TestSteeringConfigDefaults verifies that Steering configuration defaults are applied correctly.
func TestSteeringConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
host: "127.0.0.1"
port: 8080
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify Steering defaults
	if cfg.Steering.Enabled {
		t.Error("Steering should be disabled by default")
	}
	if cfg.Steering.RulesDir != ".switchailocal/steering" {
		t.Errorf("Expected RulesDir '.switchailocal/steering', got '%s'", cfg.Steering.RulesDir)
	}
	if !cfg.Steering.HotReload {
		t.Error("HotReload should be enabled by default")
	}
}

// TestHooksConfigDefaults verifies that Hooks configuration defaults are applied correctly.
func TestHooksConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
host: "127.0.0.1"
port: 8080
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify Hooks defaults
	if cfg.Hooks.Enabled {
		t.Error("Hooks should be disabled by default")
	}
	if cfg.Hooks.HooksDir != ".switchailocal/hooks" {
		t.Errorf("Expected HooksDir '.switchailocal/hooks', got '%s'", cfg.Hooks.HooksDir)
	}
	if !cfg.Hooks.HotReload {
		t.Error("HotReload should be enabled by default")
	}
}

// TestMemoryConfigParsing verifies that Memory configuration is parsed correctly from YAML.
func TestMemoryConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
host: "127.0.0.1"
port: 8080
memory:
  enabled: true
  base-dir: "/custom/memory/path"
  retention-days: 30
  max-log-size-mb: 50
  compression: false
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify Memory configuration was parsed
	if !cfg.Memory.Enabled {
		t.Error("Memory should be enabled")
	}
	if cfg.Memory.BaseDir != "/custom/memory/path" {
		t.Errorf("Expected BaseDir '/custom/memory/path', got '%s'", cfg.Memory.BaseDir)
	}
	if cfg.Memory.RetentionDays != 30 {
		t.Errorf("Expected RetentionDays 30, got %d", cfg.Memory.RetentionDays)
	}
	if cfg.Memory.MaxLogSizeMB != 50 {
		t.Errorf("Expected MaxLogSizeMB 50, got %d", cfg.Memory.MaxLogSizeMB)
	}
	if cfg.Memory.Compression {
		t.Error("Compression should be disabled")
	}
}

// TestSteeringConfigParsing verifies that Steering configuration is parsed correctly from YAML.
func TestSteeringConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
host: "127.0.0.1"
port: 8080
steering:
  enabled: true
  rules-dir: "/custom/steering/rules"
  hot-reload: false
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify Steering configuration was parsed
	if !cfg.Steering.Enabled {
		t.Error("Steering should be enabled")
	}
	if cfg.Steering.RulesDir != "/custom/steering/rules" {
		t.Errorf("Expected RulesDir '/custom/steering/rules', got '%s'", cfg.Steering.RulesDir)
	}
	if cfg.Steering.HotReload {
		t.Error("HotReload should be disabled")
	}
}

// TestHooksConfigParsing verifies that Hooks configuration is parsed correctly from YAML.
func TestHooksConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
host: "127.0.0.1"
port: 8080
hooks:
  enabled: true
  hooks-dir: "/custom/hooks/path"
  hot-reload: false
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify Hooks configuration was parsed
	if !cfg.Hooks.Enabled {
		t.Error("Hooks should be enabled")
	}
	if cfg.Hooks.HooksDir != "/custom/hooks/path" {
		t.Errorf("Expected HooksDir '/custom/hooks/path', got '%s'", cfg.Hooks.HooksDir)
	}
	if cfg.Hooks.HotReload {
		t.Error("HotReload should be disabled")
	}
}

// TestSanitizeMemory verifies that Memory configuration sanitization works correctly.
func TestSanitizeMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    MemoryConfig
		expected MemoryConfig
	}{
		{
			name: "Empty BaseDir gets default",
			input: MemoryConfig{
				Enabled:       true,
				BaseDir:       "",
				RetentionDays: 90,
				MaxLogSizeMB:  100,
				Compression:   true,
			},
			expected: MemoryConfig{
				Enabled:       true,
				BaseDir:       ".switchailocal/memory",
				RetentionDays: 90,
				MaxLogSizeMB:  100,
				Compression:   true,
			},
		},
		{
			name: "Negative RetentionDays becomes 0",
			input: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: -10,
				MaxLogSizeMB:  100,
				Compression:   true,
			},
			expected: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 0,
				MaxLogSizeMB:  100,
				Compression:   true,
			},
		},
		{
			name: "Excessive RetentionDays capped at 3650",
			input: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 5000,
				MaxLogSizeMB:  100,
				Compression:   true,
			},
			expected: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 3650,
				MaxLogSizeMB:  100,
				Compression:   true,
			},
		},
		{
			name: "Negative MaxLogSizeMB becomes 0",
			input: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 90,
				MaxLogSizeMB:  -50,
				Compression:   true,
			},
			expected: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 90,
				MaxLogSizeMB:  0,
				Compression:   true,
			},
		},
		{
			name: "Excessive MaxLogSizeMB capped at 10000",
			input: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 90,
				MaxLogSizeMB:  20000,
				Compression:   true,
			},
			expected: MemoryConfig{
				Enabled:       true,
				BaseDir:       "/custom/path",
				RetentionDays: 90,
				MaxLogSizeMB:  10000,
				Compression:   true,
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Memory: tt.input}
			cfg.SanitizeMemory()
			
			if cfg.Memory.BaseDir != tt.expected.BaseDir {
				t.Errorf("BaseDir: expected '%s', got '%s'", tt.expected.BaseDir, cfg.Memory.BaseDir)
			}
			if cfg.Memory.RetentionDays != tt.expected.RetentionDays {
				t.Errorf("RetentionDays: expected %d, got %d", tt.expected.RetentionDays, cfg.Memory.RetentionDays)
			}
			if cfg.Memory.MaxLogSizeMB != tt.expected.MaxLogSizeMB {
				t.Errorf("MaxLogSizeMB: expected %d, got %d", tt.expected.MaxLogSizeMB, cfg.Memory.MaxLogSizeMB)
			}
		})
	}
}

// TestSanitizeSteering verifies that Steering configuration sanitization works correctly.
func TestSanitizeSteering(t *testing.T) {
	tests := []struct {
		name     string
		input    SteeringConfig
		expected SteeringConfig
	}{
		{
			name: "Empty RulesDir gets default",
			input: SteeringConfig{
				Enabled:   true,
				RulesDir:  "",
				HotReload: true,
			},
			expected: SteeringConfig{
				Enabled:     true,
				RulesDir:    ".switchailocal/steering",
				SteeringDir: ".switchailocal/steering",
				HotReload:   true,
			},
		},
		{
			name: "SteeringDir used when RulesDir empty",
			input: SteeringConfig{
				Enabled:     true,
				SteeringDir: "/custom/steering",
				RulesDir:    "",
				HotReload:   true,
			},
			expected: SteeringConfig{
				Enabled:     true,
				RulesDir:    "/custom/steering",
				SteeringDir: "/custom/steering",
				HotReload:   true,
			},
		},
		{
			name: "RulesDir synced to SteeringDir",
			input: SteeringConfig{
				Enabled:   true,
				RulesDir:  "/custom/rules",
				HotReload: false,
			},
			expected: SteeringConfig{
				Enabled:     true,
				RulesDir:    "/custom/rules",
				SteeringDir: "/custom/rules",
				HotReload:   false,
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Steering: tt.input}
			cfg.SanitizeSteering()
			
			if cfg.Steering.RulesDir != tt.expected.RulesDir {
				t.Errorf("RulesDir: expected '%s', got '%s'", tt.expected.RulesDir, cfg.Steering.RulesDir)
			}
			if cfg.Steering.SteeringDir != tt.expected.SteeringDir {
				t.Errorf("SteeringDir: expected '%s', got '%s'", tt.expected.SteeringDir, cfg.Steering.SteeringDir)
			}
		})
	}
}

// TestSanitizeHooks verifies that Hooks configuration sanitization works correctly.
func TestSanitizeHooks(t *testing.T) {
	tests := []struct {
		name     string
		input    HooksConfig
		expected HooksConfig
	}{
		{
			name: "Empty HooksDir gets default",
			input: HooksConfig{
				Enabled:   true,
				HooksDir:  "",
				HotReload: true,
			},
			expected: HooksConfig{
				Enabled:   true,
				HooksDir:  ".switchailocal/hooks",
				HotReload: true,
			},
		},
		{
			name: "Custom HooksDir preserved",
			input: HooksConfig{
				Enabled:   true,
				HooksDir:  "/custom/hooks",
				HotReload: false,
			},
			expected: HooksConfig{
				Enabled:   true,
				HooksDir:  "/custom/hooks",
				HotReload: false,
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Hooks: tt.input}
			cfg.SanitizeHooks()
			
			if cfg.Hooks.HooksDir != tt.expected.HooksDir {
				t.Errorf("HooksDir: expected '%s', got '%s'", tt.expected.HooksDir, cfg.Hooks.HooksDir)
			}
		})
	}
}
