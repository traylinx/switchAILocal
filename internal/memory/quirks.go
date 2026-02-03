package memory

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// QuirksStore manages persistent storage of provider quirks in Markdown format.
// It provides thread-safe operations for adding and retrieving quirks.
type QuirksStore struct {
	filePath string
	mu       sync.RWMutex
	quirks   map[string][]*Quirk // provider -> quirks
}

// NewQuirksStore creates a new provider quirks store.
// It loads existing quirks from the file on initialization.
func NewQuirksStore(filePath string) (*QuirksStore, error) {
	store := &QuirksStore{
		filePath: filePath,
		quirks:   make(map[string][]*Quirk),
	}

	// Load existing quirks from file
	if err := store.load(); err != nil {
		return nil, fmt.Errorf("failed to load quirks: %w", err)
	}

	return store, nil
}

// AddQuirk records a provider quirk to persistent storage.
// It appends the quirk to the markdown file and updates the in-memory cache.
// Duplicate quirks (same provider and issue) are not added.
func (qs *QuirksStore) AddQuirk(quirk *Quirk) error {
	if quirk == nil {
		return fmt.Errorf("quirk cannot be nil")
	}

	// Comprehensive validation
	if err := ValidateQuirk(quirk); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check disk space before writing (estimate 2KB per quirk)
	if err := CheckDiskSpace(qs.filePath, 2*1024); err != nil {
		return fmt.Errorf("cannot write quirk: %w", err)
	}

	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Check for duplicate quirks (same provider and issue)
	if existingQuirks, ok := qs.quirks[quirk.Provider]; ok {
		for _, existing := range existingQuirks {
			if existing.Issue == quirk.Issue {
				// Update frequency and severity if changed
				if existing.Frequency != quirk.Frequency || existing.Severity != quirk.Severity {
					existing.Frequency = quirk.Frequency
					existing.Severity = quirk.Severity
					// Rewrite entire file to update the quirk
					return qs.rewriteFile()
				}
				// Duplicate quirk, no action needed
				return nil
			}
		}
	}

	// Set discovered time if not set
	if quirk.Discovered.IsZero() {
		quirk.Discovered = time.Now()
	}

	// Add to in-memory cache
	qs.quirks[quirk.Provider] = append(qs.quirks[quirk.Provider], quirk)

	// Append to file
	return qs.appendToFile(quirk)
}

// GetProviderQuirks retrieves all known quirks for a specific provider.
// Returns an empty slice if no quirks are found for the provider.
func (qs *QuirksStore) GetProviderQuirks(provider string) ([]*Quirk, error) {
	if provider == "" {
		return nil, fmt.Errorf("provider cannot be empty")
	}

	qs.mu.RLock()
	defer qs.mu.RUnlock()

	quirks, ok := qs.quirks[provider]
	if !ok {
		return []*Quirk{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]*Quirk, len(quirks))
	copy(result, quirks)

	return result, nil
}

// GetAllQuirks retrieves all quirks for all providers.
func (qs *QuirksStore) GetAllQuirks() (map[string][]*Quirk, error) {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	// Return a deep copy to prevent external modification
	result := make(map[string][]*Quirk)
	for provider, quirks := range qs.quirks {
		providerQuirks := make([]*Quirk, len(quirks))
		copy(providerQuirks, quirks)
		result[provider] = providerQuirks
	}

	return result, nil
}

// GetQuirksByFrequency retrieves quirks that match a specific frequency pattern.
// This is useful for finding quirks that occur frequently.
func (qs *QuirksStore) GetQuirksByFrequency(frequencyPattern string) ([]*Quirk, error) {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	var result []*Quirk
	for _, quirks := range qs.quirks {
		for _, quirk := range quirks {
			if strings.Contains(strings.ToLower(quirk.Frequency), strings.ToLower(frequencyPattern)) {
				result = append(result, quirk)
			}
		}
	}

	return result, nil
}

// GetQuirksBySeverity retrieves all quirks with a specific severity level.
func (qs *QuirksStore) GetQuirksBySeverity(severity string) ([]*Quirk, error) {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	var result []*Quirk
	for _, quirks := range qs.quirks {
		for _, quirk := range quirks {
			if quirk.Severity == severity {
				result = append(result, quirk)
			}
		}
	}

	return result, nil
}

// ApplyWorkaround returns the workaround for a specific provider and issue.
// Returns an empty string if no matching quirk is found.
func (qs *QuirksStore) ApplyWorkaround(provider, issue string) (string, error) {
	if provider == "" {
		return "", fmt.Errorf("provider cannot be empty")
	}
	if issue == "" {
		return "", fmt.Errorf("issue cannot be empty")
	}

	qs.mu.RLock()
	defer qs.mu.RUnlock()

	quirks, ok := qs.quirks[provider]
	if !ok {
		return "", nil
	}

	// Find matching quirk by issue (case-insensitive partial match)
	for _, quirk := range quirks {
		if strings.Contains(strings.ToLower(issue), strings.ToLower(quirk.Issue)) ||
			strings.Contains(strings.ToLower(quirk.Issue), strings.ToLower(issue)) {
			return quirk.Workaround, nil
		}
	}

	return "", nil
}

// load reads the quirks file and populates the in-memory cache.
func (qs *QuirksStore) load() error {
	file, err := os.Open(qs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's okay
			return nil
		}
		return fmt.Errorf("failed to open quirks file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentProvider string
	var currentQuirk *Quirk

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and template content
		if line == "" || strings.HasPrefix(line, "This file tracks") || strings.HasPrefix(line, "Quirks are") {
			continue
		}

		// Parse provider header (### Provider Name)
		if strings.HasPrefix(line, "### ") {
			// Save previous quirk if exists
			if currentQuirk != nil && currentProvider != "" {
				qs.quirks[currentProvider] = append(qs.quirks[currentProvider], currentQuirk)
			}

			currentProvider = strings.TrimPrefix(line, "### ")
			currentQuirk = &Quirk{Provider: currentProvider}
			continue
		}

		// Parse quirk fields
		if currentQuirk != nil {
			if strings.HasPrefix(line, "- **Issue**:") {
				currentQuirk.Issue = strings.TrimSpace(strings.TrimPrefix(line, "- **Issue**:"))
			} else if strings.HasPrefix(line, "- **Workaround**:") {
				currentQuirk.Workaround = strings.TrimSpace(strings.TrimPrefix(line, "- **Workaround**:"))
			} else if strings.HasPrefix(line, "- **Discovered**:") {
				dateStr := strings.TrimSpace(strings.TrimPrefix(line, "- **Discovered**:"))
				if t, err := time.Parse("2006-01-02", dateStr); err == nil {
					currentQuirk.Discovered = t
				}
			} else if strings.HasPrefix(line, "- **Frequency**:") {
				currentQuirk.Frequency = strings.TrimSpace(strings.TrimPrefix(line, "- **Frequency**:"))
			} else if strings.HasPrefix(line, "- **Severity**:") {
				currentQuirk.Severity = strings.TrimSpace(strings.TrimPrefix(line, "- **Severity**:"))
			}
		}

		// Check if we've completed a quirk entry (next provider or separator)
		if line == "---" && currentQuirk != nil && currentProvider != "" {
			// Validate quirk has required fields before adding
			if currentQuirk.Issue != "" && currentQuirk.Workaround != "" {
				qs.quirks[currentProvider] = append(qs.quirks[currentProvider], currentQuirk)
			}
			currentQuirk = nil
			currentProvider = ""
		}
	}

	// Save last quirk if exists
	if currentQuirk != nil && currentProvider != "" && currentQuirk.Issue != "" && currentQuirk.Workaround != "" {
		qs.quirks[currentProvider] = append(qs.quirks[currentProvider], currentQuirk)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading quirks file: %w", err)
	}

	return nil
}

// appendToFile appends a quirk to the markdown file.
func (qs *QuirksStore) appendToFile(quirk *Quirk) error {
	file, err := os.OpenFile(qs.filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, filePermissions)
	if err != nil {
		return fmt.Errorf("failed to open quirks file for appending: %w", err)
	}
	defer file.Close()

	// Format quirk as markdown
	markdown := fmt.Sprintf("\n### %s\n", quirk.Provider)
	markdown += fmt.Sprintf("- **Issue**: %s\n", quirk.Issue)
	markdown += fmt.Sprintf("- **Workaround**: %s\n", quirk.Workaround)
	markdown += fmt.Sprintf("- **Discovered**: %s\n", quirk.Discovered.Format("2006-01-02"))
	markdown += fmt.Sprintf("- **Frequency**: %s\n", quirk.Frequency)
	markdown += fmt.Sprintf("- **Severity**: %s\n", quirk.Severity)
	markdown += "\n---\n"

	if _, err := file.WriteString(markdown); err != nil {
		return fmt.Errorf("failed to write quirk to file: %w", err)
	}

	return nil
}

// rewriteFile rewrites the entire quirks file with current in-memory data.
// This is used when updating existing quirks.
func (qs *QuirksStore) rewriteFile() error {
	// Create temporary file
	tmpFile := qs.filePath + ".tmp"
	file, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePermissions)
	if err != nil {
		return fmt.Errorf("failed to create temporary quirks file: %w", err)
	}

	// Write header
	header := `# Provider Quirks & Workarounds

This file tracks known issues with providers and their workarounds.
Quirks are automatically discovered and documented during operation.

## Format

Each quirk entry should follow this format:

### Provider Name
- **Issue**: Description of the issue
- **Workaround**: How to work around the issue
- **Discovered**: Date when the issue was first discovered
- **Frequency**: How often the issue occurs
- **Severity**: low, medium, high, or critical

---

`
	if _, err := file.WriteString(header); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("failed to write header to quirks file: %w", err)
	}

	// Write all quirks
	for provider, quirks := range qs.quirks {
		for _, quirk := range quirks {
			markdown := fmt.Sprintf("\n### %s\n", provider)
			markdown += fmt.Sprintf("- **Issue**: %s\n", quirk.Issue)
			markdown += fmt.Sprintf("- **Workaround**: %s\n", quirk.Workaround)
			markdown += fmt.Sprintf("- **Discovered**: %s\n", quirk.Discovered.Format("2006-01-02"))
			markdown += fmt.Sprintf("- **Frequency**: %s\n", quirk.Frequency)
			markdown += fmt.Sprintf("- **Severity**: %s\n", quirk.Severity)
			markdown += "\n---\n"

			if _, err := file.WriteString(markdown); err != nil {
				file.Close()
				os.Remove(tmpFile)
				return fmt.Errorf("failed to write quirk to file: %w", err)
			}
		}
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to close temporary quirks file: %w", err)
	}

	// Replace original file with temporary file
	if err := os.Rename(tmpFile, qs.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to replace quirks file: %w", err)
	}

	return nil
}

// Count returns the total number of quirks across all providers.
func (qs *QuirksStore) Count() int {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	count := 0
	for _, quirks := range qs.quirks {
		count += len(quirks)
	}
	return count
}

// CountByProvider returns the number of quirks for a specific provider.
func (qs *QuirksStore) CountByProvider(provider string) int {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if quirks, ok := qs.quirks[provider]; ok {
		return len(quirks)
	}
	return 0
}

// Close gracefully shuts down the quirks store.
// Currently a no-op but provided for consistency with other stores.
func (qs *QuirksStore) Close() error {
	return nil
}
