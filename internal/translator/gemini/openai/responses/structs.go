package responses

// ResponseCreated represents the "response.created" event.
type ResponseCreated struct {
	Type           string             `json:"type"`
	SequenceNumber int                `json:"sequence_number"`
	Response       ResponseInfo       `json:"response"`
}

// ResponseInProgress represents the "response.in_progress" event.
type ResponseInProgress struct {
	Type           string             `json:"type"`
	SequenceNumber int                `json:"sequence_number"`
	Response       ResponseInfo       `json:"response"`
}

// ResponseInfo used in created, in_progress, and completed.
type ResponseInfo struct {
	ID                  string               `json:"id"`
	Object              string               `json:"object"`
	CreatedAt           int64                `json:"created_at"`
	Status              string               `json:"status"`
	Background          bool                 `json:"background"` // Removed omitempty
	Error               any                  `json:"error"`      // Removed omitempty
	Output              *[]any               `json:"output,omitempty"` // Changed to pointer and added omitempty
	Instructions        string               `json:"instructions,omitempty"`
	MaxOutputTokens     int64                `json:"max_output_tokens,omitempty"`
	MaxToolCalls        int64                `json:"max_tool_calls,omitempty"`
	Model               string               `json:"model,omitempty"`
	ParallelToolCalls   *bool                `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID  string               `json:"previous_response_id,omitempty"`
	PromptCacheKey      string               `json:"prompt_cache_key,omitempty"`
	Reasoning           any                  `json:"reasoning,omitempty"`
	SafetyIdentifier    string               `json:"safety_identifier,omitempty"`
	ServiceTier         string               `json:"service_tier,omitempty"`
	Store               *bool                `json:"store,omitempty"`
	Temperature         *float64             `json:"temperature,omitempty"`
	Text                any                  `json:"text,omitempty"`
	ToolChoice          any                  `json:"tool_choice,omitempty"`
	Tools               any                  `json:"tools,omitempty"`
	TopLogprobs         int64                `json:"top_logprobs,omitempty"`
	TopP                *float64             `json:"top_p,omitempty"`
	Truncation          string               `json:"truncation,omitempty"`
	User                any                  `json:"user,omitempty"`
	Metadata            any                  `json:"metadata,omitempty"`
	Usage               *ResponseUsage       `json:"usage,omitempty"`
}

type ResponseUsage struct {
	InputTokens        int64              `json:"input_tokens"`
	OutputTokens       int64              `json:"output_tokens"`
	TotalTokens        int64              `json:"total_tokens"`
	InputTokensDetails *InputTokensDetails `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
}

type InputTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type OutputTokensDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

// OutputItemAdded represents the "response.output_item.added" event.
type OutputItemAdded struct {
	Type           string     `json:"type"`
	SequenceNumber int        `json:"sequence_number"`
	OutputIndex    int        `json:"output_index"`
	Item           OutputItem `json:"item"`
}

type OutputItem struct {
	ID               string        `json:"id"`
	Type             string        `json:"type"`
	Status           string        `json:"status,omitempty"`
	Content          []ContentPart `json:"content,omitempty"`
	Role             string        `json:"role,omitempty"`
	Arguments        string        `json:"arguments,omitempty"`
	CallID           string        `json:"call_id,omitempty"`
	Name             string        `json:"name,omitempty"`
	Summary          []SummaryPart `json:"summary,omitempty"`
	EncryptedContent string        `json:"encrypted_content,omitempty"`
}

type SummaryPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ContentPartAdded represents the "response.content_part.added" event.
type ContentPartAdded struct {
	Type           string      `json:"type"`
	SequenceNumber int         `json:"sequence_number"`
	ItemID         string      `json:"item_id"`
	OutputIndex    int         `json:"output_index"`
	ContentIndex   int         `json:"content_index"`
	Part           ContentPart `json:"part"`
}

type ContentPart struct {
	Type        string `json:"type"`
	Annotations []any  `json:"annotations"` // Removed omitempty
	Logprobs    []any  `json:"logprobs"`    // Removed omitempty
	Text        string `json:"text"`
}

// OutputTextDelta represents the "response.output_text.delta" event.
type OutputTextDelta struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	ContentIndex   int    `json:"content_index"`
	Delta          string `json:"delta"`
	Logprobs       []any  `json:"logprobs"` // Removed omitempty
}

// FunctionCallArgumentsDelta represents the "response.function_call_arguments.delta" event.
type FunctionCallArgumentsDelta struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Delta          string `json:"delta"`
}

// ReasoningSummaryTextDelta represents the "response.reasoning_summary_text.delta" event.
type ReasoningSummaryTextDelta struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	SummaryIndex   int    `json:"summary_index"`
	Delta          string `json:"delta"`
}

// ReasoningSummaryPartAdded represents the "response.reasoning_summary_part.added" event.
type ReasoningSummaryPartAdded struct {
	Type           string      `json:"type"`
	SequenceNumber int         `json:"sequence_number"`
	ItemID         string      `json:"item_id"`
	OutputIndex    int         `json:"output_index"`
	SummaryIndex   int         `json:"summary_index"`
	Part           SummaryPart `json:"part"`
}

// ResponseReasoningSummaryTextDone represents the "response.reasoning_summary_text.done" event.
type ResponseReasoningSummaryTextDone struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	SummaryIndex   int    `json:"summary_index"`
	Text           string `json:"text"`
}

// ResponseReasoningSummaryPartDone represents the "response.reasoning_summary_part.done" event.
type ResponseReasoningSummaryPartDone struct {
	Type           string      `json:"type"`
	SequenceNumber int         `json:"sequence_number"`
	ItemID         string      `json:"item_id"`
	OutputIndex    int         `json:"output_index"`
	SummaryIndex   int         `json:"summary_index"`
	Part           SummaryPart `json:"part"`
}

// ResponseOutputItemDone represents the "response.output_item.done" event.
type ResponseOutputItemDone struct {
	Type           string     `json:"type"`
	SequenceNumber int        `json:"sequence_number"`
	OutputIndex    int        `json:"output_index"`
	Item           OutputItem `json:"item"`
}

// ResponseOutputTextDone represents the "response.output_text.done" event.
type ResponseOutputTextDone struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	ContentIndex   int    `json:"content_index"`
	Text           string `json:"text"`
	Logprobs       []any  `json:"logprobs"` // Removed omitempty
}

// ResponseContentPartDone represents the "response.content_part.done" event.
type ResponseContentPartDone struct {
	Type           string      `json:"type"`
	SequenceNumber int         `json:"sequence_number"`
	ItemID         string      `json:"item_id"`
	OutputIndex    int         `json:"output_index"`
	ContentIndex   int         `json:"content_index"`
	Part           ContentPart `json:"part"`
}

// ResponseFunctionCallArgumentsDone represents the "response.function_call_arguments.done" event.
type ResponseFunctionCallArgumentsDone struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Arguments      string `json:"arguments"`
}

// ResponseCompleted represents the "response.completed" event.
type ResponseCompleted struct {
	Type           string       `json:"type"`
	SequenceNumber int          `json:"sequence_number"`
	Response       ResponseInfo `json:"response"`
}
