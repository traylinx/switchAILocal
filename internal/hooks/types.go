package hooks

import (
	"time"
)

// HookEvent defines the type of event that can trigger a hook.
type HookEvent string

const (
	EventRequestReceived     HookEvent = "request_received"
	EventRequestFailed       HookEvent = "request_failed"
	EventProviderUnavailable HookEvent = "provider_unavailable"
	EventQuotaWarning        HookEvent = "quota_warning"
	EventQuotaExceeded       HookEvent = "quota_exceeded"
	EventModelDiscovered     HookEvent = "model_discovered"
	EventHealthCheckFailed   HookEvent = "health_check_failed"
	EventRoutingDecision     HookEvent = "routing_decision"
)

// HookAction defines the action to be performed when a hook is triggered.
type HookAction string

const (
	ActionRetryWithFallback HookAction = "retry_with_fallback"
	ActionNotifyWebhook     HookAction = "notify_webhook"
	ActionPreferProvider    HookAction = "prefer_provider"
	ActionLogWarning        HookAction = "log_warning"
	ActionRotateCredential  HookAction = "rotate_credential"
	ActionRestartProvider   HookAction = "restart_provider"
	ActionRunCommand        HookAction = "run_command"
)

// Hook represents a single automation rule.
type Hook struct {
	ID          string         `yaml:"id" json:"id"`
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	Event       HookEvent      `yaml:"event" json:"event"`
	Condition   string         `yaml:"condition" json:"condition"`
	Action      HookAction     `yaml:"action" json:"action"`
	Params      map[string]any `yaml:"params" json:"params"`
	Enabled     bool           `yaml:"enabled" json:"enabled"`

	// FilePath is the source file (not in YAML)
	FilePath string `yaml:"-" json:"-"`
}

// EventContext provides the environment for hook execution.
type EventContext struct {
	Event        HookEvent              `json:"event"`
	Timestamp    time.Time              `json:"timestamp"`
	Data         map[string]interface{} `json:"data"`
	Request      any                    `json:"request,omitempty"` // interface{} to avoid import cycle
	Provider     string                 `json:"provider,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Error        error                  `json:"-"`
	ErrorMessage string                 `json:"error,omitempty"`
}

// ActionHandler is a function that executes a hook action.
type ActionHandler func(hook *Hook, ctx *EventContext) error
