// Package router provides intelligent failover routing between AI providers.
package router

import (
	"encoding/json"
	"errors"
)

var (
	// ErrInvalidRequest indicates the request could not be parsed.
	ErrInvalidRequest = errors.New("invalid request format")

	// ErrUnsupportedProvider indicates the target provider is not supported for adaptation.
	ErrUnsupportedProvider = errors.New("unsupported provider for adaptation")
)

// ChatRequest represents an OpenAI-compatible chat completion request.
// This is the common format used for request adaptation.
type ChatRequest struct {
	// Model is the model identifier.
	Model string `json:"model"`

	// Messages is the conversation history.
	Messages []ChatMessage `json:"messages"`

	// Stream indicates whether to stream the response.
	Stream bool `json:"stream,omitempty"`

	// Temperature controls randomness (0.0 to 2.0).
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens limits the response length.
	MaxTokens *int `json:"max_tokens,omitempty"`

	// TopP controls nucleus sampling.
	TopP *float64 `json:"top_p,omitempty"`

	// Stop sequences that halt generation.
	Stop []string `json:"stop,omitempty"`

	// ExtraBody contains provider-specific fields.
	ExtraBody map[string]interface{} `json:"extra_body,omitempty"`
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	// Role is the message author (system, user, assistant).
	Role string `json:"role"`

	// Content is the message text.
	Content string `json:"content"`
}

// AdaptedRequest contains the adapted request and metadata about the adaptation.
type AdaptedRequest struct {
	// Payload is the adapted request JSON.
	Payload []byte

	// OriginalModel is the model from the original request.
	OriginalModel string

	// AdaptedModel is the model for the target provider.
	AdaptedModel string

	// PreservedSemantics lists what was preserved during adaptation.
	PreservedSemantics []string
}

// RequestAdapter adapts requests for fallback providers while preserving semantics.
type RequestAdapter struct {
	// modelMappings maps original models to fallback provider models.
	modelMappings map[string]map[string]string
}

// NewRequestAdapter creates a new request adapter with default model mappings.
func NewRequestAdapter() *RequestAdapter {
	return &RequestAdapter{
		modelMappings: defaultModelMappings(),
	}
}

// defaultModelMappings returns the default model mappings between providers.
func defaultModelMappings() map[string]map[string]string {
	return map[string]map[string]string{
		// Claude models to other providers
		"claude-3-5-sonnet-20241022": {
			"gemini":    "gemini-2.0-flash",
			"geminicli": "gemini-2.0-flash",
			"ollama":    "llama3.2",
			"openai":    "gpt-4o",
		},
		"claude-3-opus-20240229": {
			"gemini":    "gemini-2.0-pro",
			"geminicli": "gemini-2.0-pro",
			"ollama":    "llama3.2",
			"openai":    "gpt-4o",
		},
		"claude-3-haiku-20240307": {
			"gemini":    "gemini-2.0-flash",
			"geminicli": "gemini-2.0-flash",
			"ollama":    "llama3.2",
			"openai":    "gpt-4o-mini",
		},
		// Gemini models to other providers
		"gemini-2.0-flash": {
			"claudecli": "claude-3-5-sonnet-20241022",
			"ollama":    "llama3.2",
			"openai":    "gpt-4o",
		},
		"gemini-2.0-pro": {
			"claudecli": "claude-3-opus-20240229",
			"ollama":    "llama3.2",
			"openai":    "gpt-4o",
		},
		// OpenAI models to other providers
		"gpt-4o": {
			"gemini":    "gemini-2.0-pro",
			"geminicli": "gemini-2.0-pro",
			"claudecli": "claude-3-5-sonnet-20241022",
			"ollama":    "llama3.2",
		},
		"gpt-4o-mini": {
			"gemini":    "gemini-2.0-flash",
			"geminicli": "gemini-2.0-flash",
			"claudecli": "claude-3-haiku-20240307",
			"ollama":    "llama3.2",
		},
	}
}

// AdaptRequest adapts a request for a fallback provider.
// It preserves the original request semantics (messages, streaming, capabilities)
// while adapting the model name and provider-specific fields.
func (a *RequestAdapter) AdaptRequest(originalPayload []byte, targetProvider string) (*AdaptedRequest, error) {
	// Parse the original request
	var req ChatRequest
	if err := json.Unmarshal(originalPayload, &req); err != nil {
		return nil, ErrInvalidRequest
	}

	originalModel := req.Model

	// Adapt the model for the target provider
	adaptedModel := a.mapModel(originalModel, targetProvider)

	// Create the adapted request
	adaptedReq := ChatRequest{
		Model:       adaptedModel,
		Messages:    req.Messages, // Preserve messages exactly
		Stream:      req.Stream,   // Preserve streaming preference
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}

	// Adapt provider-specific fields in extra_body
	adaptedReq.ExtraBody = a.adaptExtraBody(req.ExtraBody, targetProvider)

	// Serialize the adapted request
	payload, err := json.Marshal(adaptedReq)
	if err != nil {
		return nil, err
	}

	// Track what was preserved
	preserved := []string{"messages", "stream"}
	if req.Temperature != nil {
		preserved = append(preserved, "temperature")
	}
	if req.MaxTokens != nil {
		preserved = append(preserved, "max_tokens")
	}
	if req.TopP != nil {
		preserved = append(preserved, "top_p")
	}
	if len(req.Stop) > 0 {
		preserved = append(preserved, "stop")
	}

	return &AdaptedRequest{
		Payload:            payload,
		OriginalModel:      originalModel,
		AdaptedModel:       adaptedModel,
		PreservedSemantics: preserved,
	}, nil
}

// mapModel maps an original model to an equivalent model for the target provider.
func (a *RequestAdapter) mapModel(originalModel, targetProvider string) string {
	// Check if we have a specific mapping for this model
	if providerMappings, ok := a.modelMappings[originalModel]; ok {
		if mappedModel, ok := providerMappings[targetProvider]; ok {
			return mappedModel
		}
	}

	// Fallback: use a default model for the target provider
	return a.getDefaultModel(targetProvider)
}

// getDefaultModel returns the default model for a provider.
func (a *RequestAdapter) getDefaultModel(provider string) string {
	defaults := map[string]string{
		"claudecli": "claude-3-5-sonnet-20241022",
		"gemini":    "gemini-2.0-flash",
		"geminicli": "gemini-2.0-flash",
		"ollama":    "llama3.2",
		"openai":    "gpt-4o",
		"lmstudio":  "default",
	}

	if model, ok := defaults[provider]; ok {
		return model
	}
	return "default"
}

// adaptExtraBody adapts provider-specific fields for the target provider.
func (a *RequestAdapter) adaptExtraBody(extraBody map[string]interface{}, targetProvider string) map[string]interface{} {
	if extraBody == nil {
		return nil
	}

	adapted := make(map[string]interface{})

	// Copy non-provider-specific fields
	for key, value := range extraBody {
		// Skip CLI-specific fields when targeting non-CLI providers
		if key == "cli" && !isCLIProvider(targetProvider) {
			continue
		}
		adapted[key] = value
	}

	// If empty after adaptation, return nil
	if len(adapted) == 0 {
		return nil
	}

	return adapted
}

// isCLIProvider returns true if the provider is CLI-based.
func isCLIProvider(provider string) bool {
	cliProviders := map[string]bool{
		"claudecli": true,
		"geminicli": true,
	}
	return cliProviders[provider]
}

// SetModelMapping sets a custom model mapping.
func (a *RequestAdapter) SetModelMapping(originalModel, targetProvider, mappedModel string) {
	if a.modelMappings[originalModel] == nil {
		a.modelMappings[originalModel] = make(map[string]string)
	}
	a.modelMappings[originalModel][targetProvider] = mappedModel
}

// GetModelMapping returns the mapped model for a given original model and target provider.
func (a *RequestAdapter) GetModelMapping(originalModel, targetProvider string) string {
	return a.mapModel(originalModel, targetProvider)
}
