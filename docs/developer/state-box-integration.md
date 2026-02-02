# State Box Integration Guide for Developers

This guide explains how to integrate the State Box pattern into your services and components.

## Overview

The State Box provides a centralized, secure storage system for all mutable application data. It handles:

- Path resolution with environment variable support
- Atomic file operations with durability guarantees
- Automatic permission hardening
- Read-only mode enforcement
- Backup creation and recovery

## Core Components

### StateBox Package

Located in `internal/util/statebox.go`, the StateBox package provides:

```go
// Create a new StateBox instance
sb, err := util.NewStateBox()
if err != nil {
    log.Fatal(err)
}

// Get paths
discoveryDir := sb.DiscoveryDir()      // ~/.switchailocal/discovery
intelligenceDir := sb.IntelligenceDir() // ~/.switchailocal/intelligence
credentialsDir := sb.CredentialsDir()   // ~/.switchailocal/credentials

// Check read-only mode
if sb.IsReadOnly() {
    log.Println("Running in read-only mode")
}

// Ensure directory exists with 0700 permissions
if err := sb.EnsureDir(discoveryDir); err != nil {
    log.Fatal(err)
}

// Resolve paths
path := sb.ResolvePath("discovery/registry.json")
```

### SecureWriter

Located in `internal/util/securewrite.go`, provides atomic file operations:

```go
import "github.com/traylinx/switchAILocal/internal/util"

// Write data atomically
data := []byte("important data")
opts := &util.SecureWriteOptions{
    CreateBackup: true,
    Permissions:  0600,
}

if err := util.SecureWrite(sb, "/path/to/file", data, opts); err != nil {
    if err == util.ErrReadOnlyMode {
        log.Println("Cannot write in read-only mode")
    }
    log.Fatal(err)
}

// Write JSON atomically
type Config struct {
    Name string `json:"name"`
}

config := &Config{Name: "test"}
if err := util.SecureWriteJSON(sb, "/path/to/config.json", config, opts); err != nil {
    log.Fatal(err)
}
```

### PermissionHardener

Located in `internal/util/permissions.go`, handles security hardening:

```go
import "github.com/traylinx/switchAILocal/internal/util"

// Harden permissions in State Box
if err := util.HardenPermissions(sb); err != nil {
    log.Printf("Warning: permission hardening failed: %v", err)
    // Continue operation - hardening is best-effort
}

// Audit permissions without modifying
results, err := util.AuditPermissions(sb)
if err != nil {
    log.Fatal(err)
}

for _, result := range results {
    if result.WasCorrected {
        log.Printf("Corrected permissions for %s", result.Path)
    }
}
```

## Service Integration

### Pattern 1: Service with StateBox

```go
package myservice

import (
    "github.com/traylinx/switchAILocal/internal/util"
)

type MyService struct {
    stateBox *util.StateBox
    // ... other fields
}

// SetStateBox configures the State Box for this service
func (s *MyService) SetStateBox(sb *util.StateBox) {
    s.stateBox = sb
}

// Initialize sets up the service
func (s *MyService) Initialize(ctx context.Context) error {
    if s.stateBox == nil {
        return errors.New("StateBox not configured")
    }

    // Check read-only mode
    if s.stateBox.IsReadOnly() {
        log.Println("Service running in read-only mode")
    }

    // Ensure directories exist
    if err := s.stateBox.EnsureDir(s.stateBox.DiscoveryDir()); err != nil {
        return err
    }

    return nil
}

// WriteData writes data atomically
func (s *MyService) WriteData(data interface{}) error {
    if s.stateBox == nil {
        return errors.New("StateBox not configured")
    }

    path := s.stateBox.ResolvePath("discovery/data.json")
    opts := &util.SecureWriteOptions{
        CreateBackup: true,
        Permissions:  0600,
    }

    return util.SecureWriteJSON(s.stateBox, path, data, opts)
}
```

### Pattern 2: Credential Management

```go
// Read a credential
var cred map[string]string
if err := sb.ReadCredential("gemini", &cred); err != nil {
    if os.IsNotExist(err) {
        log.Println("Credential not found")
    }
    return err
}

// Write a credential
cred := map[string]string{
    "api_key": "sk-...",
    "endpoint": "https://...",
}

if err := sb.WriteCredential("gemini", cred); err != nil {
    if err == util.ErrReadOnlyMode {
        log.Println("Cannot write credentials in read-only mode")
    }
    return err
}

// Get credential path
credPath := sb.CredentialPath("gemini")
```

### Pattern 3: Backward Compatibility

```go
// Support legacy auth-dir configuration
func NewService(authDir string) (*MyService, error) {
    sb, err := util.NewStateBox()
    if err != nil {
        return nil, err
    }

    // Set legacy auth-dir if provided
    if authDir != "" {
        if err := sb.SetLegacyAuthDir(authDir); err != nil {
            log.Printf("Warning: failed to set legacy auth-dir: %v", err)
        }
    }

    return &MyService{stateBox: sb}, nil
}
```

## Error Handling

### Read-Only Mode Errors

```go
if err := util.SecureWrite(sb, path, data, opts); err != nil {
    if err == util.ErrReadOnlyMode {
        // Handle read-only mode gracefully
        log.Println("Write operation blocked in read-only mode")
        return nil // Or return a specific error
    }
    return err
}
```

### Permission Errors

```go
if err := util.HardenPermissions(sb); err != nil {
    // Log warning but continue - hardening is best-effort
    log.Printf("Warning: permission hardening failed: %v", err)
}
```

### File Operation Errors

```go
if err := util.SecureWrite(sb, path, data, opts); err != nil {
    switch err {
    case util.ErrReadOnlyMode:
        return fmt.Errorf("read-only mode: %w", err)
    default:
        return fmt.Errorf("failed to write file: %w", err)
    }
}
```

## Testing

### Unit Tests

```go
package myservice

import (
    "os"
    "testing"
    "github.com/traylinx/switchAILocal/internal/util"
)

func TestServiceWithStateBox(t *testing.T) {
    // Create temporary State Box
    tmpDir := t.TempDir()
    t.Setenv("SWITCHAI_STATE_DIR", tmpDir)

    sb, err := util.NewStateBox()
    if err != nil {
        t.Fatal(err)
    }

    // Create service
    svc := &MyService{}
    svc.SetStateBox(sb)

    // Test initialization
    if err := svc.Initialize(context.Background()); err != nil {
        t.Fatal(err)
    }

    // Test write
    data := map[string]string{"key": "value"}
    if err := svc.WriteData(data); err != nil {
        t.Fatal(err)
    }
}

func TestReadOnlyMode(t *testing.T) {
    tmpDir := t.TempDir()
    t.Setenv("SWITCHAI_STATE_DIR", tmpDir)
    t.Setenv("SWITCHAI_READONLY", "1")

    sb, err := util.NewStateBox()
    if err != nil {
        t.Fatal(err)
    }

    if !sb.IsReadOnly() {
        t.Fatal("Expected read-only mode")
    }

    // Test write rejection
    err = util.SecureWrite(sb, tmpDir+"/test.txt", []byte("data"), nil)
    if err != util.ErrReadOnlyMode {
        t.Fatalf("Expected ErrReadOnlyMode, got %v", err)
    }
}
```

### Integration Tests

```go
func TestServiceIntegration(t *testing.T) {
    tmpDir := t.TempDir()
    t.Setenv("SWITCHAI_STATE_DIR", tmpDir)

    sb, err := util.NewStateBox()
    if err != nil {
        t.Fatal(err)
    }

    // Harden permissions
    if err := util.HardenPermissions(sb); err != nil {
        t.Fatal(err)
    }

    // Verify permissions
    info, err := os.Stat(sb.RootPath())
    if err != nil {
        t.Fatal(err)
    }

    if info.Mode().Perm() != 0700 {
        t.Fatalf("Expected 0700, got %o", info.Mode().Perm())
    }
}
```

## API Integration

### State Box Status Endpoint

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/traylinx/switchAILocal/internal/api"
    "github.com/traylinx/switchAILocal/internal/util"
)

// Register State Box routes
func setupRoutes(router *gin.Engine, sb *util.StateBox) {
    // Get the handler
    handler := api.StateBoxStatusHandler(sb)
    
    // Register endpoint
    router.GET("/api/state-box/status", handler)
}
```

### Querying State Box Status

```go
// Client-side
const response = await fetch('/api/state-box/status', {
    headers: { 'Authorization': 'Bearer sk-test-123' }
});

const status = await response.json();
console.log('Read-only mode:', status.read_only);
console.log('State Box path:', status.root_path);
```

## Best Practices

### 1. Always Check Read-Only Mode

```go
if sb.IsReadOnly() {
    log.Println("Running in read-only mode - writes disabled")
    // Handle gracefully
}
```

### 2. Use Atomic Writes for Important Data

```go
opts := &util.SecureWriteOptions{
    CreateBackup: true,
    Permissions:  0600,
}
util.SecureWriteJSON(sb, path, data, opts)
```

### 3. Ensure Directories Exist

```go
if err := sb.EnsureDir(sb.DiscoveryDir()); err != nil {
    return err
}
```

### 4. Handle Errors Gracefully

```go
if err := util.HardenPermissions(sb); err != nil {
    log.Printf("Warning: %v", err)
    // Continue operation
}
```

### 5. Test with Temporary State Box

```go
tmpDir := t.TempDir()
t.Setenv("SWITCHAI_STATE_DIR", tmpDir)
sb, _ := util.NewStateBox()
```

## Common Patterns

### Pattern: Lazy Initialization

```go
func (s *MyService) ensureStateBox() error {
    if s.stateBox == nil {
        var err error
        s.stateBox, err = util.NewStateBox()
        if err != nil {
            return err
        }
    }
    return nil
}
```

### Pattern: Graceful Degradation

```go
func (s *MyService) WriteData(data interface{}) error {
    if s.stateBox.IsReadOnly() {
        log.Println("Skipping write in read-only mode")
        return nil // Don't fail, just skip
    }
    return util.SecureWriteJSON(s.stateBox, path, data, opts)
}
```

### Pattern: Backup Recovery

```go
func (s *MyService) LoadData(path string) (interface{}, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        // Try backup
        backupPath := path + ".bak"
        data, err = os.ReadFile(backupPath)
        if err != nil {
            return nil, fmt.Errorf("failed to load data or backup: %w", err)
        }
        log.Printf("Recovered from backup: %s", backupPath)
    }
    return data, nil
}
```

## Troubleshooting

### State Box Not Initialized

**Error:** `StateBox not configured`

**Solution:** Ensure `SetStateBox()` is called before using the service:

```go
svc := NewService()
sb, _ := util.NewStateBox()
svc.SetStateBox(sb)
```

### Permission Denied

**Error:** `permission denied` when accessing State Box

**Solution:** Run permission hardening:

```go
if err := util.HardenPermissions(sb); err != nil {
    log.Printf("Warning: %v", err)
}
```

### Read-Only Mode Blocking Writes

**Error:** `Read-only environment: write operations disabled`

**Solution:** Check if read-only mode is intentional:

```go
if sb.IsReadOnly() {
    log.Println("Read-only mode is enabled")
    // Handle gracefully or disable read-only mode
}
```

## See Also

- [State Box User Guide](../user/state-box.md)
- [Implementation Report](../../.kiro/specs/state-box/IMPLEMENTATION_REPORT.md)
- [Design Document](../../.kiro/specs/state-box/design.md)
