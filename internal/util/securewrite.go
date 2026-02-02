// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// ErrReadOnlyMode is returned when a write operation is attempted in read-only mode.
var ErrReadOnlyMode = errors.New("Read-only environment: write operations disabled")

// SecureWriteOptions configures the secure write operation.
type SecureWriteOptions struct {
	// CreateBackup creates a .bak file before overwriting an existing file
	CreateBackup bool
	// Permissions sets the file permissions (default: 0600)
	Permissions os.FileMode
}

// DefaultSecureWriteOptions returns the default options for SecureWrite.
func DefaultSecureWriteOptions() *SecureWriteOptions {
	return &SecureWriteOptions{
		CreateBackup: false,
		Permissions:  0600,
	}
}

// SecureWrite atomically writes data to a file using the rename-swap pattern.
// It writes to a temporary file first, calls fsync(), then atomically renames
// to the target path. This ensures that power failures or crashes do not
// corrupt the target file.
//
// If sb is in read-only mode, returns ErrReadOnlyMode without modifying any files.
// If opts is nil, default options are used (no backup, 0600 permissions).
//
// The atomic rename is guaranteed on Unix systems. On Windows, os.Rename()
// is atomic on NTFS when source and destination are on the same volume.
func SecureWrite(sb *StateBox, path string, data []byte, opts *SecureWriteOptions) error {
	// Check read-only mode first
	if sb != nil && sb.IsReadOnly() {
		return ErrReadOnlyMode
	}

	// Use default options if not provided
	if opts == nil {
		opts = DefaultSecureWriteOptions()
	}

	// Ensure permissions have a sensible default
	if opts.Permissions == 0 {
		opts.Permissions = 0600
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Generate unique temp file name
	tempPath := fmt.Sprintf("%s.tmp.%s", path, uuid.New().String())

	// Create temp file with restricted permissions
	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, opts.Permissions)
	if err != nil {
		return fmt.Errorf("failed to create temp file %s: %w", tempPath, err)
	}

	// Track whether we need to clean up the temp file
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			os.Remove(tempPath)
		}
	}()

	// Write data to temp file
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	// Sync to disk before rename to ensure durability
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to fsync temp file: %w", err)
	}

	// Close the file before rename
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Create backup if requested and target file exists
	if opts.CreateBackup {
		if _, err := os.Stat(path); err == nil {
			backupPath := path + ".bak"
			if err := copyFile(path, backupPath, opts.Permissions); err != nil {
				// Log warning but continue with write
				// Backup failure should not prevent the write operation
				fmt.Fprintf(os.Stderr, "warning: failed to create backup %s: %v\n", backupPath, err)
			}
		}
	}

	// Atomic rename - this is the critical operation
	// On Unix: rename() is atomic within the same filesystem
	// On Windows: os.Rename() is atomic on NTFS for same-volume operations
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file to target: %w", err)
	}

	// Rename succeeded, don't clean up temp file (it's now the target)
	cleanupTemp = false

	// Sync the directory to ensure the rename is durable
	// This is important for crash consistency on some filesystems
	if err := syncDir(dir); err != nil {
		// Log warning but don't fail - the file was written successfully
		fmt.Fprintf(os.Stderr, "warning: failed to sync directory %s: %v\n", dir, err)
	}

	return nil
}

// copyFile copies a file from src to dst with the specified permissions.
func copyFile(src, dst string, perm os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

// syncDir syncs a directory to ensure metadata changes are persisted.
// This is a best-effort operation and may not be supported on all platforms.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// SecureWriteJSON marshals data to JSON with indentation and writes it atomically.
// It uses SecureWrite internally, providing the same atomicity guarantees.
//
// If sb is in read-only mode, returns ErrReadOnlyMode without modifying any files.
// If opts is nil, default options are used (no backup, 0600 permissions).
func SecureWriteJSON(sb *StateBox, path string, v interface{}, opts *SecureWriteOptions) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Append newline for better file formatting
	data = append(data, '\n')

	return SecureWrite(sb, path, data, opts)
}
