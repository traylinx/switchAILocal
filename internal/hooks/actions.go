package hooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// RegisterBuiltInActions registers the default action handlers.
func RegisterBuiltInActions(m *HookManager) {
	m.RegisterAction(ActionLogWarning, handleLogWarning)
	// Use stateful webhook handler
	wh := NewWebhookHandler()
	m.RegisterAction(ActionNotifyWebhook, wh.Handle)
	m.RegisterAction(ActionRunCommand, handleRunCommand)
	m.RegisterAction(ActionRetryWithFallback, handleRetryWithFallback)
	m.RegisterAction(ActionPreferProvider, handlePreferProvider)
	m.RegisterAction(ActionRotateCredential, handleRotateCredential)
	m.RegisterAction(ActionRestartProvider, handleRestartProvider)
}

func handleLogWarning(hook *Hook, ctx *EventContext) error {
	msg, _ := hook.Params["message"].(string)
	if msg == "" {
		msg = "Hook triggered"
	}
	log.Warnf("[Hook: %s] %s (Event: %s)", hook.Name, msg, ctx.Event)
	return nil
}

// WebhookHandler manages webhook execution with rate limiting.
type WebhookHandler struct {
	mu           sync.RWMutex
	rateLimiters map[string]*rateLimiter
}

type rateLimiter struct {
	count    int
	lastTime time.Time
}

func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		rateLimiters: make(map[string]*rateLimiter),
	}
}

func (h *WebhookHandler) Handle(hook *Hook, ctx *EventContext) error {
	url, _ := hook.Params["url"].(string)
	if url == "" {
		return fmt.Errorf("missing webhook url")
	}

	// HTTPS Validation
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://localhost") {
		return fmt.Errorf("insecure webhook url (must be https or localhost): %s", url)
	}

	// Rate Limiting (10 per minute per URL)
	if !h.checkRateLimit(url) {
		return fmt.Errorf("rate limit exceeded for webhook: %s", url)
	}

	secret, _ := hook.Params["secret"].(string)

	payload := map[string]interface{}{
		"event":     ctx.Event,
		"timestamp": ctx.Timestamp,
		"hook_id":   hook.ID,
		"data":      ctx.Data,
	}

	if ctx.Provider != "" {
		payload["provider"] = ctx.Provider
	}
	if ctx.Model != "" {
		payload["model"] = ctx.Model
	}
	if ctx.Request != nil {
		// Try to find intent in Data map as Request doesn't hold it directly
		if intent, ok := ctx.Data["intent"].(string); ok {
			payload["request_intent"] = intent
		}
		payload["selected_model"] = ctx.Model
	}
	if ctx.ErrorMessage != "" {
		payload["error"] = ctx.ErrorMessage
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Retry Logic (3 attempts: 1s, 2s, 4s)
	backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error

	for i := 0; i <= len(backoff); i++ {
		if i > 0 {
			time.Sleep(backoff[i-1])
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "switchAILocal-Hooks/1.0")

		if secret != "" {
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(body)
			signature := hex.EncodeToString(mac.Sum(nil))
			req.Header.Set("X-Hook-Signature", "sha256="+signature)
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)

		if err != nil {
			lastErr = err
			log.Warnf("Webhook attempt %d failed: %v", i+1, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			log.Warnf("Webhook attempt %d failed with status: %d", i+1, resp.StatusCode)
			continue
		}

		// Success
		return nil
	}

	return fmt.Errorf("webhook failed after retries: %v", lastErr)
}

func (h *WebhookHandler) checkRateLimit(url string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Limiter logic: 10 calls per minute
	now := time.Now()
	limiter, exists := h.rateLimiters[url]
	if !exists {
		limiter = &rateLimiter{count: 0, lastTime: now}
		h.rateLimiters[url] = limiter
	}

	if now.Sub(limiter.lastTime) > time.Minute {
		limiter.count = 0
		limiter.lastTime = now
	}

	if limiter.count >= 10 {
		return false
	}

	limiter.count++
	return true
}

func handleRunCommand(hook *Hook, ctx *EventContext) error {
	cmdStr, _ := hook.Params["command"].(string)
	if cmdStr == "" {
		return fmt.Errorf("missing command")
	}

	// Basic security check: Only allow specific commands or paths
	// In a real scenario, this should be strictly whitelisted
	allowedCommands := []string{"echo", "logger", "notify-send"}
	cmdParts := strings.Fields(cmdStr)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty command")
	}

	isAllowed := false
	for _, allowed := range allowedCommands {
		if cmdParts[0] == allowed {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return fmt.Errorf("command '%s' is not in the whitelist", cmdParts[0])
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("command failed: %v, output: %s", err, string(out))
	}

	return nil
}

func handleRetryWithFallback(hook *Hook, ctx *EventContext) error {
	// Requires integration with Router/Service to actually retry.
	// For now, we update the event context to signal a retry desire?
	// Or we can modify the request context if we had access to it pointer.
	// But Context in EventContext is a snapshot usually.
	// We might need a channel or callback in EventContext if we want immediate reaction.
	// "Retry" usually implies re-dispatching.
	// We'll log intention for now.
	log.Infof("[Hook] Requesting retry with fallback for request")
	return nil
}

func handlePreferProvider(hook *Hook, ctx *EventContext) error {
	provider, _ := hook.Params["provider"].(string)
	if provider == "" {
		return fmt.Errorf("missing provider param")
	}
	log.Infof("[Hook] Adjusting preferences to favor provider: %s", provider)
	// Integration point: Update Semantic Memory or Quirk Store
	return nil
}

func handleRotateCredential(hook *Hook, ctx *EventContext) error {
	provider, _ := hook.Params["provider"].(string)
	log.Infof("[Hook] Rotating credentials for provider: %s", provider)
	// Integration point: Call AuthManager
	return nil
}

func handleRestartProvider(hook *Hook, ctx *EventContext) error {
	provider, _ := hook.Params["provider"].(string)
	log.Infof("[Hook] Signaling restart for provider: %s", provider)
	// Integration point: Call Service Manager
	return nil
}
