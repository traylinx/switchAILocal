package steering

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextInjector_InjectSystemPrompt(t *testing.T) {
	injector := NewContextInjector()

	// Test empty messages
	messages := []map[string]string{}
	result := injector.InjectSystemPrompt(messages, "System prompt")
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "system", result[0]["role"])
	assert.Equal(t, "System prompt", result[0]["content"])

	// Test messages with existing system message
	messages = []map[string]string{
		{"role": "system", "content": "Existing system"},
		{"role": "user", "content": "Hello"},
	}
	result = injector.InjectSystemPrompt(messages, "New system")
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "system", result[0]["role"])
	assert.Equal(t, "New system\n\nExisting system", result[0]["content"])

	// Test messages without system message
	messages = []map[string]string{
		{"role": "user", "content": "Hello"},
	}
	result = injector.InjectSystemPrompt(messages, "System prompt")
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "system", result[0]["role"])
	assert.Equal(t, "System prompt", result[0]["content"])
	assert.Equal(t, "user", result[1]["role"])
	assert.Equal(t, "Hello", result[1]["content"])

	// Test empty prompt
	result = injector.InjectSystemPrompt(messages, "")
	assert.Equal(t, messages, result)
}

func TestContextInjector_ApplyProviderSettings(t *testing.T) {
	injector := NewContextInjector()

	// Test nil metadata
	settings := map[string]any{
		"temperature": 0.7,
		"max_tokens":  2048,
	}
	result := injector.ApplyProviderSettings(nil, settings)
	assert.NotNil(t, result)
	assert.Equal(t, 0.7, result["steering_temperature"])
	assert.Equal(t, 2048, result["steering_max_tokens"])

	// Test existing metadata
	metadata := map[string]interface{}{
		"existing": "value",
	}
	result = injector.ApplyProviderSettings(metadata, settings)
	assert.Equal(t, "value", result["existing"])
	assert.Equal(t, 0.7, result["steering_temperature"])
	assert.Equal(t, 2048, result["steering_max_tokens"])

	// Test empty settings
	result = injector.ApplyProviderSettings(metadata, map[string]any{})
	assert.Equal(t, metadata, result)
}

func TestContextInjector_FormatContextInjection(t *testing.T) {
	injector := NewContextInjector()

	ctx := &RoutingContext{
		Intent: "coding",
		Model:  "claude-sonnet",
		Hour:   14,
	}

	// Test template with variables
	template := "You are helping with {{intent}} tasks using {{model}} at hour {{hour}}"
	result := injector.FormatContextInjection(template, ctx)
	expected := "You are helping with coding tasks using claude-sonnet at hour 14"
	assert.Equal(t, expected, result)

	// Test template without variables
	template = "Simple prompt"
	result = injector.FormatContextInjection(template, ctx)
	assert.Equal(t, "Simple prompt", result)

	// Test empty template
	result = injector.FormatContextInjection("", ctx)
	assert.Equal(t, "", result)
}