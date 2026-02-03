package hooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHandleLogWarning(t *testing.T) {
	hook := &Hook{
		ID:   "test-hook",
		Name: "Test Hook",
		Params: map[string]any{
			"message": "Test warning message",
		},
	}

	ctx := &EventContext{
		Event:     EventRequestFailed,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	err := handleLogWarning(hook, ctx)
	if err != nil {
		t.Fatalf("handleLogWarning failed: %v", err)
	}

	// Test with no message parameter
	hook.Params = map[string]any{}
	err = handleLogWarning(hook, ctx)
	if err != nil {
		t.Fatalf("handleLogWarning with no message failed: %v", err)
	}
}

func TestWebhookHandler_Success(t *testing.T) {
	// Create test server
	var receivedPayload map[string]interface{}
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		ID:   "webhook-hook",
		Name: "Webhook Test",
		Params: map[string]any{
			"url":    serverURL,
			"secret": "test-secret",
		},
	}

	ctx := &EventContext{
		Event:     EventProviderUnavailable,
		Timestamp: time.Now(),
		Provider:  "test-provider",
		Data:      map[string]interface{}{"error": "connection failed"},
	}

	err := handler.Handle(hook, ctx)
	if err != nil {
		t.Fatalf("Webhook handler failed: %v", err)
	}

	// Verify payload
	if receivedPayload["event"] != string(EventProviderUnavailable) {
		t.Errorf("Expected event %s, got %v", EventProviderUnavailable, receivedPayload["event"])
	}

	if receivedPayload["hook_id"] != "webhook-hook" {
		t.Errorf("Expected hook_id webhook-hook, got %v", receivedPayload["hook_id"])
	}

	// Verify headers
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Error("Expected Content-Type: application/json")
	}

	if receivedHeaders.Get("User-Agent") != "switchAILocal-Hooks/1.0" {
		t.Error("Expected User-Agent: switchAILocal-Hooks/1.0")
	}

	// Verify signature
	signature := receivedHeaders.Get("X-Hook-Signature")
	if signature == "" {
		t.Error("Expected X-Hook-Signature header")
	}

	if !strings.HasPrefix(signature, "sha256=") {
		t.Error("Expected signature to start with sha256=")
	}
}

func TestWebhookHandler_HTTPSValidation(t *testing.T) {
	handler := NewWebhookHandler()

	testCases := []struct {
		url         string
		shouldError bool
		description string
	}{
		{"http://example.com/webhook", true, "HTTP non-localhost should be rejected"},
		{"ftp://example.com/webhook", true, "Non-HTTP protocol should be rejected"},
		{"", true, "Empty URL should be rejected"},
	}

	for _, tc := range testCases {
		hook := &Hook{
			Params: map[string]any{"url": tc.url},
		}
		ctx := &EventContext{Event: EventRequestFailed}

		err := handler.Handle(hook, ctx)
		if tc.shouldError && err == nil {
			t.Errorf("%s: expected error but got none", tc.description)
		}
		if !tc.shouldError && err != nil {
			t.Errorf("%s: expected no error but got: %v", tc.description, err)
		}
	}
}

func TestWebhookHandler_RateLimit(t *testing.T) {
	// Create test server
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{"url": serverURL},
	}
	ctx := &EventContext{Event: EventRequestFailed}

	// Make 10 calls (should all succeed)
	for i := 0; i < 10; i++ {
		err := handler.Handle(hook, ctx)
		if err != nil {
			t.Fatalf("Call %d failed: %v", i+1, err)
		}
	}

	// 11th call should fail due to rate limit
	err := handler.Handle(hook, ctx)
	if err == nil {
		t.Error("Expected rate limit error on 11th call")
	}

	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("Expected rate limit error, got: %v", err)
	}

	// Verify server only received 10 calls
	if callCount != 10 {
		t.Errorf("Expected 10 server calls, got %d", callCount)
	}
}

func TestWebhookHandler_Retry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			// Succeed on 3rd attempt
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{"url": serverURL},
	}
	ctx := &EventContext{Event: EventRequestFailed}

	start := time.Now()
	err := handler.Handle(hook, ctx)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Webhook should succeed after retries: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls (2 retries), got %d", callCount)
	}

	// Should take at least 3 seconds (1s + 2s delays)
	if duration < 3*time.Second {
		t.Errorf("Expected at least 3s duration for retries, got %v", duration)
	}
}

func TestWebhookHandler_RetryFailure(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError) // Always fail
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{"url": serverURL},
	}
	ctx := &EventContext{Event: EventRequestFailed}

	err := handler.Handle(hook, ctx)
	if err == nil {
		t.Error("Expected webhook to fail after all retries")
	}

	if !strings.Contains(err.Error(), "webhook failed after retries") {
		t.Errorf("Expected retry failure error, got: %v", err)
	}

	// Should have made 4 attempts (initial + 3 retries)
	if callCount != 4 {
		t.Errorf("Expected 4 calls (initial + 3 retries), got %d", callCount)
	}
}

func TestWebhookHandler_Timeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // Longer than 5s timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{"url": serverURL},
	}
	ctx := &EventContext{Event: EventRequestFailed}

	start := time.Now()
	err := handler.Handle(hook, ctx)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error")
	}

	// Should timeout in about 5 seconds per attempt
	// With retries: ~5s + 1s + ~5s + 2s + ~5s + 4s = ~22s
	// Allow some variance for test execution
	if duration < 15*time.Second || duration > 35*time.Second {
		t.Errorf("Expected ~22s duration for timeout with retries (15-35s range), got %v", duration)
	}
}

func TestWebhookHandler_Signature(t *testing.T) {
	var receivedSignature string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Hook-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{
			"url":    serverURL,
			"secret": "test-secret-key",
		},
	}
	ctx := &EventContext{
		Event:     EventQuotaWarning,
		Timestamp: time.Now(),
	}

	err := handler.Handle(hook, ctx)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}

	// Verify signature
	if !strings.HasPrefix(receivedSignature, "sha256=") {
		t.Errorf("Expected signature to start with sha256=, got: %s", receivedSignature)
	}

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte("test-secret-key"))
	mac.Write(receivedBody)
	expectedSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if receivedSignature != expectedSignature {
		t.Errorf("Signature mismatch. Expected: %s, Got: %s", expectedSignature, receivedSignature)
	}
}

func TestWebhookHandler_ConcurrentRateLimit(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{"url": serverURL},
	}

	// Make 20 concurrent calls
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := &EventContext{Event: EventRequestFailed}
			err := handler.Handle(hook, ctx)
			errors <- err
		}()
	}

	wg.Wait()
	close(errors)

	successCount := 0
	rateLimitCount := 0

	for err := range errors {
		if err == nil {
			successCount++
		} else if strings.Contains(err.Error(), "rate limit exceeded") {
			rateLimitCount++
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 successes and 10 rate limit errors
	if successCount != 10 {
		t.Errorf("Expected 10 successful calls, got %d", successCount)
	}

	if rateLimitCount != 10 {
		t.Errorf("Expected 10 rate limited calls, got %d", rateLimitCount)
	}

	mu.Lock()
	actualCallCount := callCount
	mu.Unlock()

	if actualCallCount != 10 {
		t.Errorf("Expected 10 server calls, got %d", actualCallCount)
	}
}

func TestHandleRunCommand(t *testing.T) {
	testCases := []struct {
		command     string
		shouldError bool
		description string
	}{
		{"echo hello", false, "Allowed command should succeed"},
		{"logger test message", false, "Logger command should succeed"},
		{"rm -rf /", true, "Dangerous command should be blocked"},
		{"cat /etc/passwd", true, "Unauthorized command should be blocked"},
		{"", true, "Empty command should fail"},
		{"   ", true, "Whitespace command should fail"},
	}

	for _, tc := range testCases {
		hook := &Hook{
			Params: map[string]any{"command": tc.command},
		}
		ctx := &EventContext{Event: EventRequestFailed}

		err := handleRunCommand(hook, ctx)
		if tc.shouldError && err == nil {
			t.Errorf("%s: expected error but got none", tc.description)
		}
		if !tc.shouldError && err != nil {
			t.Errorf("%s: expected no error but got: %v", tc.description, err)
		}
	}
}

func TestHandleRunCommand_MissingCommand(t *testing.T) {
	hook := &Hook{
		Params: map[string]any{}, // No command parameter
	}
	ctx := &EventContext{Event: EventRequestFailed}

	err := handleRunCommand(hook, ctx)
	if err == nil {
		t.Error("Expected error for missing command")
	}

	if !strings.Contains(err.Error(), "missing command") {
		t.Errorf("Expected 'missing command' error, got: %v", err)
	}
}

func TestHandleRetryWithFallback(t *testing.T) {
	hook := &Hook{
		Name: "Retry Hook",
	}
	ctx := &EventContext{
		Event:    EventRequestFailed,
		Provider: "failed-provider",
	}

	// Should not error (just logs for now)
	err := handleRetryWithFallback(hook, ctx)
	if err != nil {
		t.Fatalf("handleRetryWithFallback failed: %v", err)
	}
}

func TestHandlePreferProvider(t *testing.T) {
	hook := &Hook{
		Params: map[string]any{
			"provider": "preferred-provider",
		},
	}
	ctx := &EventContext{Event: EventRequestFailed}

	err := handlePreferProvider(hook, ctx)
	if err != nil {
		t.Fatalf("handlePreferProvider failed: %v", err)
	}

	// Test missing provider parameter
	hook.Params = map[string]any{}
	err = handlePreferProvider(hook, ctx)
	if err == nil {
		t.Error("Expected error for missing provider parameter")
	}
}

func TestHandleRotateCredential(t *testing.T) {
	hook := &Hook{
		Params: map[string]any{
			"provider": "test-provider",
		},
	}
	ctx := &EventContext{Event: EventProviderUnavailable}

	// Should not error (just logs for now)
	err := handleRotateCredential(hook, ctx)
	if err != nil {
		t.Fatalf("handleRotateCredential failed: %v", err)
	}
}

func TestHandleRestartProvider(t *testing.T) {
	hook := &Hook{
		Params: map[string]any{
			"provider": "test-provider",
		},
	}
	ctx := &EventContext{Event: EventProviderUnavailable}

	// Should not error (just logs for now)
	err := handleRestartProvider(hook, ctx)
	if err != nil {
		t.Fatalf("handleRestartProvider failed: %v", err)
	}
}

func TestRegisterBuiltInActions(t *testing.T) {
	// Create a hook manager
	manager, err := NewHookManager("", NewEventBus())
	if err != nil {
		t.Fatalf("Failed to create hook manager: %v", err)
	}

	// Register built-in actions
	RegisterBuiltInActions(manager)

	// Verify all actions are registered
	expectedActions := []HookAction{
		ActionLogWarning,
		ActionNotifyWebhook,
		ActionRunCommand,
		ActionRetryWithFallback,
		ActionPreferProvider,
		ActionRotateCredential,
		ActionRestartProvider,
	}

	for _, action := range expectedActions {
		// Try to execute each action to verify it's registered
		hook := &Hook{
			Action: action,
			Params: map[string]any{},
		}
		ctx := &EventContext{Event: EventRequestFailed}

		// This will call the registered handler
		manager.executeAction(hook, ctx)
		// If we get here without panic, the action is registered
	}
}

// --- Benchmark Tests ---

func BenchmarkWebhookHandler_Handle(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Convert server URL to localhost format
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	handler := NewWebhookHandler()
	hook := &Hook{
		Params: map[string]any{"url": serverURL},
	}
	ctx := &EventContext{
		Event:     EventRequestFailed,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset rate limiter for each iteration
		handler.rateLimiters = make(map[string]*rateLimiter)
		_ = handler.Handle(hook, ctx)
	}
}

func BenchmarkHandleLogWarning(b *testing.B) {
	hook := &Hook{
		Name: "Test Hook",
		Params: map[string]any{
			"message": "Test warning message",
		},
	}
	ctx := &EventContext{
		Event:     EventRequestFailed,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handleLogWarning(hook, ctx)
	}
}

func BenchmarkHandleRunCommand(b *testing.B) {
	hook := &Hook{
		Params: map[string]any{
			"command": "echo test",
		},
	}
	ctx := &EventContext{Event: EventRequestFailed}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handleRunCommand(hook, ctx)
	}
}
