# Superbrain Phased Rollout Guide

This document provides a comprehensive guide for safely rolling out Superbrain capabilities in production environments. The phased approach allows you to gradually enable features while monitoring their impact and maintaining the ability to quickly rollback if issues arise.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Rollout Phases](#rollout-phases)
- [Phase Transition Criteria](#phase-transition-criteria)
- [Rollback Procedures](#rollback-procedures)
- [Monitoring and Validation](#monitoring-and-validation)
- [Troubleshooting](#troubleshooting)

## Overview

The Superbrain rollout follows a progressive enhancement strategy:

1. **Phase 0: Quick Fix** - Immediate value with minimal risk
2. **Phase 1: Observe Mode** - Monitor without autonomous actions
3. **Phase 2: Diagnose Mode** - Add AI-powered diagnosis
4. **Phase 3: Conservative Mode** - Enable safe autonomous actions
5. **Phase 4: Autopilot Mode** - Full autonomous capabilities

Each phase builds on the previous one, allowing you to validate behavior before proceeding.

## Prerequisites

Before beginning the rollout, ensure:

- [ ] switchAILocal is running version with Superbrain support
- [ ] You have access to the configuration file (`config.yaml`)
- [ ] You can monitor logs and metrics endpoints
- [ ] You have a rollback plan and can quickly edit configuration
- [ ] You understand your current failure patterns and baseline metrics

## Rollout Phases

### Phase 0: Quick Fix (Immediate Deployment)

**Goal**: Solve the critical "Silent Hang" problem with minimal code change.

**Configuration**:
```yaml
superbrain:
  enabled: false  # Not needed for Phase 0
```

**What Happens**:
- Auto-inject `--dangerously-skip-permissions` for `claudecli` without TTY
- No AI diagnosis, no healing loops
- Immediate fix for permission prompt hangs

**Success Criteria**:
- [ ] Claude CLI requests no longer hang on permission prompts
- [ ] No increase in error rates
- [ ] Response times remain stable

**Duration**: Deploy immediately, monitor for 24 hours

**Rollback**: Remove the auto-injection code if issues occur

---

### Phase 1: Observe Mode

**Goal**: Enable monitoring and logging without any autonomous actions.

**Configuration**:
```yaml
superbrain:
  enabled: true
  mode: "observe"
  
  component_flags:
    overwatch_enabled: true      # Enable monitoring
    doctor_enabled: false        # Disable diagnosis
    injector_enabled: false      # Disable injection
    recovery_enabled: false      # Disable recovery
    fallback_enabled: false      # Disable fallback
    sculptor_enabled: false      # Disable optimization
  
  overwatch:
    silence_threshold_ms: 30000
    log_buffer_size: 50
    heartbeat_interval_ms: 1000
  
  security:
    audit_log_enabled: true
    audit_log_path: "./logs/superbrain_audit.log"
```

**What Happens**:
- Real-time monitoring of all CLI executions
- Silence detection and snapshot capture
- Logging of potential issues
- **No autonomous actions taken**

**Monitoring**:
```bash
# Check metrics endpoint
curl http://localhost:18080/management/metrics

# Monitor audit log
tail -f ./logs/superbrain_audit.log

# Check for silence detections
grep "silence_detected" ./logs/superbrain_audit.log
```

**Success Criteria**:
- [ ] Monitoring is capturing execution data
- [ ] Silence detections are being logged
- [ ] No performance degradation (< 5ms overhead)
- [ ] Audit log is being written correctly
- [ ] No false positive silence detections

**Duration**: 3-7 days

**Rollback**:
```yaml
superbrain:
  enabled: false
```

---

### Phase 2: Diagnose Mode

**Goal**: Add AI-powered failure diagnosis without taking action.

**Configuration**:
```yaml
superbrain:
  enabled: true
  mode: "diagnose"
  
  component_flags:
    overwatch_enabled: true
    doctor_enabled: true         # Enable diagnosis
    injector_enabled: false
    recovery_enabled: false
    fallback_enabled: false
    sculptor_enabled: false
  
  doctor:
    model: "gemini-flash"
    timeout_ms: 5000
```

**What Happens**:
- Continues monitoring from Phase 1
- AI analyzes failures and logs diagnosis
- Proposed remediation actions are logged but not executed
- Pattern matching for known failure types

**Monitoring**:
```bash
# Check diagnosis quality
grep "diagnosis" ./logs/superbrain_audit.log | jq .

# Monitor diagnosis latency
grep "diagnosis_latency_ms" ./logs/superbrain_audit.log

# Review proposed actions
grep "proposed_action" ./logs/superbrain_audit.log
```

**Success Criteria**:
- [ ] Diagnoses are accurate for known failure patterns
- [ ] Diagnosis completes within timeout (< 5 seconds)
- [ ] Proposed actions are appropriate
- [ ] No increase in overall latency
- [ ] Doctor model is available and responding

**Duration**: 3-7 days

**Rollback**:
```yaml
superbrain:
  mode: "observe"
  component_flags:
    doctor_enabled: false
```

---

### Phase 3: Conservative Mode

**Goal**: Enable autonomous healing for whitelisted safe actions only.

**Configuration**:
```yaml
superbrain:
  enabled: true
  mode: "conservative"
  
  component_flags:
    overwatch_enabled: true
    doctor_enabled: true
    injector_enabled: true       # Enable safe injection
    recovery_enabled: true       # Enable restart
    fallback_enabled: true       # Enable fallback
    sculptor_enabled: true       # Enable optimization
  
  stdin_injection:
    mode: "conservative"
    forbidden_patterns:
      - "delete"
      - "remove"
      - "sudo"
  
  overwatch:
    max_restart_attempts: 2
  
  fallback:
    enabled: true
    providers:
      - "geminicli"
      - "gemini"
      - "ollama"
    min_success_rate: 0.5
  
  context_sculptor:
    enabled: true
    token_estimator: "tiktoken"
```

**What Happens**:
- Autonomous stdin injection for whitelisted patterns only
- Process restart with corrective flags (max 2 attempts)
- Fallback routing to alternative providers
- Pre-flight content optimization
- All actions are logged and included in response metadata

**Monitoring**:
```bash
# Monitor healing actions
grep "healing_action" ./logs/superbrain_audit.log

# Check success rates
curl http://localhost:18080/management/metrics | jq '.superbrain'

# Review stdin injections
grep "stdin_injection" ./logs/superbrain_audit.log

# Monitor fallback routing
grep "fallback_routing" ./logs/superbrain_audit.log
```

**Success Criteria**:
- [ ] Healing actions are successful > 80% of the time
- [ ] No unintended stdin injections
- [ ] Restart attempts stay within limits
- [ ] Fallback routing works correctly
- [ ] Context optimization reduces failures
- [ ] Overall success rate improves
- [ ] No security incidents

**Duration**: 7-14 days

**Rollback**:
```yaml
superbrain:
  mode: "diagnose"
  component_flags:
    injector_enabled: false
    recovery_enabled: false
    fallback_enabled: false
    sculptor_enabled: false
```

---

### Phase 4: Autopilot Mode

**Goal**: Enable full autonomous healing capabilities.

**Configuration**:
```yaml
superbrain:
  enabled: true
  mode: "autopilot"
  
  component_flags:
    overwatch_enabled: true
    doctor_enabled: true
    injector_enabled: true
    recovery_enabled: true
    fallback_enabled: true
    sculptor_enabled: true
  
  stdin_injection:
    mode: "autopilot"            # Auto-approve all safe patterns
    forbidden_patterns:
      - "delete"
      - "remove"
      - "sudo"
  
  security:
    forbidden_operations:
      - "file_delete"
      - "system_command"
```

**What Happens**:
- All safe patterns are automatically approved
- Full autonomous healing capabilities
- Forbidden operations still require human approval
- Maximum resilience and self-healing

**Monitoring**:
```bash
# Monitor all autonomous actions
tail -f ./logs/superbrain_audit.log

# Check healing effectiveness
curl http://localhost:18080/management/metrics | jq '.superbrain.healing_success_rate'

# Review security blocks
grep "forbidden_operation" ./logs/superbrain_audit.log
```

**Success Criteria**:
- [ ] Healing success rate > 85%
- [ ] No security violations
- [ ] User satisfaction with autonomous behavior
- [ ] Reduced manual intervention required
- [ ] Improved overall reliability

**Duration**: Ongoing production use

**Rollback**:
```yaml
superbrain:
  mode: "conservative"
  stdin_injection:
    mode: "conservative"
```

---

## Phase Transition Criteria

### General Criteria for All Phases

Before transitioning to the next phase, verify:

1. **Stability**: No crashes or unexpected behavior for at least 48 hours
2. **Performance**: Latency increase < 10% from baseline
3. **Accuracy**: Diagnosis/actions are correct > 90% of the time
4. **Monitoring**: All metrics are being collected and reviewed
5. **Team Confidence**: Team is comfortable with current behavior

### Specific Transition Criteria

**Observe → Diagnose**:
- [ ] Monitoring overhead is acceptable (< 5ms)
- [ ] Silence detection is working correctly
- [ ] No false positives in monitoring
- [ ] Audit log is complete and accessible

**Diagnose → Conservative**:
- [ ] Diagnosis accuracy > 90%
- [ ] Proposed actions are appropriate
- [ ] Diagnosis latency < 5 seconds
- [ ] Team has reviewed diagnosis patterns

**Conservative → Autopilot**:
- [ ] Healing success rate > 80%
- [ ] No unintended actions in conservative mode
- [ ] Security controls are working
- [ ] Team is confident in autonomous behavior

---

## Rollback Procedures

### Emergency Rollback (Immediate)

If critical issues occur, immediately disable Superbrain:

```yaml
superbrain:
  enabled: false
```

Or set mode to disabled:

```yaml
superbrain:
  mode: "disabled"
```

**Hot-reload the configuration**:
```bash
# The configuration will be automatically reloaded
# Or send SIGHUP to the process
kill -HUP $(pgrep switchAILocal)
```

### Gradual Rollback

If issues are not critical, roll back one phase:

1. **From Autopilot → Conservative**:
   ```yaml
   superbrain:
     mode: "conservative"
     stdin_injection:
       mode: "conservative"
   ```

2. **From Conservative → Diagnose**:
   ```yaml
   superbrain:
     mode: "diagnose"
     component_flags:
       injector_enabled: false
       recovery_enabled: false
       fallback_enabled: false
       sculptor_enabled: false
   ```

3. **From Diagnose → Observe**:
   ```yaml
   superbrain:
     mode: "observe"
     component_flags:
       doctor_enabled: false
   ```

### Component-Level Rollback

Disable specific components while keeping others active:

```yaml
superbrain:
  component_flags:
    overwatch_enabled: true
    doctor_enabled: true
    injector_enabled: false      # Disable just this component
    recovery_enabled: true
    fallback_enabled: true
    sculptor_enabled: true
```

---

## Monitoring and Validation

### Key Metrics to Monitor

1. **Healing Metrics**:
   - `superbrain.healing_attempts_total` - Total healing attempts
   - `superbrain.healing_success_total` - Successful healings
   - `superbrain.healing_failure_total` - Failed healings
   - `superbrain.healing_latency_ms` - Time spent healing

2. **Component Metrics**:
   - `superbrain.silence_detections_total` - Silence detections
   - `superbrain.diagnoses_performed_total` - Diagnoses performed
   - `superbrain.stdin_injections_total` - Stdin injections
   - `superbrain.restarts_total` - Process restarts
   - `superbrain.fallbacks_total` - Fallback routings

3. **Performance Metrics**:
   - Request latency (p50, p95, p99)
   - Error rate
   - Success rate
   - Throughput

### Validation Checklist

After each phase transition, validate:

- [ ] Metrics endpoint is accessible
- [ ] Audit log is being written
- [ ] No error spikes in application logs
- [ ] Response times are within acceptable range
- [ ] Healing actions are appropriate
- [ ] No security violations
- [ ] User experience is improved or unchanged

### Monitoring Commands

```bash
# Check Superbrain metrics
curl http://localhost:18080/management/metrics | jq '.superbrain'

# Monitor audit log in real-time
tail -f ./logs/superbrain_audit.log | jq .

# Check for errors
grep "ERROR" ./logs/switchailocal.log | tail -20

# Monitor healing success rate
watch -n 5 'curl -s http://localhost:18080/management/metrics | jq ".superbrain.healing_success_rate"'
```

---

## Troubleshooting

### Common Issues and Solutions

#### Issue: High False Positive Rate for Silence Detection

**Symptoms**: Many silence detections for processes that are actually working

**Solution**:
```yaml
superbrain:
  overwatch:
    silence_threshold_ms: 60000  # Increase threshold to 60 seconds
```

#### Issue: Diagnosis Timeouts

**Symptoms**: Frequent diagnosis timeout errors

**Solution**:
```yaml
superbrain:
  doctor:
    timeout_ms: 10000  # Increase timeout to 10 seconds
    model: "gemini-flash"  # Ensure using fast model
```

#### Issue: Unwanted Stdin Injections

**Symptoms**: Stdin injection happening for unexpected patterns

**Solution**:
```yaml
superbrain:
  stdin_injection:
    mode: "conservative"  # Switch to conservative mode
    forbidden_patterns:
      - "pattern-to-block"  # Add specific patterns to block
```

#### Issue: Too Many Restart Attempts

**Symptoms**: Processes being restarted repeatedly

**Solution**:
```yaml
superbrain:
  overwatch:
    max_restart_attempts: 1  # Reduce restart attempts
```

#### Issue: Fallback Routing Not Working

**Symptoms**: Requests not routing to fallback providers

**Solution**:
1. Verify fallback providers are configured and available
2. Check provider success rates meet minimum threshold
3. Review audit log for fallback decisions

```yaml
superbrain:
  fallback:
    min_success_rate: 0.3  # Lower threshold temporarily
```

### Emergency Contacts

- **Superbrain Issues**: Check GitHub issues or documentation
- **Configuration Help**: Review `config.example.yaml`
- **Security Concerns**: Review audit log and disable immediately if needed

---

## Best Practices

1. **Start Slow**: Don't skip phases, even if you're confident
2. **Monitor Continuously**: Set up alerts for key metrics
3. **Review Audit Logs**: Regularly review autonomous actions
4. **Test Rollback**: Practice rollback procedures before issues occur
5. **Document Learnings**: Keep notes on what works for your environment
6. **Communicate**: Keep team informed of phase transitions
7. **Be Conservative**: When in doubt, stay in a lower phase longer

---

## Summary

The phased rollout approach ensures safe deployment of Superbrain capabilities:

- **Phase 0**: Quick fix (immediate)
- **Phase 1**: Observe mode (3-7 days)
- **Phase 2**: Diagnose mode (3-7 days)
- **Phase 3**: Conservative mode (7-14 days)
- **Phase 4**: Autopilot mode (production)

Total rollout time: 2-4 weeks for full autopilot mode.

Remember: You can always roll back to a previous phase or disable Superbrain entirely if issues arise. The configuration hot-reloads automatically, so changes take effect immediately without server restart.
