// Package superbrain provides intelligent orchestration and self-healing capabilities
// for the switchAILocal gateway.
//
// # Overview
//
// Superbrain transforms switchAILocal from a passive API gateway into an autonomous,
// self-healing AI orchestrator. Instead of treating errors as terminal states, Superbrain
// actively monitors executions, diagnoses failures using AI, and takes autonomous
// remediation actions to ensure requests succeed.
//
// # Architecture
//
// The Superbrain architecture implements an "Observer-Critic" duality where every
// execution is monitored by a lightweight Supervisor that can:
//
//   - Detect failures in real-time (within seconds, not minutes)
//   - Diagnose issues using AI (permission prompts, auth errors, context limits)
//   - Take autonomous healing actions (stdin injection, process restart, intelligent failover)
//   - Report all actions transparently (healing metadata in responses)
//
// # Core Components
//
// The Superbrain system consists of several specialized components:
//
//   - overwatch: Real-time monitoring of CLI process execution with heartbeat detection
//   - doctor: AI-powered failure diagnosis using lightweight models
//   - injector: Autonomous stdin injection to respond to interactive prompts
//   - recovery: Process restart with corrective flags based on diagnosis
//   - router: Intelligent failover routing to alternative providers
//   - sculptor: Pre-flight content analysis and optimization for context limits
//   - metadata: Aggregation of healing actions for transparent reporting
//   - audit: Structured logging of all autonomous actions for security review
//   - metrics: Observability infrastructure for monitoring healing effectiveness
//   - security: Safety controls and fail-safes for autonomous operations
//
// # Usage Example
//
// The SuperbrainExecutor wraps existing provider executors to add self-healing:
//
//	// Create a Superbrain-enhanced executor
//	executor := superbrain.NewSuperbrainExecutor(
//	    baseExecutor,
//	    config.Superbrain,
//	)
//
//	// Execute requests with automatic healing
//	response, err := executor.Execute(ctx, auth, request, opts)
//
//	// Check if healing occurred
//	if metadata, ok := response.Extra["superbrain"].(map[string]interface{}); ok {
//	    if healed, _ := metadata["healed"].(bool); healed {
//	        log.Info("Request was healed by Superbrain")
//	    }
//	}
//
// # Operational Modes
//
// Superbrain supports multiple operational modes for gradual rollout:
//
//   - disabled: Superbrain is completely disabled (legacy pass-through mode)
//   - observe: Monitor and log, but take no autonomous actions
//   - diagnose: Diagnose failures and log proposed actions without executing them
//   - conservative: Heal safe patterns only (whitelisted prompts, known recoverable errors)
//   - autopilot: Full autonomous healing for all detected issues
//
// # Configuration
//
// Superbrain is configured via the config.yaml file:
//
//	superbrain:
//	  enabled: true
//	  mode: "conservative"
//	  overwatch:
//	    silence_threshold_ms: 30000
//	    max_restart_attempts: 2
//	  doctor:
//	    model: "gemini-flash"
//	  stdin_injection:
//	    mode: "conservative"
//	  fallback:
//	    enabled: true
//	    providers: ["geminicli", "gemini", "ollama"]
//
// # Security & Safety
//
// Superbrain includes multiple safety controls:
//
//   - Stdin injection whitelist: Only safe patterns are auto-approved
//   - Forbidden operations: Security-sensitive operations require human approval
//   - Audit logging: All autonomous actions are logged for review
//   - Restart limits: Prevents infinite healing loops
//   - Emergency disable: Single config flag reverts to pass-through mode
//
// # Monitoring
//
// Superbrain exposes comprehensive metrics for observability:
//
//   - Total healing attempts and success rate
//   - Healing actions by type (stdin injection, restart, fallback, etc.)
//   - Average healing latency
//   - Silence detections and diagnoses performed
//
// Access metrics via the management endpoint:
//
//	curl http://localhost:18080/management/metrics
//
// # Documentation
//
// For detailed user documentation, see docs/user/superbrain.md
//
// For implementation details, see the design document at
// .kiro/specs/superbrain-intelligence/design.md
package superbrain
