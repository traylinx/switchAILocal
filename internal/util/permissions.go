// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

// AuditResult contains the results of a permission audit for a single file or directory.
type AuditResult struct {
	Path         string      // The file or directory path
	CurrentMode  os.FileMode // The current permission mode
	RequiredMode os.FileMode // The required permission mode
	WasCorrected bool        // Whether permissions were corrected
	Error        error       // Any error encountered during audit or correction
}

// AuditPermissions checks permissions in the State Box without modifying them.
// It returns a slice of AuditResult entries for each file and directory examined.
// Directories should have 0700 permissions, and sensitive files (.db, .json) should have 0600.
func AuditPermissions(sb *StateBox) ([]AuditResult, error) {
	if sb == nil {
		return nil, fmt.Errorf("StateBox cannot be nil")
	}

	rootPath := sb.RootPath()
	var results []AuditResult

	// Walk the State Box directory tree
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log warning but continue walking
			log.Warnf("permission audit: failed to access %s: %v", path, err)
			results = append(results, AuditResult{
				Path:  path,
				Error: err,
			})
			return nil
		}

		currentMode := info.Mode().Perm()
		var requiredMode os.FileMode

		if info.IsDir() {
			// Directories should be 0700
			requiredMode = 0700
		} else if isSensitiveFile(path) {
			// Sensitive files (.db, .json) should be 0600
			requiredMode = 0600
		} else {
			// Other files - no specific requirement, skip
			return nil
		}

		// Check if correction is needed
		needsCorrection := currentMode != requiredMode

		results = append(results, AuditResult{
			Path:         path,
			CurrentMode:  currentMode,
			RequiredMode: requiredMode,
			WasCorrected: false,
			Error:        nil,
		})

		if needsCorrection {
			log.Debugf("permission audit: %s has mode %04o, requires %04o", path, currentMode, requiredMode)
		}

		return nil
	})

	if err != nil {
		return results, fmt.Errorf("failed to walk State Box directory: %w", err)
	}

	return results, nil
}

// HardenPermissions audits and corrects permissions in the State Box.
// Directories are set to 0700 (owner read/write/execute only).
// Sensitive files (.db, .json) are set to 0600 (owner read/write only).
// Errors are logged as warnings, but the function continues processing.
// Returns an error only if the State Box is nil or the directory walk fails completely.
func HardenPermissions(sb *StateBox) error {
	if sb == nil {
		return fmt.Errorf("StateBox cannot be nil")
	}

	rootPath := sb.RootPath()
	
	// Check if root path exists
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		log.Warnf("permission hardening: State Box root does not exist: %s", rootPath)
		return nil
	}

	correctionCount := 0
	errorCount := 0

	// Walk the State Box directory tree
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log warning but continue walking
			log.Warnf("permission hardening: failed to access %s: %v", path, err)
			errorCount++
			return nil
		}

		currentMode := info.Mode().Perm()
		var requiredMode os.FileMode

		if info.IsDir() {
			// Directories should be 0700
			requiredMode = 0700
		} else if isSensitiveFile(path) {
			// Sensitive files (.db, .json) should be 0600
			requiredMode = 0600
		} else {
			// Other files - no specific requirement, skip
			return nil
		}

		// Check if correction is needed
		if currentMode != requiredMode {
			// Attempt to correct permissions
			if chmodErr := os.Chmod(path, requiredMode); chmodErr != nil {
				log.Warnf("permission hardening: failed to chmod %s from %04o to %04o: %v", 
					path, currentMode, requiredMode, chmodErr)
				errorCount++
			} else {
				log.Infof("security audit: corrected permissions for %s from %04o to %04o", 
					path, currentMode, requiredMode)
				correctionCount++
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk State Box directory: %w", err)
	}

	// Log summary
	if correctionCount > 0 {
		log.Infof("permission hardening: corrected %d file/directory permissions", correctionCount)
	}
	if errorCount > 0 {
		log.Warnf("permission hardening: encountered %d errors during hardening", errorCount)
	}
	if correctionCount == 0 && errorCount == 0 {
		log.Debugf("permission hardening: all permissions already correct")
	}

	return nil
}

// isSensitiveFile returns true if the file should have restricted permissions (0600).
// Currently checks for .db and .json file extensions.
func isSensitiveFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".db" || ext == ".json"
}
