package superbrain

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traylinx/switchAILocal/internal/config"
	sdkauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// mockExecutor is a mock implementation of ProviderExecutor for testing.
type mockExecutor struct {
	identifier     string
	executeFunc    func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error)
	executeStreamFunc func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error)
	refreshFunc    func(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error)
	countTokensFunc func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error)
}

func (m *mockExecutor) Identifier() string {
	return m.identifier
}

func (m *mockExecutor) Execute(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, auth, req, opts)
	}
	return switchailocalexecutor.Response{Payload: []byte(`{"choices":[{"message":{"content":"test response"}}]}`)}, nil
}

func (m *mockExecutor) ExecuteStream(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
	if m.executeStreamFunc != nil {
		return m.executeStreamFunc(ctx, auth, req, opts)
	}
	ch := make(chan switchailocalexecutor.StreamChunk, 1)
	ch <- switchailocalexecutor.StreamChunk{Payload: []byte(`{"choices":[{"delta":{"content":"test"}}]}`)}
	close(ch)
	return ch, nil
}

func (m *mockExecutor) Refresh(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, auth)
	}
	return auth, nil
}

func (m *mockExecutor) CountTokens(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	if m.countTokensFunc != nil {
		return m.countTokensFunc(ctx, auth, req, opts)
	}
	return switchailocalexecutor.Response{Payload: []byte(`{"total_tokens": 100}`)}, nil
}

func TestSuperbrainExecutor_Identifier(t *testing.T) {
	mock := &mockExecutor{identifier: "test-provider"}
	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)
	assert.Equal(t, "test-provider", se.Identifier())
}

func TestSuperbrainExecutor_DisabledMode(t *testing.T) {
	executeCalled := false
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			executeCalled = true
			return switchailocalexecutor.Response{Payload: []byte(`{"result":"success"}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: false,
		Mode:    "disabled",
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.True(t, executeCalled)
	assert.Equal(t, `{"result":"success"}`, string(resp.Payload))
}

func TestSuperbrainExecutor_ObserveMode(t *testing.T) {
	executeCalled := false
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			executeCalled = true
			return switchailocalexecutor.Response{Payload: []byte(`{"choices":[{"message":{"content":"test"}}]}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
		Overwatch: config.OverwatchConfig{
			SilenceThresholdMs:  30000,
			LogBufferSize:       50,
			HeartbeatIntervalMs: 1000,
			MaxRestartAttempts:  2,
		},
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.True(t, executeCalled)
	assert.NotEmpty(t, resp.Payload)
}

func TestSuperbrainExecutor_DiagnoseMode(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			return switchailocalexecutor.Response{Payload: []byte(`{"choices":[{"message":{"content":"test"}}]}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "diagnose",
		Overwatch: config.OverwatchConfig{
			SilenceThresholdMs:  30000,
			LogBufferSize:       50,
			HeartbeatIntervalMs: 1000,
			MaxRestartAttempts:  2,
		},
		Doctor: config.DoctorConfig{
			Model:     "gemini-flash",
			TimeoutMs: 5000,
		},
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Payload)
}

func TestSuperbrainExecutor_ConservativeMode(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			return switchailocalexecutor.Response{Payload: []byte(`{"choices":[{"message":{"content":"test"}}]}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "conservative",
		Overwatch: config.OverwatchConfig{
			SilenceThresholdMs:  30000,
			LogBufferSize:       50,
			HeartbeatIntervalMs: 1000,
			MaxRestartAttempts:  2,
		},
		StdinInjection: config.StdinInjectionConfig{
			Mode: "conservative",
		},
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Payload)
}

func TestSuperbrainExecutor_AutopilotMode(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			return switchailocalexecutor.Response{Payload: []byte(`{"choices":[{"message":{"content":"test"}}]}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "autopilot",
		Overwatch: config.OverwatchConfig{
			SilenceThresholdMs:  30000,
			LogBufferSize:       50,
			HeartbeatIntervalMs: 1000,
			MaxRestartAttempts:  2,
		},
		StdinInjection: config.StdinInjectionConfig{
			Mode: "autopilot",
		},
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Payload)
}

func TestSuperbrainExecutor_ExecuteStream(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeStreamFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
			ch := make(chan switchailocalexecutor.StreamChunk, 2)
			ch <- switchailocalexecutor.StreamChunk{Payload: []byte(`{"choices":[{"delta":{"content":"hello"}}]}`)}
			ch <- switchailocalexecutor.StreamChunk{Payload: []byte(`{"choices":[{"delta":{"content":" world"}}]}`)}
			close(ch)
			return ch, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	chunks, err := se.ExecuteStream(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)

	var received []string
	for chunk := range chunks {
		received = append(received, string(chunk.Payload))
	}

	assert.Len(t, received, 2)
}

func TestSuperbrainExecutor_Refresh(t *testing.T) {
	refreshCalled := false
	mock := &mockExecutor{
		identifier: "test-provider",
		refreshFunc: func(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error) {
			refreshCalled = true
			return auth, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)

	auth := &sdkauth.Auth{ID: "test-auth"}
	_, err := se.Refresh(context.Background(), auth)
	require.NoError(t, err)
	assert.True(t, refreshCalled)
}

func TestSuperbrainExecutor_CountTokens(t *testing.T) {
	countCalled := false
	mock := &mockExecutor{
		identifier: "test-provider",
		countTokensFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			countCalled = true
			return switchailocalexecutor.Response{Payload: []byte(`{"total_tokens": 150}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.CountTokens(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.True(t, countCalled)
	assert.Contains(t, string(resp.Payload), "150")
}

func TestSuperbrainExecutor_UpdateConfig(t *testing.T) {
	mock := &mockExecutor{identifier: "test-provider"}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)
	assert.Equal(t, "observe", se.getMode())

	newCfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "autopilot",
	}

	se.UpdateConfig(newCfg)
	assert.Equal(t, "autopilot", se.getMode())
}

func TestSuperbrainExecutor_Stop(t *testing.T) {
	mock := &mockExecutor{identifier: "test-provider"}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
		Overwatch: config.OverwatchConfig{
			SilenceThresholdMs:  30000,
			LogBufferSize:       50,
			HeartbeatIntervalMs: 1000,
			MaxRestartAttempts:  2,
		},
	}

	se := NewSuperbrainExecutor(mock, cfg)

	// Stop should not panic
	se.Stop()
}

func TestSuperbrainExecutor_ResponseEnrichment(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			return switchailocalexecutor.Response{Payload: []byte(`{"choices":[{"message":{"content":"test"}}]}`)}, nil
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)

	// Parse response to check structure
	var result map[string]interface{}
	err = json.Unmarshal(resp.Payload, &result)
	require.NoError(t, err)

	// Response should have choices
	choices, ok := result["choices"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, choices, 1)
}

func TestSuperbrainExecutor_NilConfig(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			return switchailocalexecutor.Response{Payload: []byte(`{"result":"success"}`)}, nil
		},
	}

	// Create with nil config - should work in disabled mode
	se := NewSuperbrainExecutor(mock, nil)

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	resp, err := se.Execute(context.Background(), nil, req, switchailocalexecutor.Options{})
	require.NoError(t, err)
	assert.Equal(t, `{"result":"success"}`, string(resp.Payload))
}

func TestSuperbrainExecutor_ContextCancellation(t *testing.T) {
	mock := &mockExecutor{
		identifier: "test-provider",
		executeFunc: func(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
			// Simulate slow execution
			select {
			case <-ctx.Done():
				return switchailocalexecutor.Response{}, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return switchailocalexecutor.Response{Payload: []byte(`{"result":"success"}`)}, nil
			}
		},
	}

	cfg := &config.SuperbrainConfig{
		Enabled: true,
		Mode:    "observe",
	}

	se := NewSuperbrainExecutor(mock, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := switchailocalexecutor.Request{
		Model:   "test-model",
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	_, err := se.Execute(ctx, nil, req, switchailocalexecutor.Options{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}
