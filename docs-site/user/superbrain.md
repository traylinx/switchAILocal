# Superbrain Intelligence

**Superbrain** transforms switchAILocal from a passive API gateway into an intelligent, self-healing AI orchestrator. Instead of treating errors as terminal states, Superbrain actively monitors executions, diagnoses failures using AI, and takes autonomous remediation actions to ensure your requests succeed.

## Overview

The Superbrain architecture implements an "Observer-Critic" duality where every execution is monitored by a lightweight Supervisor that can:

- **Detect failures in real-time** (within seconds, not minutes)
- **Diagnose issues using AI** (permission prompts, auth errors, context limits)
- **Take autonomous healing actions** (stdin injection, process restart, intelligent failover)
- **Report all actions transparently** (healing metadata in responses)

### Key Benefits

| Feature | Description |
|---------|-------------|
| 🔍 **Real-Time Monitoring** | Detects hung processes and silent failures before hard timeouts |
| 🧠 **AI-Powered Diagnosis** | Uses lightweight models to analyze failures and prescribe fixes |
| 🔧 **Autonomous Healing** | Automatically responds to prompts, restarts with flags, or routes to alternatives |
| 🎯 **Intelligent Failover** | Routes failed requests to alternative providers based on capabilities |
| 📊 **Context Optimization** | Pre-analyzes large requests and optimizes content to fit model limits |
| 📝 **Transparent Actions** | Every autonomous action is logged and included in response metadata |

---

## Core Components

### 1. Overwatch Layer (Real-Time Monitoring)

Monitors all CLI executions in real-time, tracking output streams and detecting silent hangs.

**Features:**
- Heartbeat detection (tracks stdout/stderr activity)
- Configurable silence threshold (default: 30 seconds)
- Rolling log buffer (captures last N lines for diagnosis)
- Diagnostic snapshot capture on anomalies

### 2. Internal Doctor (AI-Powered Diagnosis)

Analyzes failures using a lightweight AI model to identify root causes and recommend fixes.

**Detects:**
- Permission prompts (file access, tool execution)
- Authentication errors
- Context limit exceeded
- Rate limiting
- Network errors
- Process crashes

### 3. Stdin Injector (Autonomous Input)

Automatically responds to interactive CLI prompts to prevent processes from hanging.

**Safety Modes:**
- **Conservative**: Only responds to explicitly whitelisted prompts
- **Autopilot**: Automatically approves safe operations (file reads, tool executions)

### 4. Process Recovery (Self-Healing)

Automatically restarts failed processes with corrective flags based on diagnosis.

**Example:**
- Diagnosis: "Permission prompt detected"
- Action: Restart with `--dangerously-skip-permissions` flag
- Result: Process completes successfully

### 5. Fallback Router (Intelligent Failover)

Routes failed requests to alternative providers based on capabilities and success rates.

**Selection Criteria:**
- Provider capabilities (context size, streaming support)
- Current availability
- Historical success rates
- Request requirements

### 6. Context Sculptor (Pre-Flight Optimization)

Analyzes requests before execution and optimizes content to fit model context limits.

**Features:**
- Token estimation for file/folder references
- Intelligent file prioritization (README, main entry points)
- High-density map generation for excluded content
- Alternative model recommendations

---

## Configuration

Add the `superbrain` section to your `config.yaml`:

```yaml
superbrain:
  enabled: true
  mode: "conservative"  # disabled, observe, diagnose, conservative, autopilot
  
  overwatch:
    silence_threshold_ms: 30000      # 30 seconds
    log_buffer_size: 50              # lines
    heartbeat_interval_ms: 1000      # 1 second
    max_restart_attempts: 2
  
  doctor:
    model: "gemini-flash"            # Lightweight model for diagnosis
    timeout_ms: 5000                 # Max diagnosis time
    
  stdin_injection:
    mode: "conservative"             # disabled, conservative, autopilot
    custom_patterns: []              # Additional patterns
    forbidden_patterns:              # Never auto-respond to these
      - "delete"
      - "remove"
      - "sudo"
  
  context_sculptor:
    enabled: true
    token_estimator: "simple"        # simple, tiktoken
    priority_files:                  # Always include these
      - "README.md"
      - "main.go"
      - "index.ts"
      - "package.json"
  
  fallback:
    enabled: true
    providers:                       # Fallback order preference
      - "geminicli"
      - "gemini"
      - "ollama"
    min_success_rate: 0.5            # Minimum to consider provider
  
  security:
    audit_log_enabled: true
    audit_log_path: "./logs/superbrain_audit.log"
    forbidden_operations:
      - "file_delete"
      - "system_command"
```

---

## Operational Modes

Superbrain supports multiple operational modes for gradual rollout and risk management:

### Mode: `disabled`
- **Behavior**: Superbrain is completely disabled
- **Use Case**: Emergency disable or legacy pass-through mode
- **Actions**: None

### Mode: `observe`
- **Behavior**: Monitor and log, but take no autonomous actions
- **Use Case**: Initial deployment to gather baseline metrics
- **Actions**: Logging only

### Mode: `diagnose`
- **Behavior**: Diagnose failures and log proposed actions without executing them
- **Use Case**: Validate diagnosis accuracy before enabling healing
- **Actions**: Diagnosis + logging (no healing)

### Mode: `conservative` (Recommended)
- **Behavior**: Heal safe patterns only (whitelisted prompts, known recoverable errors)
- **Use Case**: Production deployment with safety controls
- **Actions**: Safe healing only

### Mode: `autopilot`
- **Behavior**: Full autonomous healing for all detected issues
- **Use Case**: Maximum automation after validation
- **Actions**: All healing actions enabled

---

## Healing Metadata

When Superbrain takes autonomous actions, the response includes healing metadata:

### Successful Response with Healing

```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "model": "geminicli:gemini-2.5-pro",
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "Here is the response..."
      }
    }
  ],
  "superbrain": {
    "healed": true,
    "original_provider": "claudecli",
    "final_provider": "geminicli",
    "healing_actions": [
      {
        "timestamp": "2026-01-26T10:30:45Z",
        "action_type": "stdin_injection",
        "description": "Injected 'y' response to permission prompt",
        "success": true
      },
      {
        "timestamp": "2026-01-26T10:30:50Z",
        "action_type": "restart_with_flags",
        "description": "Restarted with --dangerously-skip-permissions",
        "success": true
      }
    ],
    "context_optimized": false,
    "total_healing_time_ms": 5200
  }
}
```

### Negotiated Failure Response

When all healing attempts fail, Superbrain returns an intelligent error response:

```json
{
  "error": {
    "message": "Request could not be completed after autonomous remediation attempts",
    "type": "negotiated_failure",
    "code": "healing_exhausted"
  },
  "superbrain": {
    "attempted_actions": [
      "stdin_injection: permission_prompt",
      "restart_with_flags: --dangerously-skip-permissions",
      "fallback_routing: geminicli, gemini"
    ],
    "diagnosis_summary": "Process repeatedly hung on permission prompt despite auto-approval",
    "suggestions": [
      "Check CLI tool version and update if needed",
      "Verify file permissions in working directory",
      "Try using API-based provider instead of CLI"
    ],
    "fallbacks_tried": ["geminicli", "gemini"]
  }
}
```

---

## Phased Rollout Strategy

Deploy Superbrain incrementally to manage risk:

### Phase 1: Monitoring-Only (Week 1)
```yaml
superbrain:
  enabled: true
  mode: "observe"
```

**Goal**: Establish baseline metrics without any autonomous actions.

**Success Criteria**:
- Metrics endpoint shows Superbrain data
- Audit log captures all detected issues
- No impact on existing functionality

### Phase 2: Diagnostics (Week 2)
```yaml
superbrain:
  enabled: true
  mode: "diagnose"
```

**Goal**: Validate diagnosis accuracy.

**Success Criteria**:
- Diagnosis correctly identifies failure types
- Proposed actions are appropriate
- No false positives in diagnosis

### Phase 3: Conservative Healing (Week 3-4)
```yaml
superbrain:
  enabled: true
  mode: "conservative"
```

**Goal**: Enable safe autonomous healing.

**Success Criteria**:
- Success rate improvement (measure before/after)
- No security incidents
- Healing metadata is accurate

### Phase 4: Full Autopilot (Week 5+)
```yaml
superbrain:
  enabled: true
  mode: "autopilot"
```

**Goal**: Maximum automation.

**Success Criteria**:
- Consistent success rate improvement
- Reduced manual intervention
- Stable operation over time

---

## Monitoring & Metrics

### Metrics Endpoint

Superbrain exposes metrics via the existing management endpoint:

```bash
curl http://localhost:18080/management/metrics
```

**Superbrain Metrics:**
```json
{
  "superbrain": {
    "total_healing_attempts": 142,
    "successful_healings": 128,
    "failed_healings": 14,
    "healing_by_type": {
      "stdin_injection": 45,
      "restart_with_flags": 38,
      "fallback_routing": 32,
      "context_optimization": 13
    },
    "average_healing_latency_ms": 3200,
    "silence_detections": 89,
    "diagnoses_performed": 89,
    "fallbacks_triggered": 32
  }
}
```

### Audit Log

All autonomous actions are logged to the audit log:

```bash
tail -f logs/superbrain_audit.log
```

**Log Entry Format:**
```json
{
  "timestamp": "2026-01-26T10:30:45Z",
  "request_id": "req-abc123",
  "action_type": "stdin_injection",
  "provider": "claudecli",
  "model": "claude-sonnet-4",
  "action_details": {
    "pattern_matched": "claude_file_permission",
    "response_injected": "y\n"
  },
  "outcome": "success"
}
```

---

## Security & Safety

### Stdin Injection Whitelist

Only safe patterns are auto-approved by default:

```yaml
# Built-in safe patterns
- claude_file_permission: "Allow read file? [y/n]" → "y"
- claude_tool_permission: "Allow tool execution? [y/n]" → "y"
- generic_continue: "Press enter to continue" → "\n"
```

### Forbidden Patterns

Patterns that will NEVER be auto-approved:

```yaml
stdin_injection:
  forbidden_patterns:
    - "delete"
    - "remove"
    - "sudo"
    - "rm -rf"
```

### Security Fail-Safe

If a security-sensitive operation is detected, Superbrain will:
1. Abort autonomous remediation
2. Return a safe failure response
3. Log the incident to the audit log

**Example:**
```json
{
  "error": {
    "message": "Security-sensitive operation detected, autonomous remediation aborted",
    "type": "security_failsafe",
    "code": "forbidden_operation"
  }
}
```

---

## Emergency Procedures

### Emergency Disable

If Superbrain causes issues, disable it immediately:

```yaml
superbrain:
  enabled: false
```

Or via environment variable:
```bash
export SUPERBRAIN_ENABLED=false
./switchAILocal
```

The system will revert to legacy pass-through mode instantly.

### Hot-Reload Configuration

Configuration changes take effect without restart:

```bash
# Edit config.yaml
vim config.yaml

# Changes apply to new requests automatically
# No restart needed
```

### Rollback Procedure

1. Set mode to `observe` or `disabled`
2. Monitor metrics for stability
3. Review audit log for issues
4. Adjust configuration as needed
5. Gradually re-enable features

---

## Troubleshooting

### Issue: Superbrain not activating

**Check:**
1. `superbrain.enabled: true` in config.yaml
2. Mode is not `disabled`
3. Provider supports CLI execution (Superbrain only monitors CLI providers)

### Issue: Too many healing attempts

**Solution:**
```yaml
overwatch:
  max_restart_attempts: 1  # Reduce from default 2
```

### Issue: Unwanted stdin injections

**Solution:**
```yaml
stdin_injection:
  mode: "disabled"  # Disable stdin injection entirely
```

Or use conservative mode with explicit whitelist:
```yaml
stdin_injection:
  mode: "conservative"
  custom_patterns:
    - name: "my_safe_pattern"
      regex: "Continue\\? \\[y/n\\]"
      response: "y\n"
      is_safe: true
```

### Issue: Diagnosis taking too long

**Solution:**
```yaml
doctor:
  timeout_ms: 3000  # Reduce from default 5000ms
```

### Issue: Context optimization too aggressive

**Solution:**
```yaml
context_sculptor:
  enabled: false  # Disable optimization
```

Or adjust priority files:
```yaml
context_sculptor:
  priority_files:
    - "README.md"
    - "main.go"
    - "**/*.md"  # Include all markdown files
```

---

## Best Practices

### 1. Start with Observe Mode
Always deploy in `observe` mode first to establish baseline metrics.

### 2. Monitor Audit Logs
Regularly review audit logs to understand what actions Superbrain is taking.

### 3. Use Conservative Mode in Production
The `conservative` mode provides the best balance of automation and safety.

### 4. Tune Silence Threshold
Adjust based on your typical request latency:
- Fast models (< 10s): 15000ms
- Medium models (10-30s): 30000ms (default)
- Slow models (> 30s): 60000ms

### 5. Configure Fallback Order
List providers in order of preference:
```yaml
fallback:
  providers:
    - "geminicli"    # Preferred
    - "gemini"       # Fallback 1
    - "ollama"       # Fallback 2
```

### 6. Enable Context Sculptor for Large Requests
If you frequently send large file/folder references, enable context optimization:
```yaml
context_sculptor:
  enabled: true
```

### 7. Review Metrics Regularly
Check metrics endpoint weekly to track:
- Success rate improvements
- Healing action distribution
- Average healing latency

---

## Advanced Configuration

### Per-Provider Overrides

Configure different settings for specific providers:

```yaml
superbrain:
  enabled: true
  mode: "conservative"
  
  # Provider-specific overrides
  provider_overrides:
    claudecli:
      overwatch:
        silence_threshold_ms: 45000  # Claude is slower
      stdin_injection:
        mode: "autopilot"  # More aggressive for Claude
    
    geminicli:
      overwatch:
        silence_threshold_ms: 20000  # Gemini is faster
```

### Custom Stdin Patterns

Add your own prompt patterns:

```yaml
stdin_injection:
  custom_patterns:
    - name: "my_custom_prompt"
      regex: "Do you want to proceed\\? \\(yes/no\\)"
      response: "yes\n"
      is_safe: true
      description: "Custom application prompt"
```

### Diagnostic Model Selection

Use different models for diagnosis:

```yaml
doctor:
  model: "gemini-flash"      # Fast and cheap
  # model: "claude-haiku"    # Alternative
  # model: "ollama:llama3.2" # Local model
```

---

## FAQ

### Q: Does Superbrain work with API-based providers?
**A:** Superbrain primarily monitors CLI-based providers (claudecli, geminicli, etc.). API-based providers already have built-in error handling and don't suffer from silent hangs.

### Q: Will Superbrain increase my costs?
**A:** Minimal. The Internal Doctor uses a lightweight model (gemini-flash by default) for diagnosis, which costs fractions of a cent per diagnosis.

### Q: Can I disable specific components?
**A:** Yes, each component can be independently disabled:
```yaml
superbrain:
  enabled: true
  overwatch:
    enabled: true
  doctor:
    enabled: true
  stdin_injection:
    enabled: false  # Disable this component
  context_sculptor:
    enabled: true
  fallback:
    enabled: true
```

### Q: How do I know if Superbrain is working?
**A:** Check the metrics endpoint or look for the `superbrain` field in response metadata.

### Q: What happens if the diagnostic model is unavailable?
**A:** Superbrain falls back to pattern-matching-only diagnosis (regex-based detection).

### Q: Can I use Superbrain with streaming requests?
**A:** Yes, Superbrain fully supports streaming requests and monitors them in real-time.

---

## Related Documentation

- [Configuration Guide](configuration.md) - All config.yaml options
- [Provider Setup](providers.md) - Connect your AI providers
- [API Reference](api-reference.md) - Endpoints and examples
- [Advanced Features](advanced-features.md) - Payload overrides, TLS, etc.

---

**Need Help?** Open an issue on [GitHub](https://github.com/traylinx/switchAILocal/issues) or check the audit logs for detailed diagnostic information.
