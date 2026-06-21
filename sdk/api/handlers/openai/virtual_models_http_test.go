package openai

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	internalconfig "github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/registry"
	"github.com/traylinx/switchAILocal/internal/virtualmodels"
	basehandlers "github.com/traylinx/switchAILocal/sdk/api/handlers"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	coreexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

type recordingExecutor struct {
	provider string
	mu       sync.Mutex
	models   []string
}

func (e *recordingExecutor) Identifier() string { return e.provider }

func (e *recordingExecutor) Execute(_ context.Context, _ *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (coreexecutor.Response, error) {
	e.mu.Lock()
	e.models = append(e.models, req.Model)
	e.mu.Unlock()

	if opts.SourceFormat.String() == "openai-response" {
		return coreexecutor.Response{Payload: []byte(fmt.Sprintf(`{"id":"resp-test","object":"response","model":%q,"output_text":"ok from %s"}`, req.Model, e.provider))}, nil
	}
	return coreexecutor.Response{Payload: []byte(fmt.Sprintf(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"ok from %s"},"finish_reason":"stop"}]}`, req.Model, e.provider))}, nil
}

func (e *recordingExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (<-chan coreexecutor.StreamChunk, error) {
	ch := make(chan coreexecutor.StreamChunk)
	close(ch)
	return ch, nil
}

func (e *recordingExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *recordingExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}

func (e *recordingExecutor) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.models)
}

func setupVirtualHTTPTest(t *testing.T, cfg *sdkconfig.SDKConfig) (*gin.Engine, map[string]*recordingExecutor) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	manager := coreauth.NewManager(nil, nil, nil)
	executors := map[string]*recordingExecutor{}
	register := func(provider, authID, model string) {
		exec := &recordingExecutor{provider: provider}
		executors[provider] = exec
		manager.RegisterExecutor(exec)
		_, err := manager.Register(context.Background(), &coreauth.Auth{
			ID:       authID,
			Provider: provider,
			Status:   coreauth.StatusActive,
			Metadata: map[string]any{"source": "test"},
		})
		if err != nil {
			t.Fatalf("register auth %s: %v", provider, err)
		}
		registry.GetGlobalRegistry().RegisterClient(authID, provider, []*registry.ModelInfo{{ID: model, Object: "model", OwnedBy: provider}})
		t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(authID) })
	}
	register("provider-a", "virtual-http-auth-a", "native-a")
	register("provider-b", "virtual-http-auth-b", "native-b")
	register("provider-agentic", "virtual-http-auth-agentic", "native-agentic")

	base := basehandlers.NewBaseAPIHandlers(cfg, manager, nil, nil, nil, nil)
	base.VirtualRouter = virtualmodels.NewRouterWithStatePath(filepath.Join(t.TempDir(), "virtual_models_state.json"))
	openaiHandler := NewOpenAIAPIHandler(base)
	responsesHandler := NewOpenAIResponsesAPIHandler(base)

	r := gin.New()
	v1 := r.Group("/v1")
	v1.GET("/models", openaiHandler.OpenAIModels)
	v1.POST("/chat/completions", openaiHandler.ChatCompletions)
	v1.POST("/completions", openaiHandler.Completions)
	v1.POST("/responses", responsesHandler.Responses)
	return r, executors
}

func virtualHTTPWeightedConfig() *sdkconfig.SDKConfig {
	return virtualHTTPConfigWithMembers([]internalconfig.VirtualModelMemberConfig{
		{ID: "backend-a", Provider: "provider-a", Model: "native-a", Weight: 3, Enabled: boolPtr(true), Capabilities: textHTTPTestCaps()},
		{ID: "backend-b", Provider: "provider-b", Model: "native-b", Weight: 2, Enabled: boolPtr(true), Capabilities: textHTTPTestCaps()},
	})
}

func virtualHTTPAgenticConfig() *sdkconfig.SDKConfig {
	return virtualHTTPConfigWithMembers([]internalconfig.VirtualModelMemberConfig{
		{ID: "backend-a", Provider: "provider-a", Model: "native-a", Weight: 3, Enabled: boolPtr(true), Capabilities: textHTTPTestCaps()},
		{ID: "backend-b", Provider: "provider-b", Model: "native-b", Weight: 2, Enabled: boolPtr(true), Capabilities: textHTTPTestCaps()},
		{ID: "agentic", Provider: "provider-agentic", Model: "native-agentic", Weight: 1, Enabled: boolPtr(true), Capabilities: internalconfig.VirtualModelCapabilitiesConfig{Operations: []string{"chat_multiturn_tools"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true, ToolHistoryReplay: true}},
	})
}

func virtualHTTPConfigWithMembers(members []internalconfig.VirtualModelMemberConfig) *sdkconfig.SDKConfig {
	return &sdkconfig.SDKConfig{VirtualModels: map[string]internalconfig.VirtualModelConfig{
		"ail-compound": {
			Expose:        true,
			Strategy:      "weighted-round-robin",
			Fallback:      true,
			ResponseModel: "requested",
			Members:       members,
		},
	}}
}

func textHTTPTestCaps() internalconfig.VirtualModelCapabilitiesConfig {
	return internalconfig.VirtualModelCapabilitiesConfig{Operations: []string{"chat", "chat_text_tools"}, Input: []string{"text"}, Output: []string{"text"}, Tools: true}
}

func boolPtr(v bool) *bool { return &v }

func doJSON(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s status=%d body=%s", method, path, w.Code, w.Body.String())
	}
	return w
}

func TestVirtualModelHTTPWeightedRoutesAndPreservesEndpointShapes(t *testing.T) {
	r, executors := setupVirtualHTTPTest(t, virtualHTTPWeightedConfig())

	chatBody := `{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`
	for i := 0; i < 10; i++ {
		w := doJSON(t, r, http.MethodPost, "/v1/chat/completions", chatBody)
		if !bytes.Contains(w.Body.Bytes(), []byte(`"model":"ail-compound"`)) {
			t.Fatalf("chat response did not rewrite public model: %s", w.Body.String())
		}
	}
	if executors["provider-a"].Count() != 6 || executors["provider-b"].Count() != 4 {
		t.Fatalf("expected HTTP chat 6/4 routing, got provider-a=%d provider-b=%d", executors["provider-a"].Count(), executors["provider-b"].Count())
	}

	doJSON(t, r, http.MethodPost, "/v1/completions", `{"model":"ail-compound","prompt":"hello"}`)
	doJSON(t, r, http.MethodPost, "/v1/responses", `{"model":"ail-compound","input":"hello"}`)
}

func TestVirtualModelHTTPModelsExposeOnlyPublicAlias(t *testing.T) {
	r, _ := setupVirtualHTTPTest(t, virtualHTTPWeightedConfig())
	w := doJSON(t, r, http.MethodGet, "/v1/models", ``)
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"id":"ail-compound"`)) {
		t.Fatalf("/models missing public virtual alias: %s", body)
	}
	if bytes.Contains([]byte(body), []byte("native-a")) || bytes.Contains([]byte(body), []byte("native-b")) {
		t.Fatalf("/models exposed private backend aliases: %s", body)
	}
}

func TestVirtualModelHTTPToolHistoryOnlyUsesReplaySafeMember(t *testing.T) {
	r, executors := setupVirtualHTTPTest(t, virtualHTTPAgenticConfig())
	toolHistoryBody := `{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"}]}`
	for i := 0; i < 5; i++ {
		doJSON(t, r, http.MethodPost, "/v1/chat/completions", toolHistoryBody)
	}
	responsesToolHistoryBody := `{"model":"ail-compound","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`
	for i := 0; i < 5; i++ {
		doJSON(t, r, http.MethodPost, "/v1/responses", responsesToolHistoryBody)
	}
	if executors["provider-agentic"].Count() != 10 {
		t.Fatalf("expected replay-safe provider to receive all tool-history requests, got %d", executors["provider-agentic"].Count())
	}
	if executors["provider-a"].Count() != 0 || executors["provider-b"].Count() != 0 {
		t.Fatalf("text-only providers received tool-history traffic: provider-a=%d provider-b=%d", executors["provider-a"].Count(), executors["provider-b"].Count())
	}
}

func TestVirtualModelHTTPResponsesImageDoesNotFallThroughToTextOnly(t *testing.T) {
	r, executors := setupVirtualHTTPTest(t, virtualHTTPWeightedConfig())
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"ail-compound","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected image Responses request with no vision member to fail closed with 422, got %d body=%s", w.Code, w.Body.String())
	}
	if executors["provider-a"].Count() != 0 || executors["provider-b"].Count() != 0 {
		t.Fatalf("image Responses request fell through to text providers: provider-a=%d provider-b=%d", executors["provider-a"].Count(), executors["provider-b"].Count())
	}
}

func virtualHTTPVisionAgenticConfig() *sdkconfig.SDKConfig {
	return virtualHTTPConfigWithMembers([]internalconfig.VirtualModelMemberConfig{
		{ID: "unsafe-vision", Provider: "provider-a", Model: "native-a", Weight: 1, Enabled: boolPtr(true), Capabilities: internalconfig.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}, Tools: true}},
		{ID: "safe-vision", Provider: "provider-agentic", Model: "native-agentic", Weight: 1, Enabled: boolPtr(true), Capabilities: internalconfig.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}, Tools: true, ToolHistoryReplay: true}},
	})
}

func TestVirtualModelHTTPMixedMediaToolHistoryRequiresReplaySafeMember(t *testing.T) {
	r, executors := setupVirtualHTTPTest(t, virtualHTTPVisionAgenticConfig())
	chatMixed := `{"model":"ail-compound","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`
	responsesMixed := `{"model":"ail-compound","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"},{"role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`
	for i := 0; i < 3; i++ {
		doJSON(t, r, http.MethodPost, "/v1/chat/completions", chatMixed)
		doJSON(t, r, http.MethodPost, "/v1/responses", responsesMixed)
	}
	if executors["provider-agentic"].Count() != 6 {
		t.Fatalf("expected replay-safe vision provider to receive all mixed media/tool-history requests, got %d", executors["provider-agentic"].Count())
	}
	if executors["provider-a"].Count() != 0 || executors["provider-b"].Count() != 0 {
		t.Fatalf("unsafe/text providers received mixed media/tool-history traffic: provider-a=%d provider-b=%d", executors["provider-a"].Count(), executors["provider-b"].Count())
	}
}

func virtualHTTPSplitMediaConfig() *sdkconfig.SDKConfig {
	return virtualHTTPConfigWithMembers([]internalconfig.VirtualModelMemberConfig{
		{ID: "vision-only", Provider: "provider-a", Model: "native-a", Weight: 1, Enabled: boolPtr(true), Capabilities: internalconfig.VirtualModelCapabilitiesConfig{Operations: []string{"chat_image_understanding"}, Input: []string{"text", "image"}, Output: []string{"text"}}},
		{ID: "audio-only", Provider: "provider-b", Model: "native-b", Weight: 1, Enabled: boolPtr(true), Capabilities: internalconfig.VirtualModelCapabilitiesConfig{Operations: []string{"chat_audio_understanding"}, Input: []string{"text", "audio"}, Output: []string{"text"}}},
	})
}

func TestVirtualModelHTTPMixedImageAudioFailsClosedWithoutCombinedMember(t *testing.T) {
	r, executors := setupVirtualHTTPTest(t, virtualHTTPSplitMediaConfig())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"ail-compound","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}},{"type":"input_audio","input_audio":{"data":"abc","format":"mp3"}}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected mixed image+audio request with no combined member to fail closed with 422, got %d body=%s", w.Code, w.Body.String())
	}
	if executors["provider-a"].Count() != 0 || executors["provider-b"].Count() != 0 || executors["provider-agentic"].Count() != 0 {
		t.Fatalf("mixed image+audio request fell through to incomplete providers: provider-a=%d provider-b=%d provider-agentic=%d", executors["provider-a"].Count(), executors["provider-b"].Count(), executors["provider-agentic"].Count())
	}
}
