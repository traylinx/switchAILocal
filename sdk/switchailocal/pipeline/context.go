// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"context"
	"net/http"

	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	sdktranslator "github.com/traylinx/switchAILocal/sdk/translator"
)

// Context encapsulates execution state shared across middleware, translators, and executors.
type Context struct {
	// Request encapsulates the provider facing request payload.
	Request switchailocalexecutor.Request
	// Options carries execution flags (streaming, headers, etc.).
	Options switchailocalexecutor.Options
	// Auth references the credential selected for execution.
	Auth *switchailocalauth.Auth
	// Translator represents the pipeline responsible for schema adaptation.
	Translator *sdktranslator.Pipeline
	// HTTPClient allows middleware to customise the outbound transport per request.
	HTTPClient *http.Client
}

// Hook captures middleware callbacks around execution.
type Hook interface {
	BeforeExecute(ctx context.Context, execCtx *Context)
	AfterExecute(ctx context.Context, execCtx *Context, resp switchailocalexecutor.Response, err error)
	OnStreamChunk(ctx context.Context, execCtx *Context, chunk switchailocalexecutor.StreamChunk)
}

// HookFunc aggregates optional hook implementations.
type HookFunc struct {
	Before func(context.Context, *Context)
	After  func(context.Context, *Context, switchailocalexecutor.Response, error)
	Stream func(context.Context, *Context, switchailocalexecutor.StreamChunk)
}

// BeforeExecute implements Hook.
func (h HookFunc) BeforeExecute(ctx context.Context, execCtx *Context) {
	if h.Before != nil {
		h.Before(ctx, execCtx)
	}
}

// AfterExecute implements Hook.
func (h HookFunc) AfterExecute(ctx context.Context, execCtx *Context, resp switchailocalexecutor.Response, err error) {
	if h.After != nil {
		h.After(ctx, execCtx, resp, err)
	}
}

// OnStreamChunk implements Hook.
func (h HookFunc) OnStreamChunk(ctx context.Context, execCtx *Context, chunk switchailocalexecutor.StreamChunk) {
	if h.Stream != nil {
		h.Stream(ctx, execCtx, chunk)
	}
}

// RoundTripperProvider allows injection of custom HTTP transports per auth entry.
type RoundTripperProvider interface {
	RoundTripperFor(auth *switchailocalauth.Auth) http.RoundTripper
}
