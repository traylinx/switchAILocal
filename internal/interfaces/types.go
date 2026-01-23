// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package interfaces provides type aliases for backwards compatibility with translator functions.
// It defines common interface types used throughout the switchAILocal for request and response
// transformation operations, maintaining compatibility with the SDK translator package.
package interfaces

import sdktranslator "github.com/traylinx/switchAILocal/sdk/translator"

// Backwards compatible aliases for translator function types.
type TranslateRequestFunc = sdktranslator.RequestTransform

type TranslateResponseFunc = sdktranslator.ResponseStreamTransform

type TranslateResponseNonStreamFunc = sdktranslator.ResponseNonStreamTransform

type TranslateResponse = sdktranslator.ResponseTransform
