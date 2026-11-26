package configaccess

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	sdkconfig "github.com/traylinx/switchAILocal/sdk/config"
)

func TestProvider_Authenticate(t *testing.T) {
	tests := []struct {
		name          string
		configKeys    []string
		headers       map[string]string
		queryParams   map[string]string
		expectedErr   error
		expectedPrinc string
		expectedMode  string
	}{
		// -------------------------
		// Strict Mode (Keys configured)
		// -------------------------
		{
			name:        "Strict Mode - No Auth Provided",
			configKeys:  []string{"secret-123"},
			headers:     map[string]string{},
			expectedErr: sdkaccess.ErrNoCredentials,
		},
		{
			name:        "Strict Mode - Invalid Auth Provided",
			configKeys:  []string{"secret-123"},
			headers:     map[string]string{"Authorization": "Bearer wrong-key"},
			expectedErr: sdkaccess.ErrInvalidCredential,
		},
		{
			name:          "Strict Mode - Valid Bearer Auth",
			configKeys:    []string{"secret-123"},
			headers:       map[string]string{"Authorization": "Bearer secret-123"},
			expectedErr:   nil,
			expectedPrinc: "secret-123",
			expectedMode:  "",
		},
		{
			name:          "Strict Mode - Valid Google Auth",
			configKeys:    []string{"secret-123"},
			headers:       map[string]string{"X-Goog-Api-Key": "secret-123"},
			expectedErr:   nil,
			expectedPrinc: "secret-123",
			expectedMode:  "",
		},
		{
			name:          "Strict Mode - Valid Query Param",
			configKeys:    []string{"secret-123"},
			queryParams:   map[string]string{"key": "secret-123"},
			expectedErr:   nil,
			expectedPrinc: "secret-123",
			expectedMode:  "",
		},
		// -------------------------
		// Optional Mode (No Keys configured / Empty)
		// -------------------------
		{
			name:          "Optional Mode - No Auth Provided",
			configKeys:    []string{""}, // Simulates config parsing empty string
			headers:       map[string]string{},
			expectedErr:   nil,
			expectedPrinc: "anonymous",
			expectedMode:  "optional",
		},
		{
			name:          "Optional Mode - Empty Auth Array",
			configKeys:    []string{},
			headers:       map[string]string{},
			expectedErr:   nil,
			expectedPrinc: "anonymous",
			expectedMode:  "optional",
		},
		{
			name:          "Optional Mode - Specific Auth Provided",
			configKeys:    []string{},
			headers:       map[string]string{"Authorization": "Bearer my-custom-key"},
			expectedErr:   nil,
			expectedPrinc: "my-custom-key",
			expectedMode:  "optional",
		},
		{
			name:          "Optional Mode - Malformed Empty Auth",
			configKeys:    []string{},
			headers:       map[string]string{"Authorization": "Bearer "},
			expectedErr:   nil,
			expectedPrinc: "anonymous",
			expectedMode:  "optional",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerCfg := &sdkconfig.AccessProvider{
				Name:    "test-provider",
				Type:    "config-api-key",
				APIKeys: tt.configKeys,
			}
			prov, err := newProvider(providerCfg, nil)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}

			req := &http.Request{
				Header: make(http.Header),
				URL:    &url.URL{},
			}

			// Add headers
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// Add queryparams
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()

			result, err := prov.Authenticate(context.Background(), req)

			if err != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}

			if tt.expectedErr == nil {
				if result == nil {
					t.Fatalf("expected result, got nil")
				}
				if result.Principal != tt.expectedPrinc {
					t.Errorf("expected principal %q, got %q", tt.expectedPrinc, result.Principal)
				}
				if gotMode := result.Metadata["mode"]; gotMode != tt.expectedMode {
					t.Errorf("expected metadata mode %q, got %q", tt.expectedMode, gotMode)
				}
			}
		})
	}
}

func TestProviderAddsCallerIDWithoutReplacingSecretPrincipal(t *testing.T) {
	root := &sdkconfig.SDKConfig{CallerPolicies: sdkconfig.CallerPoliciesConfig{Callers: []sdkconfig.CallerPolicyConfig{{
		ID: "agent-1", APIKey: "secret-123", AllowedModels: []string{"chat-model"}, MaxConcurrency: 1, DailyTokenCap: 100,
	}}}}
	providerCfg := sdkconfig.MakeInlineAPIKeyProvider(root.InboundAPIKeys())
	prov, err := newProvider(providerCfg, root)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer secret-123")
	result, err := prov.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Principal != "secret-123" {
		t.Fatalf("authentication principal changed: %q", result.Principal)
	}
	if result.Metadata["caller_id"] != "agent-1" {
		t.Fatalf("caller id metadata = %q", result.Metadata["caller_id"])
	}
}

func TestProviderMapsEverySupportedCredentialPathToCallerPolicy(t *testing.T) {
	root := &sdkconfig.SDKConfig{CallerPolicies: sdkconfig.CallerPoliciesConfig{Callers: []sdkconfig.CallerPolicyConfig{{
		ID: "service-a", APIKey: "secret-123", AllowedModels: []string{"chat-model"}, MaxConcurrency: 1, DailyTokenCap: 100,
	}}}}
	providerCfg := sdkconfig.MakeInlineAPIKeyProvider(root.InboundAPIKeys())
	prov, err := newProvider(providerCfg, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		header string
		value  string
		query  string
	}{
		{"authorization", "Authorization", "Bearer secret-123", ""},
		{"google header", "X-Goog-Api-Key", "secret-123", ""},
		{"anthropic header", "X-Api-Key", "secret-123", ""},
		{"query key", "", "", "key=secret-123"},
		{"query auth token", "", "", "auth_token=secret-123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions?"+tc.query, nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			result, err := prov.Authenticate(context.Background(), req)
			if err != nil || result.Metadata["caller_id"] != "service-a" {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}
