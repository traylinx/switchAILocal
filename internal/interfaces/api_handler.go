// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package interfaces defines the core interfaces and shared structures for the switchAILocal server.
// These interfaces provide a common contract for different components of the application,
// such as AI service clients, API handlers, and data models.
package interfaces

// APIHandler defines the interface that all API handlers must implement.
// This interface provides methods for identifying handler types and retrieving
// supported models for different AI service endpoints.
type APIHandler interface {
	// HandlerType returns the type identifier for this API handler.
	// This is used to determine which request/response translators to use.
	HandlerType() string

	// Models returns a list of supported models for this API handler.
	// Each model is represented as a map containing model metadata.
	Models() []map[string]any
}
