// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/traylinx/switchAILocal/internal/registry"
	coreexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

type dispatchPolicyObserver struct{ allowed string }

func (*dispatchPolicyObserver) ObserveUsage(context.Context, coreusage.Record) {}

func (o *dispatchPolicyObserver) AuthorizeModel(model string) error {
	if model != o.allowed {
		return errors.New("caller_model_not_allowed")
	}
	return nil
}

type dispatchRecordingExecutor struct{ calls atomic.Int32 }

func (*dispatchRecordingExecutor) Identifier() string { return "dispatch-test" }
func (e *dispatchRecordingExecutor) Execute(context.Context, *Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls.Add(1)
	return coreexecutor.Response{Payload: []byte(`{}`)}, nil
}
func (e *dispatchRecordingExecutor) ExecuteStream(context.Context, *Auth, coreexecutor.Request, coreexecutor.Options) (<-chan coreexecutor.StreamChunk, error) {
	e.calls.Add(1)
	chunks := make(chan coreexecutor.StreamChunk)
	close(chunks)
	return chunks, nil
}
func (e *dispatchRecordingExecutor) CountTokens(context.Context, *Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls.Add(1)
	return coreexecutor.Response{Payload: []byte(`{}`)}, nil
}
func (*dispatchRecordingExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func newDispatchTestManager(t *testing.T) (*Manager, *dispatchRecordingExecutor) {
	t.Helper()
	m := NewManager(nil, nil, nil)
	exec := &dispatchRecordingExecutor{}
	m.RegisterExecutor(exec)
	if _, err := m.Register(context.Background(), &Auth{ID: "dispatch-auth", Provider: exec.Identifier(), Status: StatusActive}); err != nil {
		t.Fatal(err)
	}
	registry.GetGlobalRegistry().RegisterClient("dispatch-auth", exec.Identifier(), []*registry.ModelInfo{
		{ID: "allowed-model", Object: "model", OwnedBy: exec.Identifier()},
		{ID: "forbidden-model", Object: "model", OwnedBy: exec.Identifier()},
	})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("dispatch-auth") })
	return m, exec
}

func TestResolvedModelPolicyRunsAtEveryExecutorDispatch(t *testing.T) {
	for _, mode := range []string{"execute", "count", "stream"} {
		t.Run(mode, func(t *testing.T) {
			manager, executor := newDispatchTestManager(t)
			ctx := coreusage.WithModelAuthorizationRequired(context.Background())
			ctx = coreusage.WithSynchronousObserver(ctx, &dispatchPolicyObserver{allowed: "allowed-model"})
			req := coreexecutor.Request{Model: "forbidden-model", Payload: []byte(`{}`)}
			var err error
			switch mode {
			case "execute":
				_, err = manager.Execute(ctx, []string{executor.Identifier()}, req, coreexecutor.Options{})
			case "count":
				_, err = manager.ExecuteCount(ctx, []string{executor.Identifier()}, req, coreexecutor.Options{})
			case "stream":
				_, err = manager.ExecuteStream(ctx, []string{executor.Identifier()}, req, coreexecutor.Options{})
			}
			if err == nil || err.Error() != "caller_model_not_allowed" {
				t.Fatalf("dispatch error = %v", err)
			}
			if calls := executor.calls.Load(); calls != 0 {
				t.Fatalf("forbidden model reached executor %d times", calls)
			}
		})
	}
}

func TestNamedCallerDispatchWithoutAuthorizerFailsClosed(t *testing.T) {
	manager, executor := newDispatchTestManager(t)
	ctx := coreusage.WithModelAuthorizationRequired(context.Background())
	_, err := manager.Execute(ctx, []string{executor.Identifier()}, coreexecutor.Request{Model: "allowed-model"}, coreexecutor.Options{})
	if !errors.Is(err, coreusage.ErrModelAuthorizerUnavailable) {
		t.Fatalf("dispatch error = %v, want %v", err, coreusage.ErrModelAuthorizerUnavailable)
	}
	if calls := executor.calls.Load(); calls != 0 {
		t.Fatalf("observer-less named caller reached executor %d times", calls)
	}
}
