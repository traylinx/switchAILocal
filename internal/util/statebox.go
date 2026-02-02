// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package util provides utility functions for the switchAILocal server.
package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// StateBox manages the canonical state directory for switchAILocal.
// It provides centralized path resolution for all mutable application data,
// ensuring consistent handling of environment variables and preventing ghost directories.
type StateBox struct {
	rootPath       string
	readOnly       bool
	legacyAuthDir  string // For backward compatibility with auth-dir config
	mu             sync.RWMutex
}

// NewStateBox creates a new StateBox instance.
// It reads SWITCHAI_STATE_DIR and SWITCHAI_READONLY from environment variables.
// If SWITCHAI_STATE_DIR is not set, it defaults to ~/.switchailocal.
// If SWITCHAI_READONLY is set to "1", the StateBox operates in read-only mode.
func NewStateBox() (*StateBox, error) {
	return NewStateBoxWithAuthDir("")
}

// NewStateBoxWithAuthDir creates a new StateBox instance with optional legacy auth-dir support.
// If legacyAuthDir is provided and non-empty, it will be used for credential operations
// to maintain backward compatibility with existing auth-dir configurations.
func NewStateBoxWithAuthDir(legacyAuthDir string) (*StateBox, error) {
	// Read state directory from environment or use default
	stateDir := os.Getenv("SWITCHAI_STATE_DIR")
	if stateDir == "" {
		stateDir = "~/.switchailocal"
	}

	// Expand tilde and clean the path
	resolvedPath, err := ExpandPath(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve state directory: %w", err)
	}

	// Resolve legacy auth dir if provided
	var resolvedLegacyAuthDir string
	if legacyAuthDir != "" {
		resolvedLegacyAuthDir, err = ExpandPath(legacyAuthDir)
		if err != nil {
			// Log warning but continue - fall back to State Box credentials dir
			resolvedLegacyAuthDir = ""
		}
	}

	// Check read-only mode
	readOnly := os.Getenv("SWITCHAI_READONLY") == "1"

	return &StateBox{
		rootPath:      resolvedPath,
		readOnly:      readOnly,
		legacyAuthDir: resolvedLegacyAuthDir,
	}, nil
}

// RootPath returns the resolved State Box root directory.
func (sb *StateBox) RootPath() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.rootPath
}

// IsReadOnly returns whether the State Box is in read-only mode.
func (sb *StateBox) IsReadOnly() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.readOnly
}

// DiscoveryDir returns the path to the discovery subdirectory.
func (sb *StateBox) DiscoveryDir() string {
	return filepath.Join(sb.RootPath(), "discovery")
}

// IntelligenceDir returns the path to the intelligence subdirectory.
func (sb *StateBox) IntelligenceDir() string {
	return filepath.Join(sb.RootPath(), "intelligence")
}

// CredentialsDir returns the path to the credentials subdirectory.
// If a legacy auth-dir was configured, it returns that path for backward compatibility.
// Otherwise, it returns the State Box credentials directory.
func (sb *StateBox) CredentialsDir() string {
	sb.mu.RLock()
	legacyDir := sb.legacyAuthDir
	sb.mu.RUnlock()

	if legacyDir != "" {
		return legacyDir
	}
	return filepath.Join(sb.RootPath(), "credentials")
}

// EnsureCredentialsDir creates the credentials directory with 0700 permissions if it doesn't exist.
// Returns the path to the credentials directory.
func (sb *StateBox) EnsureCredentialsDir() (string, error) {
	credDir := sb.CredentialsDir()
	if err := sb.EnsureDir(credDir); err != nil {
		return "", fmt.Errorf("failed to ensure credentials directory: %w", err)
	}
	return credDir, nil
}

// CredentialPath returns the full path for a credential file for the given provider.
// The provider name is sanitized to prevent path traversal attacks.
func (sb *StateBox) CredentialPath(provider string) string {
	// Sanitize provider name to prevent path traversal
	sanitized := filepath.Base(provider)
	if sanitized == "." || sanitized == ".." {
		sanitized = "unknown"
	}
	// Ensure .json extension
	if !strings.HasSuffix(sanitized, ".json") {
		sanitized = sanitized + ".json"
	}
	return filepath.Join(sb.CredentialsDir(), sanitized)
}

// ReadCredential reads a credential file for the given provider and unmarshals it into v.
// Returns os.ErrNotExist if the credential file does not exist.
func (sb *StateBox) ReadCredential(provider string, v interface{}) error {
	credPath := sb.CredentialPath(provider)

	data, err := os.ReadFile(credPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to unmarshal credential for %s: %w", provider, err)
	}

	return nil
}

// WriteCredential writes a credential to a file for the given provider using atomic writes.
// Returns ErrReadOnlyMode if the StateBox is in read-only mode.
func (sb *StateBox) WriteCredential(provider string, v interface{}) error {
	// Ensure credentials directory exists
	if _, err := sb.EnsureCredentialsDir(); err != nil {
		return err
	}

	credPath := sb.CredentialPath(provider)

	// Use SecureWriteJSON for atomic writes with 0600 permissions
	opts := &SecureWriteOptions{
		CreateBackup: true,
		Permissions:  0600,
	}

	return SecureWriteJSON(sb, credPath, v, opts)
}

// HasLegacyAuthDir returns true if a legacy auth-dir was configured.
func (sb *StateBox) HasLegacyAuthDir() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.legacyAuthDir != ""
}

// SetLegacyAuthDir sets the legacy auth-dir for backward compatibility.
// This should be called during initialization if the config has an auth-dir set.
func (sb *StateBox) SetLegacyAuthDir(authDir string) error {
	if authDir == "" {
		return nil
	}

	resolved, err := ExpandPath(authDir)
	if err != nil {
		return fmt.Errorf("failed to resolve legacy auth-dir: %w", err)
	}

	sb.mu.Lock()
	sb.legacyAuthDir = resolved
	sb.mu.Unlock()

	return nil
}

// ResolvePath joins a relative path with the State Box root.
// If the path is already absolute or starts with tilde, it is returned as-is after cleaning.
// Otherwise, it is joined with the State Box root directory.
func (sb *StateBox) ResolvePath(relativePath string) string {
	if relativePath == "" {
		return sb.RootPath()
	}

	// If path starts with tilde or is absolute, return cleaned path
	if strings.HasPrefix(relativePath, "~") || filepath.IsAbs(relativePath) {
		cleaned, err := ExpandPath(relativePath)
		if err != nil {
			// If expansion fails, return cleaned original path
			return filepath.Clean(relativePath)
		}
		return cleaned
	}

	// Join relative path with State Box root
	return filepath.Join(sb.RootPath(), relativePath)
}

// EnsureDir creates a directory with secure permissions (0700) if it doesn't exist.
// It creates all necessary parent directories as well.
// Returns an error if the directory cannot be created.
func (sb *StateBox) EnsureDir(path string) error {
	// Check if directory already exists
	info, err := os.Stat(path)
	if err == nil {
		// Directory exists, verify it's actually a directory
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		return nil
	}

	// If error is not "not exists", return it
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}

	// Create directory with 0700 permissions (owner read/write/execute only)
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	return nil
}
