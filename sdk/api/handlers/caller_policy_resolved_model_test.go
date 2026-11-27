// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	internalconfig "github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/virtualmodels"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

type resolvedModelPolicyObserver struct {
	allowed string
	seen    []string
}

func (*resolvedModelPolicyObserver) ObserveUsage(context.Context, coreusage.Record) {}

func (o *resolvedModelPolicyObserver) AuthorizeModel(model string) error {
	o.seen = append(o.seen, model)
	if model != o.allowed {
		return errors.New("caller_model_not_allowed")
	}
	return nil
}

func TestResolvedVirtualModelIsRecheckedBeforeExecutorDispatch(t *testing.T) {
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
	handler := NewBaseAPIHandlers(cfg, nil, nil, nil, nil, nil)
	handler.VirtualRouter = virtualmodels.NewRouterWithStatePath(filepath.Join(t.TempDir(), "routes.json"))
	observer := &resolvedModelPolicyObserver{allowed: "public-alias"}
	ctx := coreusage.WithSynchronousObserver(context.Background(), observer)

	_, _, _, _, errMsg := handler.getRequestDetails(ctx, "public-alias", []byte(`{"model":"public-alias","messages":[{"role":"user","content":"hello"}]}`))
	if errMsg == nil || errMsg.StatusCode != http.StatusForbidden || errMsg.Error == nil || errMsg.Error.Error() != "caller_model_not_allowed" {
		t.Fatalf("resolved-model policy error = %#v", errMsg)
	}
	if len(observer.seen) != 1 || observer.seen[0] != "private-native-model" {
		t.Fatalf("resolved models checked = %v", observer.seen)
	}
}
