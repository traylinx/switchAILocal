package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewQuirksStore(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	// Create initial file
	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	// Create quirks store
	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

func TestAddQuirk(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	// Initialize directory structure
	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout on first request after idle",
		Workaround: "Send warmup request on startup",
		Discovered: time.Now(),
		Frequency:  "3/10 startups",
		Severity:   "medium",
	}

	err = store.AddQuirk(quirk)
	if err != nil {
		t.Fatalf("Failed to add quirk: %v", err)
	}

	// Verify quirk was added
	quirks, err := store.GetProviderQuirks("ollama")
	if err != nil {
		t.Fatalf("Failed to get provider quirks: %v", err)
	}

	if len(quirks) != 1 {
		t.Fatalf("Expected 1 quirk, got %d", len(quirks))
	}

	if quirks[0].Issue != quirk.Issue {
		t.Errorf("Expected issue %q, got %q", quirk.Issue, quirks[0].Issue)
	}
}

func TestAddQuirk_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name    string
		quirk   *Quirk
		wantErr bool
	}{
		{
			name:    "nil quirk",
			quirk:   nil,
			wantErr: true,
		},
		{
			name: "empty provider",
			quirk: &Quirk{
				Provider:   "",
				Issue:      "test issue",
				Workaround: "test workaround",
				Severity:   "low",
			},
			wantErr: true,
		},
		{
			name: "empty issue",
			quirk: &Quirk{
				Provider:   "test",
				Issue:      "",
				Workaround: "test workaround",
				Severity:   "low",
			},
			wantErr: true,
		},
		{
			name: "empty workaround",
			quirk: &Quirk{
				Provider:   "test",
				Issue:      "test issue",
				Workaround: "",
				Severity:   "low",
			},
			wantErr: true,
		},
		{
			name: "invalid severity",
			quirk: &Quirk{
				Provider:   "test",
				Issue:      "test issue",
				Workaround: "test workaround",
				Severity:   "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid quirk",
			quirk: &Quirk{
				Provider:   "test",
				Issue:      "test issue",
				Workaround: "test workaround",
				Frequency:  "always",
				Severity:   "high",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.AddQuirk(tt.quirk)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddQuirk() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddQuirk_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout",
		Workaround: "Send warmup request",
		Frequency:  "3/10",
		Severity:   "medium",
	}

	// Add quirk first time
	if err := store.AddQuirk(quirk); err != nil {
		t.Fatalf("Failed to add quirk: %v", err)
	}

	// Add same quirk again (should not duplicate)
	if err := store.AddQuirk(quirk); err != nil {
		t.Fatalf("Failed to add duplicate quirk: %v", err)
	}

	// Verify only one quirk exists
	quirks, err := store.GetProviderQuirks("ollama")
	if err != nil {
		t.Fatalf("Failed to get provider quirks: %v", err)
	}

	if len(quirks) != 1 {
		t.Errorf("Expected 1 quirk, got %d", len(quirks))
	}
}

func TestAddQuirk_UpdateFrequency(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	quirk1 := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout",
		Workaround: "Send warmup request",
		Frequency:  "3/10",
		Severity:   "medium",
	}

	// Add quirk first time
	if err := store.AddQuirk(quirk1); err != nil {
		t.Fatalf("Failed to add quirk: %v", err)
	}

	// Update frequency
	quirk2 := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout",
		Workaround: "Send warmup request",
		Frequency:  "5/10",
		Severity:   "high",
	}

	if err := store.AddQuirk(quirk2); err != nil {
		t.Fatalf("Failed to update quirk: %v", err)
	}

	// Verify frequency was updated
	quirks, err := store.GetProviderQuirks("ollama")
	if err != nil {
		t.Fatalf("Failed to get provider quirks: %v", err)
	}

	if len(quirks) != 1 {
		t.Fatalf("Expected 1 quirk, got %d", len(quirks))
	}

	if quirks[0].Frequency != "5/10" {
		t.Errorf("Expected frequency '5/10', got %q", quirks[0].Frequency)
	}

	if quirks[0].Severity != "high" {
		t.Errorf("Expected severity 'high', got %q", quirks[0].Severity)
	}
}

func TestGetProviderQuirks(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Add multiple quirks for different providers
	quirks := []*Quirk{
		{
			Provider:   "ollama",
			Issue:      "Issue 1",
			Workaround: "Workaround 1",
			Frequency:  "always",
			Severity:   "high",
		},
		{
			Provider:   "ollama",
			Issue:      "Issue 2",
			Workaround: "Workaround 2",
			Frequency:  "sometimes",
			Severity:   "medium",
		},
		{
			Provider:   "gemini",
			Issue:      "Issue 3",
			Workaround: "Workaround 3",
			Frequency:  "rarely",
			Severity:   "low",
		},
	}

	for _, quirk := range quirks {
		if err := store.AddQuirk(quirk); err != nil {
			t.Fatalf("Failed to add quirk: %v", err)
		}
	}

	// Get ollama quirks
	ollamaQuirks, err := store.GetProviderQuirks("ollama")
	if err != nil {
		t.Fatalf("Failed to get ollama quirks: %v", err)
	}

	if len(ollamaQuirks) != 2 {
		t.Errorf("Expected 2 ollama quirks, got %d", len(ollamaQuirks))
	}

	// Get gemini quirks
	geminiQuirks, err := store.GetProviderQuirks("gemini")
	if err != nil {
		t.Fatalf("Failed to get gemini quirks: %v", err)
	}

	if len(geminiQuirks) != 1 {
		t.Errorf("Expected 1 gemini quirk, got %d", len(geminiQuirks))
	}

	// Get non-existent provider
	nonExistent, err := store.GetProviderQuirks("nonexistent")
	if err != nil {
		t.Fatalf("Failed to get non-existent provider quirks: %v", err)
	}

	if len(nonExistent) != 0 {
		t.Errorf("Expected 0 quirks for non-existent provider, got %d", len(nonExistent))
	}
}

func TestGetProviderQuirks_EmptyProvider(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	_, err = store.GetProviderQuirks("")
	if err == nil {
		t.Error("Expected error for empty provider, got nil")
	}
}

func TestGetAllQuirks(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Add quirks for multiple providers
	quirks := []*Quirk{
		{Provider: "ollama", Issue: "Issue 1", Workaround: "Workaround 1", Frequency: "always", Severity: "high"},
		{Provider: "ollama", Issue: "Issue 2", Workaround: "Workaround 2", Frequency: "sometimes", Severity: "medium"},
		{Provider: "gemini", Issue: "Issue 3", Workaround: "Workaround 3", Frequency: "rarely", Severity: "low"},
	}

	for _, quirk := range quirks {
		if err := store.AddQuirk(quirk); err != nil {
			t.Fatalf("Failed to add quirk: %v", err)
		}
	}

	// Get all quirks
	allQuirks, err := store.GetAllQuirks()
	if err != nil {
		t.Fatalf("Failed to get all quirks: %v", err)
	}

	if len(allQuirks) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(allQuirks))
	}

	if len(allQuirks["ollama"]) != 2 {
		t.Errorf("Expected 2 ollama quirks, got %d", len(allQuirks["ollama"]))
	}

	if len(allQuirks["gemini"]) != 1 {
		t.Errorf("Expected 1 gemini quirk, got %d", len(allQuirks["gemini"]))
	}
}

func TestGetQuirksBySeverity(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Add quirks with different severities
	quirks := []*Quirk{
		{Provider: "ollama", Issue: "Issue 1", Workaround: "Workaround 1", Frequency: "always", Severity: "critical"},
		{Provider: "gemini", Issue: "Issue 2", Workaround: "Workaround 2", Frequency: "sometimes", Severity: "high"},
		{Provider: "claude", Issue: "Issue 3", Workaround: "Workaround 3", Frequency: "rarely", Severity: "high"},
		{Provider: "openai", Issue: "Issue 4", Workaround: "Workaround 4", Frequency: "never", Severity: "low"},
	}

	for _, quirk := range quirks {
		if err := store.AddQuirk(quirk); err != nil {
			t.Fatalf("Failed to add quirk: %v", err)
		}
	}

	// Get high severity quirks
	highQuirks, err := store.GetQuirksBySeverity("high")
	if err != nil {
		t.Fatalf("Failed to get high severity quirks: %v", err)
	}

	if len(highQuirks) != 2 {
		t.Errorf("Expected 2 high severity quirks, got %d", len(highQuirks))
	}

	// Get critical severity quirks
	criticalQuirks, err := store.GetQuirksBySeverity("critical")
	if err != nil {
		t.Fatalf("Failed to get critical severity quirks: %v", err)
	}

	if len(criticalQuirks) != 1 {
		t.Errorf("Expected 1 critical severity quirk, got %d", len(criticalQuirks))
	}
}

func TestGetQuirksByFrequency(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Add quirks with different frequencies
	quirks := []*Quirk{
		{Provider: "ollama", Issue: "Issue 1", Workaround: "Workaround 1", Frequency: "daily during peak", Severity: "high"},
		{Provider: "gemini", Issue: "Issue 2", Workaround: "Workaround 2", Frequency: "3/10 startups", Severity: "medium"},
		{Provider: "claude", Issue: "Issue 3", Workaround: "Workaround 3", Frequency: "rarely", Severity: "low"},
	}

	for _, quirk := range quirks {
		if err := store.AddQuirk(quirk); err != nil {
			t.Fatalf("Failed to add quirk: %v", err)
		}
	}

	// Search for "daily" frequency
	dailyQuirks, err := store.GetQuirksByFrequency("daily")
	if err != nil {
		t.Fatalf("Failed to get daily quirks: %v", err)
	}

	if len(dailyQuirks) != 1 {
		t.Errorf("Expected 1 daily quirk, got %d", len(dailyQuirks))
	}

	// Search for "startup" frequency
	startupQuirks, err := store.GetQuirksByFrequency("startup")
	if err != nil {
		t.Fatalf("Failed to get startup quirks: %v", err)
	}

	if len(startupQuirks) != 1 {
		t.Errorf("Expected 1 startup quirk, got %d", len(startupQuirks))
	}
}

func TestApplyWorkaround(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout on first request",
		Workaround: "Send warmup request on startup",
		Frequency:  "3/10 startups",
		Severity:   "medium",
	}

	if err := store.AddQuirk(quirk); err != nil {
		t.Fatalf("Failed to add quirk: %v", err)
	}

	// Test exact match
	workaround, err := store.ApplyWorkaround("ollama", "Connection timeout on first request")
	if err != nil {
		t.Fatalf("Failed to apply workaround: %v", err)
	}

	if workaround != "Send warmup request on startup" {
		t.Errorf("Expected workaround %q, got %q", "Send warmup request on startup", workaround)
	}

	// Test partial match
	workaround, err = store.ApplyWorkaround("ollama", "Connection timeout")
	if err != nil {
		t.Fatalf("Failed to apply workaround: %v", err)
	}

	if workaround != "Send warmup request on startup" {
		t.Errorf("Expected workaround %q, got %q", "Send warmup request on startup", workaround)
	}

	// Test no match
	workaround, err = store.ApplyWorkaround("ollama", "Different issue")
	if err != nil {
		t.Fatalf("Failed to apply workaround: %v", err)
	}

	if workaround != "" {
		t.Errorf("Expected empty workaround, got %q", workaround)
	}

	// Test non-existent provider
	workaround, err = store.ApplyWorkaround("nonexistent", "Some issue")
	if err != nil {
		t.Fatalf("Failed to apply workaround: %v", err)
	}

	if workaround != "" {
		t.Errorf("Expected empty workaround, got %q", workaround)
	}
}

func TestApplyWorkaround_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Test empty provider
	_, err = store.ApplyWorkaround("", "issue")
	if err == nil {
		t.Error("Expected error for empty provider, got nil")
	}

	// Test empty issue
	_, err = store.ApplyWorkaround("provider", "")
	if err == nil {
		t.Error("Expected error for empty issue, got nil")
	}
}

func TestQuirksCount(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Initial count should be 0
	if count := store.Count(); count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}

	// Add quirks
	quirks := []*Quirk{
		{Provider: "ollama", Issue: "Issue 1", Workaround: "Workaround 1", Frequency: "always", Severity: "high"},
		{Provider: "ollama", Issue: "Issue 2", Workaround: "Workaround 2", Frequency: "sometimes", Severity: "medium"},
		{Provider: "gemini", Issue: "Issue 3", Workaround: "Workaround 3", Frequency: "rarely", Severity: "low"},
	}

	for _, quirk := range quirks {
		if err := store.AddQuirk(quirk); err != nil {
			t.Fatalf("Failed to add quirk: %v", err)
		}
	}

	// Count should be 3
	if count := store.Count(); count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestCountByProvider(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Add quirks
	quirks := []*Quirk{
		{Provider: "ollama", Issue: "Issue 1", Workaround: "Workaround 1", Frequency: "always", Severity: "high"},
		{Provider: "ollama", Issue: "Issue 2", Workaround: "Workaround 2", Frequency: "sometimes", Severity: "medium"},
		{Provider: "gemini", Issue: "Issue 3", Workaround: "Workaround 3", Frequency: "rarely", Severity: "low"},
	}

	for _, quirk := range quirks {
		if err := store.AddQuirk(quirk); err != nil {
			t.Fatalf("Failed to add quirk: %v", err)
		}
	}

	// Count ollama quirks
	if count := store.CountByProvider("ollama"); count != 2 {
		t.Errorf("Expected ollama count 2, got %d", count)
	}

	// Count gemini quirks
	if count := store.CountByProvider("gemini"); count != 1 {
		t.Errorf("Expected gemini count 1, got %d", count)
	}

	// Count non-existent provider
	if count := store.CountByProvider("nonexistent"); count != 0 {
		t.Errorf("Expected nonexistent count 0, got %d", count)
	}
}

func TestLoadExistingQuirks(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	// Create quirks file with existing content
	content := `# Provider Quirks & Workarounds

### Ollama
- **Issue**: Connection timeout on first request after idle
- **Workaround**: Send warmup request on startup
- **Discovered**: 2026-01-15
- **Frequency**: 3/10 startups
- **Severity**: medium

---

### Gemini CLI
- **Issue**: Rate limit 429 during peak hours (9-11 AM PST)
- **Workaround**: Route to Gemini API instead
- **Discovered**: 2026-01-20
- **Frequency**: Daily during peak
- **Severity**: high

---
`

	if err := os.WriteFile(quirksFile, []byte(content), filePermissions); err != nil {
		t.Fatalf("Failed to write quirks file: %v", err)
	}

	// Load quirks store
	store, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create quirks store: %v", err)
	}
	defer store.Close()

	// Verify quirks were loaded
	if count := store.Count(); count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}

	// Verify ollama quirk
	ollamaQuirks, err := store.GetProviderQuirks("Ollama")
	if err != nil {
		t.Fatalf("Failed to get ollama quirks: %v", err)
	}

	if len(ollamaQuirks) != 1 {
		t.Fatalf("Expected 1 ollama quirk, got %d", len(ollamaQuirks))
	}

	if ollamaQuirks[0].Issue != "Connection timeout on first request after idle" {
		t.Errorf("Expected issue %q, got %q", "Connection timeout on first request after idle", ollamaQuirks[0].Issue)
	}

	// Verify gemini quirk
	geminiQuirks, err := store.GetProviderQuirks("Gemini CLI")
	if err != nil {
		t.Fatalf("Failed to get gemini quirks: %v", err)
	}

	if len(geminiQuirks) != 1 {
		t.Fatalf("Expected 1 gemini quirk, got %d", len(geminiQuirks))
	}

	if geminiQuirks[0].Severity != "high" {
		t.Errorf("Expected severity 'high', got %q", geminiQuirks[0].Severity)
	}
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	quirksFile := filepath.Join(tmpDir, "quirks.md")

	// Initialize directory structure
	ds := NewDirectoryStructure(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize directory structure: %v", err)
	}

	// Create first store and add quirk
	store1, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create first quirks store: %v", err)
	}

	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Test issue",
		Workaround: "Test workaround",
		Frequency:  "always",
		Severity:   "high",
	}

	if err := store1.AddQuirk(quirk); err != nil {
		t.Fatalf("Failed to add quirk: %v", err)
	}

	store1.Close()

	// Create second store and verify quirk persisted
	store2, err := NewQuirksStore(quirksFile)
	if err != nil {
		t.Fatalf("Failed to create second quirks store: %v", err)
	}
	defer store2.Close()

	quirks, err := store2.GetProviderQuirks("ollama")
	if err != nil {
		t.Fatalf("Failed to get provider quirks: %v", err)
	}

	if len(quirks) != 1 {
		t.Fatalf("Expected 1 quirk, got %d", len(quirks))
	}

	if quirks[0].Issue != "Test issue" {
		t.Errorf("Expected issue %q, got %q", "Test issue", quirks[0].Issue)
	}
}
