// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"errors"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

var ErrRefreshNotSupported = errors.New("switchailocal auth: refresh not supported")

// LoginOptions captures generic knobs shared across authenticators.
// Provider-specific logic can inspect Metadata for extra parameters.
type LoginOptions struct {
	NoBrowser bool
	ProjectID string
	Metadata  map[string]string
	Prompt    func(prompt string) (string, error)
}

// Authenticator manages login and optional refresh flows for a provider.
type Authenticator interface {
	Provider() string
	Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error)
	RefreshLead() *time.Duration
}
