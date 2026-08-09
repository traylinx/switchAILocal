// Package privatefile provides dependency-minimal owner-only atomic writes for
// sensitive artifacts used below the config/util dependency layer.
package privatefile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write atomically replaces path with owner-only bytes. A missing parent is
// created at 0700. Existing parent modes are never changed because path may be
// an operator-selected location. Symlinks and filesystems that cannot replace
// an anchored inode fall back to a chmod-before-truncate write.
func Write(path string, data []byte) error {
	return writeWithOps(path, data, os.CreateTemp, os.Rename)
}

func writeWithOps(
	path string,
	data []byte,
	createTemp func(string, string) (*os.File, error),
	rename func(string, string) error,
) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return writeInPlace(path, data, fmt.Errorf("create private file directory: %w", err))
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return writeInPlace(path, data, nil)
	}
	temp, err := createTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return writeInPlace(path, data, fmt.Errorf("create private temp file: %w", err))
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err = temp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure private temp file: %w", err)
	}
	if _, err = temp.Write(data); err != nil {
		return fmt.Errorf("write private temp file: %w", err)
	}
	if err = temp.Sync(); err != nil {
		return fmt.Errorf("sync private temp file: %w", err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("close private temp file: %w", err)
	}
	if err = rename(tempPath, path); err != nil {
		return writeInPlace(path, data, fmt.Errorf("replace private file: %w", err))
	}
	cleanup = false
	if directory, openErr := os.Open(dir); openErr == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

// WriteAnchored preserves an existing target inode. Use it for operator-owned
// config and credential files that may be bind-mounted, symlinked, hard-linked,
// or writable while their parent directory is not. Fresh files still use the
// atomic Write path.
func WriteAnchored(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return writeInPlace(path, data, nil)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat private file: %w", err)
	}
	return Write(path, data)
}

func writeInPlace(path string, data []byte, replacementErr error) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		if replacementErr != nil {
			return fmt.Errorf("%v; fallback open failed: %w", replacementErr, err)
		}
		return fmt.Errorf("open anchored private file: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err = file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure anchored private file: %w", err)
	}
	if err = file.Truncate(0); err != nil {
		return fmt.Errorf("truncate anchored private file: %w", err)
	}
	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("write anchored private file: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync anchored private file: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close anchored private file: %w", err)
	}
	return nil
}
