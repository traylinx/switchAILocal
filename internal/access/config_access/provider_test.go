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
		},
		{
			name:          "Strict Mode - Valid Google Auth",
			configKeys:    []string{"secret-123"},
			headers:       map[string]string{"X-Goog-Api-Key": "secret-123"},
			expectedErr:   nil,
			expectedPrinc: "secret-123",
		},
		{
			name:          "Strict Mode - Valid Query Param",
			configKeys:    []string{"secret-123"},
			queryParams:   map[string]string{"key": "secret-123"},
			expectedErr:   nil,
			expectedPrinc: "secret-123",
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
		},
		{
			name:          "Optional Mode - Empty Auth Array",
			configKeys:    []string{},
			headers:       map[string]string{},
			expectedErr:   nil,
			expectedPrinc: "anonymous",
		},
		{
			name:          "Optional Mode - Specific Auth Provided",
			configKeys:    []string{},
			headers:       map[string]string{"Authorization": "Bearer my-custom-key"},
			expectedErr:   nil,
			expectedPrinc: "my-custom-key",
		},
		{
			name:          "Optional Mode - Malformed Empty Auth",
			configKeys:    []string{},
			headers:       map[string]string{"Authorization": "Bearer "},
			expectedErr:   nil,
			expectedPrinc: "anonymous",
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
			}
		})
	}
}
