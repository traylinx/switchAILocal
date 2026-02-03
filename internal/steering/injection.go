package steering

import (
	"fmt"
	"strings"
)

// ContextInjector handles modifications to RoutingRequests based on steering rules.
type ContextInjector struct{}

// NewContextInjector creates a new context injector.
func NewContextInjector() *ContextInjector {
	return &ContextInjector{}
}

// InjectSystemPrompt adds a system message to the beginning of the messages slice.
func (i *ContextInjector) InjectSystemPrompt(messages []map[string]string, prompt string) []map[string]string {
	if prompt == "" {
		return messages
	}

	// Create a new system message
	systemMsg := map[string]string{
		"role":    "system",
		"content": prompt,
	}

	// Check if there's already a system message at the start
	if len(messages) > 0 && messages[0]["role"] == "system" {
		// Append to existing system message
		newMessages := make([]map[string]string, len(messages))
		copy(newMessages, messages)
		newMessages[0]["content"] = prompt + "\n\n" + newMessages[0]["content"]
		return newMessages
	}

	// Prepend new system message
	newMessages := make([]map[string]string, len(messages)+1)
	newMessages[0] = systemMsg
	copy(newMessages[1:], messages)
	return newMessages
}

// ApplyProviderSettings applies specialized settings for the selected provider.
// This is used slightly later in the pipeline when the model is already selected,
// or it can be used to prepare a template of settings.
func (i *ContextInjector) ApplyProviderSettings(metadata map[string]interface{}, settings map[string]any) map[string]interface{} {
	if len(settings) == 0 {
		return metadata
	}

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	// Merge settings into metadata or specialized fields
	// For simplicity, we merge into metadata which can be used by handlers
	for k, v := range settings {
		metadata["steering_"+k] = v
	}

	return metadata
}

// FormatContextInjection templates the injection string if it contains variables.
func (i *ContextInjector) FormatContextInjection(template string, ctx *RoutingContext) string {
	// Basic string replacement for now
	// Real implementation could use text/template
	res := template
	res = strings.ReplaceAll(res, "{{intent}}", ctx.Intent)
	res = strings.ReplaceAll(res, "{{model}}", ctx.Model)
	res = strings.ReplaceAll(res, "{{hour}}", fmt.Sprintf("%d", ctx.Hour))
	return res
}
