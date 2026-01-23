// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
)

// DoVibeLogin triggers the "login" for Vibe (enabling local tool).
func DoVibeLogin(cfg *config.Config, options *LoginOptions) {
	if options == nil {
		options = &LoginOptions{}
	}

	manager := newAuthManager()
	authOpts := &sdkAuth.LoginOptions{
		NoBrowser: options.NoBrowser,
		Metadata:  map[string]string{},
	}

	record, savedPath, err := manager.Login(context.Background(), "vibe", cfg, authOpts)
	if err != nil {
		log.Errorf("Vibe authentication failed: %v", err)
		return
	}

	if savedPath != "" {
		fmt.Printf("Vibe enabled and saved to %s\n", savedPath)
	}
	if record != nil && record.Label != "" {
		fmt.Printf("Authenticated as %s\n", record.Label)
	}
	fmt.Println("Vibe enabled successfully!")
}
