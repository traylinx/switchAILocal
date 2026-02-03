# Memory Package

The memory package provides persistent storage for routing decisions, provider quirks, and user preferences. This enables switchAILocal to learn from past decisions and improve routing over time.

## Overview

The memory system is a core component of the Clawd Patterns Integration that transforms switchAILocal from a stateless proxy into an intelligent, learning gateway. It stores:

- **Routing History**: All routing decisions with their outcomes
- **Provider Quirks**: Known issues and workarounds for providers
- **User Preferences**: Learned preferences per API key
- **Daily Logs**: Time-series data for analytics
- **Analytics**: Aggregated metrics and performance data

## Directory Structure

The memory system creates the following directory structure:

```
.switchailocal/memory/
├── routing-history.jsonl       # All routing decisions with outcomes
├── provider-quirks.md           # Known issues and workarounds
├── user-preferences/            # Per-API-key preferences
│   ├── sha256-abc123.json
│   └── sha256-def456.json
├── daily/                       # Daily operation logs
│   ├── 2026-02-01.jsonl
│   └── 2026-02-02.jsonl
└── analytics/                   # Aggregated metrics
    ├── provider-stats.json
    └── model-performance.json
```

## Usage

### Initializing the Directory Structure

```go
import "github.com/traylinx/switchAILocal/internal/memory"

// Create directory structure manager
ds := memory.NewDirectoryStructure(".switchailocal/memory")

// Initialize the directory structure
if err := ds.Initialize(); err != nil {
    log.Fatalf("Failed to initialize memory structure: %v", err)
}

// Validate the structure
if err := ds.Validate(); err != nil {
    log.Fatalf("Validation failed: %v", err)
}
```

### Getting Paths

```go
// Get path to routing history file
historyPath := ds.GetRoutingHistoryPath()

// Get path to provider quirks file
quirksPath := ds.GetProviderQuirksPath()

// Get path to user preferences directory
prefsDir := ds.GetUserPreferencesDir()

// Get path to specific user's preferences
userPrefsPath := ds.GetUserPreferencePath("sha256:abc123")

// Get path to daily log file
dailyLogPath := ds.GetDailyLogPath("2026-02-02")
```

### Configuration

```go
// Get default configuration
config := memory.DefaultMemoryConfig()

// Customize configuration
config.Enabled = true
config.BaseDir = "/custom/path/memory"
config.RetentionDays = 30
config.MaxLogSizeMB = 50
config.Compression = false
```

## Data Types

### RoutingDecision

Represents a complete routing decision with its outcome:

```go
decision := &memory.RoutingDecision{
    Timestamp:  time.Now(),
    APIKeyHash: "sha256:abc123",
    Request: memory.RequestInfo{
        Model:         "auto",
        Intent:        "coding",
        ContentHash:   "sha256:def456",
        ContentLength: 1234,
    },
    Routing: memory.RoutingInfo{
        Tier:          "semantic",
        SelectedModel: "claudecli:claude-sonnet-4",
        Confidence:    0.92,
        LatencyMs:     15,
    },
    Outcome: memory.OutcomeInfo{
        Success:        true,
        ResponseTimeMs: 2340,
        QualityScore:   0.88,
    },
}
```

### UserPreferences

Represents learned preferences for a specific API key:

```go
prefs := &memory.UserPreferences{
    APIKeyHash:  "sha256:abc123",
    LastUpdated: time.Now(),
    ModelPreferences: map[string]string{
        "coding":    "claudecli:claude-sonnet-4",
        "reasoning": "geminicli:gemini-2.5-pro",
    },
    ProviderBias: map[string]float64{
        "ollama":    0.5,
        "claudecli": 0.3,
    },
    CustomRules: []memory.PreferenceRule{
        {
            Condition: "intent == 'coding'",
            Model:     "claudecli:claude-sonnet-4",
            Priority:  100,
        },
    },
}
```

### Quirk

Represents a known provider issue and its workaround:

```go
quirk := &memory.Quirk{
    Provider:   "ollama",
    Issue:      "Connection timeout on first request after idle",
    Workaround: "Send warmup request on startup",
    Discovered: time.Now(),
    Frequency:  "3/10 startups",
    Severity:   "medium",
}
```

## Security

The memory system implements several security measures:

- **API Key Hashing**: API keys are hashed using SHA-256 and never stored in plaintext
- **File Permissions**: All files are created with restrictive permissions (0600 for files, 0700 for directories)
- **PII Sanitization**: Personal information is sanitized before storage
- **Encryption**: Optional encryption at rest for sensitive data

## Performance

The memory system is designed for minimal performance impact:

- **Async Writes**: All writes are asynchronous and non-blocking
- **Append-Only**: Routing history uses JSONL format for efficient appends
- **Caching**: Frequently accessed data is cached in memory
- **Batch Processing**: Analytics are computed in batches

Target performance metrics:
- Memory append: < 5ms
- Memory read: < 10ms
- Memory query: < 50ms

## Testing

Run the tests:

```bash
go test -v ./internal/memory/
```

Run with coverage:

```bash
go test -v -cover ./internal/memory/
```

## Requirements Validation

This implementation validates the following requirements:

- **FR-1.1**: Persistent Storage - Creates `.switchailocal/memory/` directory structure
- **FR-1.2**: Memory Types - Defines all core types (RoutingDecision, UserPreferences, Quirk, etc.)

## Next Steps

After implementing the directory structure and types, the next tasks are:

1. Implement routing history store (JSONL append-only storage)
2. Implement provider quirks tracker (Markdown format)
3. Implement user preferences system (JSON per API key)
4. Create unified memory manager
5. Integrate with Cortex Router

## References

- [Design Document](../../../.kiro/specs/clawd-patterns-integration/design.md)
- [Requirements](../../../.kiro/specs/clawd-patterns-integration/requirements.md)
- [Tasks](../../../.kiro/specs/clawd-patterns-integration/tasks.md)
