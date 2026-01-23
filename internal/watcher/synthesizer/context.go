// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package synthesizer

import (
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
)

// SynthesisContext provides the context needed for auth synthesis.
type SynthesisContext struct {
	// Config is the current configuration
	Config *config.Config
	// AuthDir is the directory containing auth files
	AuthDir string
	// Now is the current time for timestamps
	Now time.Time
	// IDGenerator generates stable IDs for auth entries
	IDGenerator *StableIDGenerator
}
