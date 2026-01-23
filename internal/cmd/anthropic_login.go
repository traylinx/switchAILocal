// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/auth/claude"
	"github.com/traylinx/switchAILocal/internal/config"
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
)

// DoClaudeLogin triggers the Claude OAuth flow through the shared authentication manager.
// It initiates the OAuth authentication process for Anthropic Claude services and saves
// the authentication tokens to the configured auth directory.
//
// Parameters:
//   - cfg: The application configuration
//   - options: Login options including browser behavior and prompts
func DoClaudeLogin(cfg *config.Config, options *LoginOptions) {
	if options == nil {
		options = &LoginOptions{}
	}

	promptFn := options.Prompt
	if promptFn == nil {
		promptFn = defaultProjectPrompt()
	}

	manager := newAuthManager()

	authOpts := &sdkAuth.LoginOptions{
		NoBrowser: options.NoBrowser,
		Metadata:  map[string]string{},
		Prompt:    promptFn,
	}

	_, savedPath, err := manager.Login(context.Background(), "claude", cfg, authOpts)
	if err != nil {
		var authErr *claude.AuthenticationError
		if errors.As(err, &authErr) {
			log.Error(claude.GetUserFriendlyMessage(authErr))
			if authErr.Type == claude.ErrPortInUse.Type {
				os.Exit(claude.ErrPortInUse.Code)
			}
			return
		}
		fmt.Printf("Claude authentication failed: %v\n", err)
		return
	}

	if savedPath != "" {
		fmt.Printf("Authentication saved to %s\n", savedPath)
	}

	fmt.Println("Claude authentication successful!")
}
