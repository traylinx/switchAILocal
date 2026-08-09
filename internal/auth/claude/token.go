// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package claude provides authentication and token management functionality
// for Anthropic's Claude AI services. It handles OAuth2 token storage, serialization,
// and retrieval for maintaining authenticated sessions with the Claude API.
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/traylinx/switchAILocal/internal/auth/credentialfile"
	"github.com/traylinx/switchAILocal/internal/misc"
)

// MarshalToken serializes the credential without performing path-based I/O.
func (ts *ClaudeTokenStorage) MarshalToken() ([]byte, error) {
	ts.Type = "claude"
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ClaudeTokenStorage stores OAuth2 token information for Anthropic Claude API authentication.
// It maintains compatibility with the existing auth system while adding Claude-specific fields
// for managing access tokens, refresh tokens, and user account information.
type ClaudeTokenStorage struct {
	// IDToken is the JWT ID token containing user claims and identity information.
	IDToken string `json:"id_token"`

	// AccessToken is the OAuth2 access token used for authenticating API requests.
	AccessToken string `json:"access_token"`

	// RefreshToken is used to obtain new access tokens when the current one expires.
	RefreshToken string `json:"refresh_token"`

	// LastRefresh is the timestamp of the last token refresh operation.
	LastRefresh string `json:"last_refresh"`

	// Email is the Anthropic account email address associated with this token.
	Email string `json:"email"`

	// Type indicates the authentication provider type, always "claude" for this storage.
	Type string `json:"type"`

	// Expire is the timestamp when the current access token expires.
	Expire string `json:"expired"`
}

// SaveTokenToFile serializes the Claude token storage to a JSON file.
// This method creates the necessary directory structure and writes the token
// data in JSON format to the specified file path for persistent storage.
//
// Parameters:
//   - authFilePath: The full path where the token file should be saved
//
// Returns:
//   - error: An error if the operation fails, nil otherwise
func (ts *ClaudeTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "claude"

	data, err := ts.MarshalToken()
	if err != nil {
		return fmt.Errorf("failed to encode token: %w", err)
	}
	if err = credentialfile.Write(authFilePath, data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}
