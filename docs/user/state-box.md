# State Box: Secure State Management

The **State Box** is switchAILocal's centralized, secure storage system for all mutable application data. It ensures that your AI provider configurations, feedback data, and credentials are stored safely outside the repository, with automatic security hardening and atomic write protection.

## Overview

### What is the State Box?

The State Box is a mandatory architectural boundary that separates code from data:

- **Location:** `~/.switchailocal/` (customizable via `SWITCHAI_STATE_DIR`)
- **Purpose:** Centralized storage for all persistent state
- **Security:** Automatic permission hardening and atomic writes
- **Immutable-Friendly:** Full support for read-only environments (NixOS, containers)

### Directory Structure

```
~/.switchailocal/
├── discovery/
│   ├── registry.json          # Discovered AI models
│   └── registry.json.bak      # Automatic backup
├── intelligence/
│   ├── feedback.db            # Routing feedback database
│   └── feedback.db.bak        # Automatic backup
└── credentials/
    ├── gemini.json            # Provider credentials
    ├── claude.json
    └── openai.json
```

All directories are created with `0700` permissions (owner read/write/execute only).
All sensitive files are created with `0600` permissions (owner read/write only).

## Configuration

### Environment Variables

#### `SWITCHAI_STATE_DIR`

Override the default State Box location:

```bash
# Use custom location
export SWITCHAI_STATE_DIR=/data/switchai-state

# Use relative path (expanded from home)
export SWITCHAI_STATE_DIR=~/my-switchai-data
```

**Default:** `~/.switchailocal`

#### `SWITCHAI_READONLY`

Enable read-only mode for immutable environments:

```bash
# Enable read-only mode
export SWITCHAI_READONLY=1
```

When enabled:
- All write operations are blocked with clear error messages
- The Management Dashboard displays a "Read-Only Mode" indicator
- Save and edit buttons are disabled
- Discovery processes skip writing model registries
- Feedback collection is disabled

**Default:** Disabled (0)

### Configuration File

No additional configuration is needed. The State Box is automatically initialized on startup.

## Features

### 1. Atomic File Operations

The State Box uses the **rename-swap pattern** to ensure data durability:

1. **Buffered Write:** Data is written to a temporary file (`.tmp.{uuid}`)
2. **Sync:** `fsync()` is called to flush OS buffers to disk
3. **Atomic Rename:** The temporary file is atomically renamed to the target
4. **Backup:** The previous version is saved as `.bak` before overwriting

**Benefits:**
- Power failures during writes don't corrupt files
- Previous versions are always available for recovery
- No partial writes or corrupted state

### 2. Automatic Security Hardening

On startup, switchAILocal automatically audits and corrects file permissions:

- **Directories:** Corrected to `0700` (owner only)
- **Sensitive Files:** Corrected to `0600` (owner only)
- **Logging:** All corrections are logged as security audit events

```bash
# Example log output
security audit: corrected permissions for ~/.switchailocal from 0755 to 0700
security audit: corrected permissions for ~/.switchailocal/discovery/registry.json from 0644 to 0600
permission hardening: corrected 2 file/directory permissions
```

### 3. Read-Only Mode Support

Perfect for immutable environments like NixOS or containerized deployments:

```bash
# Start in read-only mode
SWITCHAI_READONLY=1 ./switchAILocal
```

**Behavior:**
- All write operations return clear error messages
- UI displays read-only indicator
- System continues to function for queries and routing
- No data loss or corruption

### 4. Backward Compatibility

If you have an existing `auth-dir` configuration, switchAILocal will:

1. Check the legacy `auth-dir` location first
2. Fall back to State Box credentials if not found
3. Automatically migrate credentials on first write

```yaml
# config.yaml - legacy configuration still works
auth-dir: ~/.switchai/auth
```

## API Endpoint

### GET /api/state-box/status

Query the current State Box status:

```bash
curl http://localhost:18080/api/state-box/status \
  -H "Authorization: Bearer sk-test-123"
```

**Response:**

```json
{
  "root_path": "/Users/user/.switchailocal",
  "read_only": false,
  "initialized": true,
  "discovery_registry": {
    "path": "/Users/user/.switchailocal/discovery/registry.json",
    "exists": true,
    "size": 4096,
    "mode": "-rw-------",
    "mod_time": "2026-01-31T10:30:00Z"
  },
  "feedback_database": {
    "path": "/Users/user/.switchailocal/intelligence/feedback.db",
    "exists": true,
    "size": 102400,
    "mode": "-rw-------",
    "mod_time": "2026-01-31T10:25:00Z"
  },
  "permission_status": "ok",
  "warnings": [],
  "errors": []
}
```

## Management Dashboard

The Management Dashboard displays State Box status in the settings panel:

- **Read-Only Indicator:** Shows when the system is in read-only mode
- **State Box Path:** Displays the current State Box location
- **File Status:** Shows discovery registry and feedback database status
- **Permission Status:** Indicates if permissions are correctly hardened

### Read-Only Mode Indicator

When `SWITCHAI_READONLY=1` is set:

- A "Read-Only Mode" badge appears in the top-right corner
- All "Save" and "Edit" buttons are disabled
- Hovering over the badge shows an explanation tooltip

## Troubleshooting

### State Box Directory Not Found

**Error:** `State Box root directory does not exist`

**Solution:** The directory will be created automatically on first write. If you need to pre-create it:

```bash
mkdir -p ~/.switchailocal
chmod 0700 ~/.switchailocal
```

### Permission Denied Errors

**Error:** `permission denied` when accessing State Box files

**Solution:** Run the permission hardening manually:

```bash
# Restart switchAILocal to trigger automatic hardening
./switchAILocal
```

Or manually fix permissions:

```bash
chmod 0700 ~/.switchailocal
chmod 0700 ~/.switchailocal/discovery
chmod 0700 ~/.switchailocal/intelligence
chmod 0700 ~/.switchailocal/credentials
chmod 0600 ~/.switchailocal/discovery/registry.json
chmod 0600 ~/.switchailocal/intelligence/feedback.db
```

### Read-Only Mode Errors

**Error:** `Read-only environment: write operations disabled`

**Solution:** This is expected when `SWITCHAI_READONLY=1` is set. To enable writes:

```bash
unset SWITCHAI_READONLY
./switchAILocal
```

### Corrupted Files

**Error:** `failed to parse registry.json`

**Solution:** The system will automatically restore from the `.bak` file:

```bash
# Manual recovery if needed
cp ~/.switchailocal/discovery/registry.json.bak ~/.switchailocal/discovery/registry.json
chmod 0600 ~/.switchailocal/discovery/registry.json
```

## Docker & Container Deployment

### Using State Box with Docker

Mount the State Box directory as a volume:

```bash
docker run -v ~/.switchailocal:/root/.switchailocal \
  -p 18080:18080 \
  switchailocal:latest
```

Or use a named volume:

```bash
docker volume create switchai-state
docker run -v switchai-state:/root/.switchailocal \
  -p 18080:18080 \
  switchailocal:latest
```

### Custom State Box Location in Docker

```bash
docker run -e SWITCHAI_STATE_DIR=/data/switchai \
  -v /host/data:/data \
  -p 18080:18080 \
  switchailocal:latest
```

### Read-Only Mode in Docker

```bash
docker run -e SWITCHAI_READONLY=1 \
  -v ~/.switchailocal:/root/.switchailocal:ro \
  -p 18080:18080 \
  switchailocal:latest
```

## NixOS Integration

### Declarative Configuration

```nix
{
  environment.variables = {
    SWITCHAI_STATE_DIR = "/var/lib/switchailocal";
    SWITCHAI_READONLY = "1";  # Optional: enable read-only mode
  };

  systemd.tmpfiles.rules = [
    "d /var/lib/switchailocal 0700 switchai switchai -"
  ];
}
```

### Stateful Directory

```nix
{
  systemd.services.switchailocal = {
    serviceConfig = {
      StateDirectory = "switchailocal";
      StateDirectoryMode = "0700";
    };
  };
}
```

## Security Best Practices

### 1. Protect Credentials

Never commit credentials to version control:

```bash
# Add to .gitignore
echo "~/.switchailocal/" >> .gitignore
```

### 2. Use Environment Variables

For sensitive data, use environment variables instead of config files:

```bash
export GEMINI_API_KEY="your-key-here"
export CLAUDE_API_KEY="your-key-here"
```

### 3. Regular Backups

Back up your State Box regularly:

```bash
# Daily backup
tar -czf ~/backups/switchai-$(date +%Y%m%d).tar.gz ~/.switchailocal/
```

### 4. Monitor Permissions

Verify permissions are correct:

```bash
ls -la ~/.switchailocal/
ls -la ~/.switchailocal/discovery/
ls -la ~/.switchailocal/intelligence/
ls -la ~/.switchailocal/credentials/
```

Expected output:
```
drwx------  user  group  ~/.switchailocal/
-rw-------  user  group  ~/.switchailocal/discovery/registry.json
-rw-------  user  group  ~/.switchailocal/intelligence/feedback.db
```

### 5. Read-Only Deployments

For production deployments, use read-only mode:

```bash
SWITCHAI_READONLY=1 ./switchAILocal
```

## Advanced Topics

### Custom State Box Location

For multi-user systems or special deployments:

```bash
# User-specific State Box
export SWITCHAI_STATE_DIR=~/.config/switchailocal

# System-wide State Box
export SWITCHAI_STATE_DIR=/opt/switchailocal/state
```

### Migrating Existing Data

If you have existing data in a different location:

```bash
# Copy existing data to State Box
mkdir -p ~/.switchailocal
cp -r /old/location/* ~/.switchailocal/

# Fix permissions
chmod 0700 ~/.switchailocal
chmod 0700 ~/.switchailocal/discovery
chmod 0700 ~/.switchailocal/intelligence
chmod 0700 ~/.switchailocal/credentials
chmod 0600 ~/.switchailocal/discovery/registry.json
chmod 0600 ~/.switchailocal/intelligence/feedback.db
```

### Monitoring State Box Operations

Enable debug logging to see State Box operations:

```bash
# Set log level to debug
export LOG_LEVEL=debug
./switchAILocal
```

Look for log messages like:
```
StateBox initialized at /Users/user/.switchailocal (read-only: false)
security audit: corrected permissions for ...
permission hardening: corrected X file/directory permissions
```

## FAQ

**Q: Can I use a network drive for State Box?**  
A: Not recommended. Network drives may not support atomic operations reliably. Use local storage for best results.

**Q: What happens if State Box is on a read-only filesystem?**  
A: Set `SWITCHAI_READONLY=1` to enable read-only mode. The system will continue to function for queries.

**Q: Can I share State Box between multiple machines?**  
A: Not recommended. Each machine should have its own State Box to avoid conflicts.

**Q: How do I recover from a corrupted State Box?**  
A: The system automatically restores from `.bak` files. If needed, delete the corrupted file and restart.

**Q: Is State Box compatible with cloud storage (Dropbox, iCloud)?**  
A: Not recommended. Cloud sync may interfere with atomic writes. Use local storage.

**Q: Can I disable State Box?**  
A: No, State Box is mandatory. However, you can use read-only mode to prevent writes.

## See Also

- [Configuration Guide](configuration.md)
- [Management Dashboard](management-dashboard.md)
- [API Reference](api-reference.md)
