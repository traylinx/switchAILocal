# Security Fail-Safe

The security fail-safe provides safety controls for autonomous Superbrain actions. It ensures that dangerous operations are never performed without human approval, even in autopilot mode.

## Overview

The `FailSafe` component examines diagnoses and healing actions to detect security-sensitive operations. When a forbidden operation is detected, it blocks the autonomous remediation and returns a safe failure response explaining what was attempted and why it was blocked.

## Usage

### Creating a FailSafe

```go
import "github.com/traylinx/switchAILocal/internal/superbrain/security"

// Create fail-safe with forbidden operations from config
forbiddenOps := []string{"file_delete", "system_command", "sudo"}
failSafe := security.NewFailSafe(forbiddenOps)
```

### Checking Diagnoses

```go
// After diagnosis, check if it involves forbidden operations
diagnosis, err := doctor.Diagnose(ctx, snapshot)
if err != nil {
    return err
}

isForbidden, reason := failSafe.CheckDiagnosis(diagnosis)
if isForbidden {
    // Create safe failure response
    response := failSafe.CreateSafeFailureResponse(diagnosis, reason)
    
    // Log the security block
    if auditLogger != nil {
        auditLogger.LogSecurityBlock(requestID, diagnosis, reason)
    }
    
    // Return safe failure instead of attempting remediation
    return response, nil
}

// Safe to proceed with healing
```

### Checking Healing Actions

```go
// Before executing a healing action, check if it's forbidden
action := &types.HealingAction{
    ActionType:  "restart_with_flags",
    Description: "Restarting with --skip-permissions",
    Details: map[string]interface{}{
        "flags": []string{"--skip-permissions"},
    },
}

isForbidden, reason := failSafe.CheckHealingAction(action)
if isForbidden {
    // Block the action
    return fmt.Errorf("action blocked by security fail-safe: %s", reason)
}

// Safe to execute the action
```

## Integration with Executor

The security fail-safe should be integrated into the executor's healing flow:

```go
func (se *SuperbrainExecutor) executeWithHealing(ctx context.Context, ...) {
    // ... execute request ...
    
    // Diagnose failure
    diagnosis, err := se.doctor.Diagnose(ctx, snapshot)
    if err != nil {
        return err
    }
    
    // Check security fail-safe
    if se.failSafe != nil {
        isForbidden, reason := se.failSafe.CheckDiagnosis(diagnosis)
        if isForbidden {
            response := se.failSafe.CreateSafeFailureResponse(diagnosis, reason)
            
            // Log security block
            if se.auditLogger != nil {
                se.auditLogger.LogSecurityBlock(
                    aggregator.GetMetadata().RequestID,
                    string(diagnosis.FailureType),
                    string(diagnosis.Remediation),
                    reason,
                )
            }
            
            // Return safe failure
            return response, nil
        }
    }
    
    // Safe to proceed with healing
    switch diagnosis.Remediation {
    case RemediationStdinInject:
        // ... attempt stdin injection ...
    case RemediationRestartFlags:
        // ... attempt restart with flags ...
    // ...
    }
}
```

## Configuration

The forbidden operations list is configured in `config.yaml`:

```yaml
superbrain:
  enabled: true
  mode: "autopilot"
  
  security:
    audit_log_enabled: true
    audit_log_path: "./logs/superbrain_audit.log"
    forbidden_operations:
      - "file_delete"
      - "system_command"
      - "sudo"
      - "rm -rf"
      - "chmod"
```

## Safe Failure Response Format

When a forbidden operation is detected, the fail-safe returns a structured response:

```json
{
  "error": {
    "message": "Autonomous remediation blocked by security fail-safe",
    "type": "security_violation",
    "code": "forbidden_operation"
  },
  "superbrain": {
    "security_blocked": true,
    "block_reason": "remediation type 'file_delete' is forbidden",
    "attempted_actions": [
      "Diagnosis: process_crash",
      "Proposed remediation: file_delete"
    ],
    "diagnosis_summary": "Process attempted to delete critical file",
    "suggestions": [
      "This operation requires human approval",
      "Review the diagnosis and manually apply the remediation if appropriate",
      "Consider updating the forbidden_operations configuration if this operation should be allowed"
    ]
  }
}
```

## Testing

The security fail-safe includes comprehensive unit tests covering:

- Detection of forbidden operations in remediation types
- Detection of forbidden operations in root causes
- Detection of forbidden operations in remediation arguments
- Detection of forbidden operations in healing action types
- Detection of forbidden operations in action descriptions
- Detection of forbidden operations in action details
- Case-insensitive matching
- Safe failure response generation
- Empty forbidden operations list handling

Run tests with:

```bash
go test ./internal/superbrain/security/...
```

## Security Properties

The security fail-safe validates **Property 40: Security Fail-Safe** from the design document:

> For any detected security-sensitive operation (file deletion, system commands, sudo), the Superbrain shall abort autonomous remediation and return a safe failure response.

This ensures that:
1. Forbidden operations are never executed autonomously
2. All security blocks are logged for audit
3. Users receive clear explanations of what was blocked and why
4. Suggestions are provided for manual remediation
