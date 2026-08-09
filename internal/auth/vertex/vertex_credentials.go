// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package vertex provides token storage for Google Vertex AI Gemini via service account credentials.
// It serialises service account JSON into an auth file that is consumed by the runtime executor.
package vertex

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/traylinx/switchAILocal/internal/auth/credentialfile"
	"github.com/traylinx/switchAILocal/internal/misc"
)

// MarshalToken serializes the credential without performing path-based I/O.
func (s *VertexCredentialStorage) MarshalToken() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("vertex credential: storage is nil")
	}
	if s.ServiceAccount == nil {
		return nil, fmt.Errorf("vertex credential: service account content is empty")
	}
	s.Type = "vertex"
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// VertexCredentialStorage stores the service account JSON for Vertex AI access.
// The content is persisted verbatim under the "service_account" key, together with
// helper fields for project, location and email to improve logging and discovery.
type VertexCredentialStorage struct {
	// ServiceAccount holds the parsed service account JSON content.
	ServiceAccount map[string]any `json:"service_account"`

	// ProjectID is derived from the service account JSON (project_id).
	ProjectID string `json:"project_id"`

	// Email is the client_email from the service account JSON.
	Email string `json:"email"`

	// Location optionally sets a default region (e.g., us-central1) for Vertex endpoints.
	Location string `json:"location,omitempty"`

	// Type is the provider identifier stored alongside credentials. Always "vertex".
	Type string `json:"type"`
}

// SaveTokenToFile writes the credential payload to the given file path in JSON format.
// It ensures the parent directory exists and logs the operation for transparency.
func (s *VertexCredentialStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	if s == nil {
		return fmt.Errorf("vertex credential: storage is nil")
	}
	if s.ServiceAccount == nil {
		return fmt.Errorf("vertex credential: service account content is empty")
	}
	// Ensure we tag the file with the provider type.
	s.Type = "vertex"

	data, err := s.MarshalToken()
	if err != nil {
		return fmt.Errorf("vertex credential: encode failed: %w", err)
	}
	if err = credentialfile.Write(authFilePath, data); err != nil {
		return fmt.Errorf("vertex credential: %w", err)
	}
	return nil
}
