// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/traylinx/switchAILocal/internal/callerpolicy"
	internalconfig "github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/virtualmodels"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	coreexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

type resolvedModelRecordingExecutor struct{ calls atomic.Int32 }

func (*resolvedModelRecordingExecutor) Identifier() string { return "provider-a" }
func (e *resolvedModelRecordingExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls.Add(1)
	return coreexecutor.Response{Payload: []byte(`{}`)}, nil
}
func (e *resolvedModelRecordingExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (<-chan coreexecutor.StreamChunk, error) {
	e.calls.Add(1)
	ch := make(chan coreexecutor.StreamChunk)
	close(ch)
	return ch, nil
}
func (e *resolvedModelRecordingExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls.Add(1)
	return coreexecutor.Response{Payload: []byte(`{}`)}, nil
}
func (*resolvedModelRecordingExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func TestResolvedVirtualModelIsRecheckedAtExecutorDispatch(t *testing.T) {
	enabled := true
	cfg := &internalconfig.SDKConfig{VirtualModels: map[string]internalconfig.VirtualModelConfig{
		"public-alias": {
			Expose: true,
			Members: []internalconfig.VirtualModelMemberConfig{{
				ID:       "backend",
				Provider: "provider-a",
				Model:    "private-native-model",
				Enabled:  &enabled,
				Capabilities: internalconfig.VirtualModelCapabilitiesConfig{
					Operations: []string{"chat"}, Input: []string{"text"}, Output: []string{"text"},
				},
			}},
		},
	}}
	authManager := coreauth.NewManager(nil, nil, nil)
	executor := &resolvedModelRecordingExecutor{}
	authManager.RegisterExecutor(executor)
	if _, err := authManager.Register(context.Background(), &coreauth.Auth{ID: "auth-a", Provider: "provider-a", Status: coreauth.StatusActive}); err != nil {
		t.Fatal(err)
	}
	handler := NewBaseAPIHandlers(cfg, authManager, nil, nil, nil, nil)
	handler.VirtualRouter = virtualmodels.NewRouterWithStatePath(filepath.Join(t.TempDir(), "routes.json"))
	policyCfg := internalconfig.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "usage.json"),
		Callers: []internalconfig.CallerPolicyConfig{{
			ID: "caller-a", APIKey: "key-a", AllowedModels: []string{"public-alias"}, MaxConcurrency: 1, DailyTokenCap: 100,
		}},
	}
	policyManager := callerpolicy.New(policyCfg, "")
	t.Cleanup(func() { _ = policyManager.Close() })
	lease, err := policyManager.Begin("caller-a", "public-alias", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Finish(http.StatusForbidden)
	ctx := coreusage.WithModelAuthorizationRequired(context.Background())
	ctx = coreusage.WithSynchronousObserver(ctx, lease)

	_, errMsg := handler.ExecuteWithAuthManager(ctx, "openai", "public-alias", []byte(`{"model":"public-alias","messages":[{"role":"user","content":"hello"}]}`), "")
	if errMsg == nil || errMsg.StatusCode != http.StatusForbidden || errMsg.Error == nil || errMsg.Error.Error() != string(callerpolicy.CodeModelDenied) {
		t.Fatalf("resolved-model policy error = %#v", errMsg)
	}
	if calls := executor.calls.Load(); calls != 0 {
		t.Fatalf("forbidden resolved model reached executor %d times", calls)
	}
}
