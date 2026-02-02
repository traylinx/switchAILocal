// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/util"
)

// StateBoxStatus represents the State Box status for API responses.
type StateBoxStatus struct {
	RootPath          string      `json:"root_path"`
	ReadOnly          bool        `json:"read_only"`
	Initialized       bool        `json:"initialized"`
	DiscoveryRegistry *FileStatus `json:"discovery_registry,omitempty"`
	FeedbackDatabase  *FileStatus `json:"feedback_database,omitempty"`
	PermissionStatus  string      `json:"permission_status"` // "ok", "warning", "error"
	Warnings          []string    `json:"warnings,omitempty"`
	Errors            []string    `json:"errors,omitempty"`
}

// FileStatus represents the status of a State Box file.
type FileStatus struct {
	Path    string    `json:"path"`
	Exists  bool      `json:"exists"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time,omitempty"`
}

// getFileStatus retrieves the status of a file at the given path.
func getFileStatus(path string) *FileStatus {
	status := &FileStatus{
		Path:   path,
		Exists: false,
	}

	info, err := os.Stat(path)
	if err != nil {
		// File doesn't exist or can't be accessed
		return status
	}

	status.Exists = true
	status.Size = info.Size()
	status.Mode = info.Mode().String()
	status.ModTime = info.ModTime()

	return status
}

// StateBoxStatusHandler returns a handler for the /api/state-box/status endpoint.
// It provides information about the State Box configuration and file status.
func StateBoxStatusHandler(sb *util.StateBox) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sb == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "State Box not initialized",
			})
			return
		}

		status := &StateBoxStatus{
			RootPath:         sb.RootPath(),
			ReadOnly:         sb.IsReadOnly(),
			Initialized:      true,
			PermissionStatus: "ok",
			Warnings:         []string{},
			Errors:           []string{},
		}

		// Check if State Box root directory exists
		if _, err := os.Stat(sb.RootPath()); err != nil {
			if os.IsNotExist(err) {
				status.Warnings = append(status.Warnings, "State Box root directory does not exist")
				status.PermissionStatus = "warning"
			} else {
				status.Errors = append(status.Errors, "Failed to access State Box root directory")
				status.PermissionStatus = "error"
			}
		}

		// Get discovery registry status
		discoveryPath := sb.ResolvePath("discovery/registry.json")
		status.DiscoveryRegistry = getFileStatus(discoveryPath)

		// Get feedback database status
		feedbackPath := sb.ResolvePath("intelligence/feedback.db")
		status.FeedbackDatabase = getFileStatus(feedbackPath)

		// Check for permission issues
		if status.DiscoveryRegistry.Exists {
			if info, err := os.Stat(discoveryPath); err == nil {
				mode := info.Mode().Perm()
				// Check if permissions are too permissive (should be 0600)
				if mode&0077 != 0 {
					status.Warnings = append(status.Warnings, "Discovery registry has overly permissive permissions")
					if status.PermissionStatus == "ok" {
						status.PermissionStatus = "warning"
					}
				}
			}
		}

		if status.FeedbackDatabase.Exists {
			if info, err := os.Stat(feedbackPath); err == nil {
				mode := info.Mode().Perm()
				// Check if permissions are too permissive (should be 0600)
				if mode&0077 != 0 {
					status.Warnings = append(status.Warnings, "Feedback database has overly permissive permissions")
					if status.PermissionStatus == "ok" {
						status.PermissionStatus = "warning"
					}
				}
			}
		}

		c.JSON(http.StatusOK, status)
	}
}
