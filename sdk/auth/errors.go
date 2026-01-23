// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"github.com/traylinx/switchAILocal/internal/interfaces"
)

// ProjectSelectionError indicates that the user must choose a specific project ID.
type ProjectSelectionError struct {
	Email    string
	Projects []interfaces.GCPProjectProjects
}

func (e *ProjectSelectionError) Error() string {
	if e == nil {
		return "switchailocal auth: project selection required"
	}
	return fmt.Sprintf("switchailocal auth: project selection required for %s", e.Email)
}

// ProjectsDisplay returns the projects list for caller presentation.
func (e *ProjectSelectionError) ProjectsDisplay() []interfaces.GCPProjectProjects {
	if e == nil {
		return nil
	}
	return e.Projects
}

// EmailRequiredError indicates that the calling context must provide an email or alias.
type EmailRequiredError struct {
	Prompt string
}

func (e *EmailRequiredError) Error() string {
	if e == nil || e.Prompt == "" {
		return "switchailocal auth: email is required"
	}
	return e.Prompt
}
