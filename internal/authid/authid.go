// Package authid defines the canonical identifier and containment contract for
// auth files managed by switchAILocal stores.
package authid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maxIDBytes      = 4096
	maxSegmentBytes = 255
)

// Validate checks that id is a canonical, slash-separated relative auth ID.
// It validates rather than normalizes so malformed spellings cannot silently
// change the ID. Stores must use FoldKey to reject case-folding and Unicode-
// normalization collisions against existing IDs before writes.
func Validate(id string) error {
	if id == "" {
		return fmt.Errorf("auth id is empty")
	}
	if len(id) > maxIDBytes {
		return fmt.Errorf("auth id exceeds %d bytes", maxIDBytes)
	}
	if !utf8.ValidString(id) {
		return fmt.Errorf("auth id is not valid UTF-8")
	}
	if strings.HasPrefix(id, "/") || strings.HasSuffix(id, "/") {
		return fmt.Errorf("auth id must be relative without a trailing separator")
	}
	if strings.ContainsRune(id, '\\') {
		return fmt.Errorf("auth id must use forward-slash separators")
	}
	for _, r := range id {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return fmt.Errorf("auth id contains a control character")
		}
		if unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("auth id contains a Unicode format character")
		}
	}
	for _, segment := range strings.Split(id, "/") {
		if segment == "" {
			return fmt.Errorf("auth id contains an empty segment")
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("auth id contains a traversal segment")
		}
		if strings.ContainsAny(segment, `<>:"|?*`) {
			return fmt.Errorf("auth id contains a Windows-reserved character")
		}
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return fmt.Errorf("auth id segment has a Windows-ambiguous suffix")
		}
		if isWindowsDeviceName(segment) {
			return fmt.Errorf("auth id contains a Windows-reserved device name")
		}
		if len(segment) > maxSegmentBytes {
			return fmt.Errorf("auth id segment exceeds %d bytes", maxSegmentBytes)
		}
	}
	return nil
}

// FoldKey returns a conservative, cross-platform collision key for id. Stores
// must not persist two distinct auth IDs with the same key. The key is for
// equality checks only and must never replace the case-preserving auth ID.
func FoldKey(id string) (string, error) {
	if err := Validate(id); err != nil {
		return "", err
	}
	folded := cases.Fold().String(norm.NFD.String(id))
	return norm.NFD.String(folded), nil
}

// FromFSPath converts a path beneath root into a canonical auth ID. Relative
// paths are interpreted relative to root. Existing parent symlinks are resolved
// before containment is checked; the final file component is intentionally not
// followed so deleting a symlink removes the link, not its target. Callers must
// use the returned ID for subsequent access, never the original path string.
func FromFSPath(root, path string) (string, error) {
	rootReal, err := realRoot(root)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("auth path is empty")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootReal, filepath.FromSlash(path))
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve auth path: %w", err)
	}

	parentReal, tail, err := realExistingParent(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	parts := append([]string{parentReal}, tail...)
	parts = append(parts, filepath.Base(path))
	target := filepath.Join(parts...)
	rel, err := containedRelative(rootReal, target)
	if err != nil {
		return "", err
	}
	id := filepath.ToSlash(rel)
	if err = Validate(id); err != nil {
		return "", err
	}
	return id, nil
}

// ToFSPath resolves a canonical auth ID beneath root. It rejects an ID whose
// existing parent components resolve through a symlink to a different ID.
// Callers that read or mutate the filesystem must still use os.Root with the ID
// to enforce containment during the operation and prevent symlink races.
func ToFSPath(root, id string) (string, error) {
	if err := Validate(id); err != nil {
		return "", err
	}
	rootReal, err := realRoot(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(rootReal, filepath.FromSlash(id))
	resolvedID, err := FromFSPath(rootReal, target)
	if err != nil {
		return "", err
	}
	if resolvedID != id {
		return "", fmt.Errorf("auth id resolves to noncanonical path %q", resolvedID)
	}
	return target, nil
}

func realRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("auth root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve auth root: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve auth root symlinks: %w", err)
	}
	return filepath.Clean(real), nil
}

// realExistingParent resolves the deepest existing parent and returns any
// not-yet-created path components beneath it in original order.
func realExistingParent(parent string) (string, []string, error) {
	missing := make([]string, 0, 4)
	for {
		real, err := filepath.EvalSymlinks(parent)
		if err == nil {
			return real, missing, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, fmt.Errorf("resolve auth parent symlinks: %w", err)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", nil, fmt.Errorf("resolve auth parent: no existing ancestor")
		}
		missing = append([]string{filepath.Base(parent)}, missing...)
		parent = next
	}
}

func containedRelative(root, target string) (string, error) {
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil {
		return "", fmt.Errorf("compute auth path containment: %w", err)
	}
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("auth path is outside managed root")
	}
	return rel, nil
}

func isWindowsDeviceName(segment string) bool {
	base := segment
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, ". ")
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		return base[3] >= '1' && base[3] <= '9'
	}
	if len([]rune(base)) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		last := []rune(base)[3]
		return last == '\u00b9' || last == '\u00b2' || last == '\u00b3'
	}
	return false
}
