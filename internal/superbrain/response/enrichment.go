// Package response provides response enrichment capabilities for the Superbrain system.
// It adds healing metadata to successful responses and creates negotiated failure
// responses for unrecoverable errors.
package response

import (
	"encoding/json"

	"github.com/traylinx/switchAILocal/internal/superbrain/metadata"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// Enricher provides response enrichment capabilities.
type Enricher struct {
	// includeMetadataOnSuccess controls whether to include metadata on successful responses.
	includeMetadataOnSuccess bool

	// includeMetadataOnlyWhenHealed controls whether to only include metadata when healing occurred.
	includeMetadataOnlyWhenHealed bool
}

// NewEnricher creates a new response enricher.
func NewEnricher() *Enricher {
	return &Enricher{
		includeMetadataOnSuccess:      true,
		includeMetadataOnlyWhenHealed: true,
	}
}

// SetIncludeMetadataOnSuccess controls whether to include metadata on successful responses.
func (e *Enricher) SetIncludeMetadataOnSuccess(include bool) {
	e.includeMetadataOnSuccess = include
}

// SetIncludeMetadataOnlyWhenHealed controls whether to only include metadata when healing occurred.
func (e *Enricher) SetIncludeMetadataOnlyWhenHealed(onlyWhenHealed bool) {
	e.includeMetadataOnlyWhenHealed = onlyWhenHealed
}

// EnrichResponse adds healing metadata to a successful response.
// If no healing actions were taken and includeMetadataOnlyWhenHealed is true,
// the response is returned unchanged.
func (e *Enricher) EnrichResponse(resp switchailocalexecutor.Response, aggregator *metadata.Aggregator) switchailocalexecutor.Response {
	if aggregator == nil {
		return resp
	}

	// Check if we should include metadata
	if !e.includeMetadataOnSuccess {
		return resp
	}

	if e.includeMetadataOnlyWhenHealed && !aggregator.HasActions() {
		return resp
	}

	// Get the healing metadata
	healingMeta := aggregator.GetMetadata()

	// Try to parse the response payload as JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		// If we can't parse as JSON, return unchanged
		return resp
	}

	// Add superbrain extension
	payload["superbrain"] = healingMeta.ToOpenAIExtension()["superbrain"]

	// Re-marshal the payload
	enrichedPayload, err := json.Marshal(payload)
	if err != nil {
		// If we can't marshal, return unchanged
		return resp
	}

	resp.Payload = enrichedPayload
	return resp
}

// EnrichResponseWithMetadata adds specific healing metadata to a response.
func (e *Enricher) EnrichResponseWithMetadata(resp switchailocalexecutor.Response, healingMeta *types.HealingMetadata) switchailocalexecutor.Response {
	if healingMeta == nil {
		return resp
	}

	// Check if we should include metadata
	if !e.includeMetadataOnSuccess {
		return resp
	}

	if e.includeMetadataOnlyWhenHealed && len(healingMeta.Actions) == 0 {
		return resp
	}

	// Try to parse the response payload as JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		return resp
	}

	// Add superbrain extension
	payload["superbrain"] = healingMeta.ToOpenAIExtension()["superbrain"]

	// Re-marshal the payload
	enrichedPayload, err := json.Marshal(payload)
	if err != nil {
		return resp
	}

	resp.Payload = enrichedPayload
	return resp
}

// CreateNegotiatedFailureResponse creates a NegotiatedFailureResponse from an error and metadata.
func (e *Enricher) CreateNegotiatedFailureResponse(err error, aggregator *metadata.Aggregator, diagnosis *types.Diagnosis) *types.NegotiatedFailureResponse {
	nf := &types.NegotiatedFailureResponse{}

	// Set error information
	if err != nil {
		nf.Error.Message = err.Error()
	} else {
		nf.Error.Message = "Request failed"
	}
	nf.Error.Type = "execution_failed"
	nf.Error.Code = "superbrain_failure"

	// Set Superbrain information from aggregator
	if aggregator != nil {
		healingMeta := aggregator.GetMetadata()

		// Convert actions to strings
		actions := make([]string, len(healingMeta.Actions))
		for i, action := range healingMeta.Actions {
			actions[i] = action.Description
		}
		nf.Superbrain.AttemptedActions = actions

		// Check if provider was changed (fallback occurred)
		if healingMeta.OriginalProvider != healingMeta.FinalProvider {
			nf.Superbrain.FallbacksTried = []string{healingMeta.FinalProvider}
		}
	}

	// Set diagnosis summary
	if diagnosis != nil {
		nf.Superbrain.DiagnosisSummary = diagnosis.RootCause
		nf.Error.Type = string(diagnosis.FailureType)
	}

	// Add suggestions
	nf.Superbrain.Suggestions = e.generateSuggestions(diagnosis)

	return nf
}

// generateSuggestions generates helpful suggestions based on the diagnosis.
func (e *Enricher) generateSuggestions(diagnosis *types.Diagnosis) []string {
	suggestions := []string{}

	if diagnosis != nil {
		switch diagnosis.FailureType {
		case types.FailureTypePermissionPrompt:
			suggestions = append(suggestions, "The CLI tool is waiting for permission. Consider using auto-approve flags.")
			suggestions = append(suggestions, "Enable autopilot mode in Superbrain configuration for automatic permission handling.")

		case types.FailureTypeAuthError:
			suggestions = append(suggestions, "Check your API credentials and ensure they are valid.")
			suggestions = append(suggestions, "Try re-authenticating with the provider.")

		case types.FailureTypeContextExceeded:
			suggestions = append(suggestions, "Reduce the size of your request content.")
			suggestions = append(suggestions, "Use a model with a larger context window.")
			suggestions = append(suggestions, "Enable Context Sculptor to automatically optimize content.")

		case types.FailureTypeRateLimit:
			suggestions = append(suggestions, "Wait a few minutes before retrying.")
			suggestions = append(suggestions, "Consider using a different provider or API key.")

		case types.FailureTypeNetworkError:
			suggestions = append(suggestions, "Check your network connection.")
			suggestions = append(suggestions, "Verify the provider's service status.")

		case types.FailureTypeProcessCrash:
			suggestions = append(suggestions, "The CLI process crashed unexpectedly.")
			suggestions = append(suggestions, "Check the CLI tool's logs for more information.")

		default:
			suggestions = append(suggestions, "Try the request again.")
			suggestions = append(suggestions, "Check the provider's documentation for troubleshooting.")
		}
	} else {
		suggestions = append(suggestions, "Try the request again.")
		suggestions = append(suggestions, "Check your configuration and credentials.")
	}

	return suggestions
}

// NegotiatedFailureToJSON converts a NegotiatedFailureResponse to JSON bytes.
func NegotiatedFailureToJSON(nf *types.NegotiatedFailureResponse) ([]byte, error) {
	return json.Marshal(nf)
}

// NegotiatedFailureToResponse converts a NegotiatedFailureResponse to an executor Response.
func NegotiatedFailureToResponse(nf *types.NegotiatedFailureResponse) (switchailocalexecutor.Response, error) {
	payload, err := NegotiatedFailureToJSON(nf)
	if err != nil {
		return switchailocalexecutor.Response{}, err
	}
	return switchailocalexecutor.Response{Payload: payload}, nil
}

// StreamChunkEnricher provides enrichment for streaming responses.
type StreamChunkEnricher struct {
	aggregator *metadata.Aggregator
	enricher   *Enricher
	isLast     bool
}

// NewStreamChunkEnricher creates a new stream chunk enricher.
func NewStreamChunkEnricher(aggregator *metadata.Aggregator) *StreamChunkEnricher {
	return &StreamChunkEnricher{
		aggregator: aggregator,
		enricher:   NewEnricher(),
		isLast:     false,
	}
}

// EnrichChunk enriches a stream chunk with metadata if it's the last chunk.
// For streaming responses, metadata is typically only added to the final chunk.
func (sce *StreamChunkEnricher) EnrichChunk(chunk switchailocalexecutor.StreamChunk, isLast bool) switchailocalexecutor.StreamChunk {
	if !isLast || sce.aggregator == nil || !sce.aggregator.HasActions() {
		return chunk
	}

	// Try to parse the chunk payload as JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(chunk.Payload, &payload); err != nil {
		return chunk
	}

	// Get the healing metadata
	healingMeta := sce.aggregator.GetMetadata()

	// Add superbrain extension
	payload["superbrain"] = healingMeta.ToOpenAIExtension()["superbrain"]

	// Re-marshal the payload
	enrichedPayload, err := json.Marshal(payload)
	if err != nil {
		return chunk
	}

	chunk.Payload = enrichedPayload
	return chunk
}

// WrapStreamChannel wraps a stream channel to add metadata to the final chunk.
func (sce *StreamChunkEnricher) WrapStreamChannel(input <-chan switchailocalexecutor.StreamChunk) <-chan switchailocalexecutor.StreamChunk {
	output := make(chan switchailocalexecutor.StreamChunk)

	go func() {
		defer close(output)

		var lastChunk *switchailocalexecutor.StreamChunk

		for chunk := range input {
			// If we have a previous chunk, send it (it's not the last)
			if lastChunk != nil {
				output <- *lastChunk
			}
			// Store current chunk as potentially the last
			chunkCopy := chunk
			lastChunk = &chunkCopy
		}

		// Send the last chunk with enrichment
		if lastChunk != nil {
			enriched := sce.EnrichChunk(*lastChunk, true)
			output <- enriched
		}
	}()

	return output
}

// BuildOpenAIErrorResponse creates an OpenAI-compatible error response with Superbrain metadata.
func BuildOpenAIErrorResponse(err error, aggregator *metadata.Aggregator, diagnosis *types.Diagnosis) ([]byte, error) {
	enricher := NewEnricher()
	nf := enricher.CreateNegotiatedFailureResponse(err, aggregator, diagnosis)
	return NegotiatedFailureToJSON(nf)
}

// ExtractSuperbrainMetadata extracts Superbrain metadata from a response payload.
func ExtractSuperbrainMetadata(payload []byte) (map[string]interface{}, bool) {
	var response map[string]interface{}
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, false
	}

	superbrain, ok := response["superbrain"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	return superbrain, true
}

// WasHealed checks if a response indicates that healing occurred.
func WasHealed(payload []byte) bool {
	superbrain, ok := ExtractSuperbrainMetadata(payload)
	if !ok {
		return false
	}

	healed, ok := superbrain["healed"].(bool)
	return ok && healed
}

// GetHealingActions extracts the healing actions from a response payload.
func GetHealingActions(payload []byte) []map[string]interface{} {
	superbrain, ok := ExtractSuperbrainMetadata(payload)
	if !ok {
		return nil
	}

	actions, ok := superbrain["healing_actions"].([]interface{})
	if !ok {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(actions))
	for _, action := range actions {
		if actionMap, ok := action.(map[string]interface{}); ok {
			result = append(result, actionMap)
		}
	}

	return result
}
