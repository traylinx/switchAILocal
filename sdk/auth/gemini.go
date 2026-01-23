// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/traylinx/switchAILocal/internal/auth/gemini"
	// legacy client removed
	"github.com/traylinx/switchAILocal/internal/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// GeminiAuthenticator implements the login flow for Google Gemini CLI accounts.
type GeminiAuthenticator struct{}

// NewGeminiAuthenticator constructs a Gemini authenticator.
func NewGeminiAuthenticator() *GeminiAuthenticator {
	return &GeminiAuthenticator{}
}

func (a *GeminiAuthenticator) Provider() string {
	return "gemini"
}

func (a *GeminiAuthenticator) RefreshLead() *time.Duration {
	return nil
}

func (a *GeminiAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("switchailocal auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	var ts gemini.GeminiTokenStorage
	if opts.ProjectID != "" {
		ts.ProjectID = opts.ProjectID
	}

	geminiAuth := gemini.NewGeminiAuth()
	_, err := geminiAuth.GetAuthenticatedClient(ctx, &ts, cfg, &gemini.WebLoginOptions{
		NoBrowser: opts.NoBrowser,
		Prompt:    opts.Prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini authentication failed: %w", err)
	}

	// Skip onboarding here; rely on upstream configuration

	fileName := fmt.Sprintf("%s-%s.json", ts.Email, ts.ProjectID)
	metadata := map[string]any{
		"email":      ts.Email,
		"project_id": ts.ProjectID,
	}

	fmt.Println("Gemini authentication successful")

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  &ts,
		Metadata: metadata,
	}, nil
}
