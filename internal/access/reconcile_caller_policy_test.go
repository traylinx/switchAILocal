// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package access

import (
	"context"
	"net/http"
	"testing"

	configaccess "github.com/traylinx/switchAILocal/internal/access/config_access"
	"github.com/traylinx/switchAILocal/internal/config"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
)

func TestReconcileExplicitConfigAPIKeyProviderRebuildsCallerKeySwap(t *testing.T) {
	configaccess.Register()
	makeConfig := func(keyA, keyB string) *config.Config {
		return &config.Config{SDKConfig: sdkconfig.SDKConfig{
			Access: sdkconfig.AccessConfig{Providers: []sdkconfig.AccessProvider{{
				Name: "explicit-keys", Type: sdkconfig.AccessProviderTypeConfigAPIKey, APIKeys: []string{"key-1", "key-2"},
			}}},
			CallerPolicies: sdkconfig.CallerPoliciesConfig{Callers: []sdkconfig.CallerPolicyConfig{
				{ID: "caller-a", APIKey: keyA, AllowedModels: []string{"model-a"}, MaxConcurrency: 1, DailyTokenCap: 100},
				{ID: "caller-b", APIKey: keyB, AllowedModels: []string{"model-b"}, MaxConcurrency: 1, DailyTokenCap: 100},
			}},
		}}
	}
	oldCfg := makeConfig("key-1", "key-2")
	newCfg := makeConfig("key-2", "key-1")
	providers, err := sdkaccess.BuildProviders(&oldCfg.SDKConfig)
	if err != nil {
		t.Fatal(err)
	}
	manager := sdkaccess.NewManager()
	manager.SetProviders(providers)
	changed, err := ApplyAccessProviders(manager, oldCfg, newCfg)
	if err != nil || !changed {
		t.Fatalf("apply changed=%v err=%v", changed, err)
	}

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer key-1")
	result, err := manager.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Metadata["caller_id"]; got != "caller-b" {
		t.Fatalf("key-1 caller id = %q, want caller-b", got)
	}
}
