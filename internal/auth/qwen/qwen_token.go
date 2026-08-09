// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package qwen provides authentication and token management functionality
// for Alibaba's Qwen AI services. It handles OAuth2 token storage, serialization,
// and retrieval for maintaining authenticated sessions with the Qwen API.
package qwen

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/traylinx/switchAILocal/internal/auth/credentialfile"
	"github.com/traylinx/switchAILocal/internal/misc"
)

// MarshalToken serializes the credential without performing path-based I/O.
func (ts *QwenTokenStorage) MarshalToken() ([]byte, error) {
	ts.Type = "qwen"
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// QwenTokenStorage stores OAuth2 token information for Alibaba Qwen API authentication.
// It maintains compatibility with the existing auth system while adding Qwen-specific fields
// for managing access tokens, refresh tokens, and user account information.
type QwenTokenStorage struct {
	// AccessToken is the OAuth2 access token used for authenticating API requests.
	AccessToken string `json:"access_token"`
	// RefreshToken is used to obtain new access tokens when the current one expires.
	RefreshToken string `json:"refresh_token"`
	// LastRefresh is the timestamp of the last token refresh operation.
	LastRefresh string `json:"last_refresh"`
	// ResourceURL is the base URL for API requests.
	ResourceURL string `json:"resource_url"`
	// Email is the Qwen account email address associated with this token.
	Email string `json:"email"`
	// Type indicates the authentication provider type, always "qwen" for this storage.
	Type string `json:"type"`
	// Expire is the timestamp when the current access token expires.
	Expire string `json:"expired"`
}

// SaveTokenToFile serializes the Qwen token storage to a JSON file.
// This method creates the necessary directory structure and writes the token
// data in JSON format to the specified file path for persistent storage.
//
// Parameters:
//   - authFilePath: The full path where the token file should be saved
//
// Returns:
//   - error: An error if the operation fails, nil otherwise
func (ts *QwenTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "qwen"
	data, err := ts.MarshalToken()
	if err != nil {
		return fmt.Errorf("failed to encode token: %w", err)
	}
	if err = credentialfile.Write(authFilePath, data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}
