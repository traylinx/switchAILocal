// Package credentialfile provides the single private, atomic persistence path
// for legacy provider credential serializers.
package credentialfile

import (
	"fmt"

	"github.com/traylinx/switchAILocal/internal/privatefile"
)

// Write persists owner-only credential bytes. Existing targets keep their inode
// so bind mounts, symlinks, hard links, and read-only parent directories retain
// their historical behavior; chmod occurs before truncation.
func Write(path string, data []byte) error {
	if err := privatefile.WriteAnchored(path, data); err != nil {
		return fmt.Errorf("write private credential: %w", err)
	}
	return nil
}
