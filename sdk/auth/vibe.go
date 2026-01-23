// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// VibeAuthenticator handles the "login" for Vibe which just confirms local availability.
type VibeAuthenticator struct{}

func NewVibeAuthenticator() *VibeAuthenticator {
	return &VibeAuthenticator{}
}

func (a *VibeAuthenticator) Provider() string {
	return "vibe"
}

// RefreshLead returns nil as there's no token expiration to manage
func (a *VibeAuthenticator) RefreshLead() *time.Duration {
	return nil
}

// VibeTokenStorage implements baseauth.TokenStorage (implicitly via interface)
type VibeTokenStorage struct {
	Email       string `json:"email"`
	AccessToken string `json:"access_token"`
}

func (s *VibeTokenStorage) SaveTokenToFile(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (a *VibeAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	// Vibe uses the local CLI state, so we just create a marker auth file
	// that tells the system "Vibe is enabled".

	fileName := "vibe-local.json"

	// We can store some metadata if needed, but empty is fine.
	metadata := map[string]any{
		"type":       "local-cli",
		"created_at": time.Now().UTC(),
	}

	storage := &VibeTokenStorage{
		Email:       "local@vibe",
		AccessToken: "dummy-local-token", // Needs a value to be considered valid by some checks
	}

	fmt.Println("Vibe is a local tool. Enabling Vibe provider...")

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  storage,
		Metadata: metadata,
	}, nil
}
