package executor

import (
	"net/http"
	"testing"
)

func TestDetectSSEErrorEvent_MiniMaxError(t *testing.T) {
	line := []byte(`data: {"type":"error","error":{"type":"server_error","message":"unknown error, 798 (1000)","http_code":"500"},"request_id":"061950ae53e979dcdb91b46926cc4e0c"}`)
	err := detectSSEErrorEvent(line)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se, ok := err.(statusErr)
	if !ok {
		t.Fatalf("expected statusErr, got %T", err)
	}
	if se.code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", se.code)
	}
	if se.msg != "unknown error, 798 (1000)" {
		t.Errorf("unexpected message: %s", se.msg)
	}
	t.Logf("✓ MiniMax error detected: code=%d msg=%q", se.code, se.msg)
}

func TestDetectSSEErrorEvent_NormalChunk(t *testing.T) {
	line := []byte(`data: {"id":"abc","choices":[{"index":0,"delta":{"content":"hello"}}],"model":"MiniMax-M2.7","object":"chat.completion.chunk"}`)
	err := detectSSEErrorEvent(line)
	if err != nil {
		t.Fatalf("expected nil for normal chunk, got: %v", err)
	}
	t.Log("✓ Normal chunk correctly ignored")
}

func TestDetectSSEErrorEvent_DoneMarker(t *testing.T) {
	line := []byte("data: [DONE]")
	err := detectSSEErrorEvent(line)
	if err != nil {
		t.Fatalf("expected nil for [DONE], got: %v", err)
	}
	t.Log("✓ [DONE] marker correctly ignored")
}

func TestDetectSSEErrorEvent_NonDataLine(t *testing.T) {
	line := []byte("event: message")
	err := detectSSEErrorEvent(line)
	if err != nil {
		t.Fatalf("expected nil for non-data line, got: %v", err)
	}
	t.Log("✓ Non-data line correctly ignored")
}

func TestDetectSSEErrorEvent_OpenAIStyleError(t *testing.T) {
	line := []byte(`data: {"error":{"message":"Rate limit exceeded","type":"rate_limit_error","status":429}}`)
	err := detectSSEErrorEvent(line)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se, ok := err.(statusErr)
	if !ok {
		t.Fatalf("expected statusErr, got %T", err)
	}
	if se.code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", se.code)
	}
	t.Logf("✓ OpenAI-style error detected: code=%d msg=%q", se.code, se.msg)
}

func TestDetectSSEErrorEvent_AnthropicOverloaded(t *testing.T) {
	line := []byte(`data: {"type":"overloaded_error","message":"Overloaded"}`)
	err := detectSSEErrorEvent(line)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se, ok := err.(statusErr)
	if !ok {
		t.Fatalf("expected statusErr, got %T", err)
	}
	if se.code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", se.code)
	}
	if se.msg != "Overloaded" {
		t.Errorf("unexpected message: %s", se.msg)
	}
	t.Logf("✓ Anthropic overloaded detected: code=%d msg=%q", se.code, se.msg)
}

func TestDetectSSEErrorEvent_AnthropicRateLimit(t *testing.T) {
	line := []byte(`data: {"type":"rate_limit_error","message":"Rate limit reached"}`)
	err := detectSSEErrorEvent(line)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se := err.(statusErr)
	if se.code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", se.code)
	}
	t.Logf("✓ Anthropic rate_limit_error detected: code=%d msg=%q", se.code, se.msg)
}

func TestDetectSSEErrorEvent_DetailFormat(t *testing.T) {
	line := []byte(`data: {"detail":"Rate limit exceeded for model","status_code":429}`)
	err := detectSSEErrorEvent(line)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se := err.(statusErr)
	if se.code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", se.code)
	}
	t.Logf("✓ detail+status_code format detected: code=%d msg=%q", se.code, se.msg)
}

func TestDetectSSEErrorEvent_ErrorCodeAsInt(t *testing.T) {
	line := []byte(`data: {"error":{"message":"Too many requests","type":"rate_limit","code":429}}`)
	err := detectSSEErrorEvent(line)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se := err.(statusErr)
	if se.code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", se.code)
	}
	t.Logf("✓ error.code as int detected: code=%d msg=%q", se.code, se.msg)
}
