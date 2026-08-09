package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/authid"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

const (
	storeAtomicTempPrefix = ".tmp-"
	storeTempMaxAge       = time.Hour
)

type rootSafeTokenMarshaler interface {
	MarshalToken() ([]byte, error)
}

func resolveStoredAuthID(auth *switchailocalauth.Auth, baseDir string) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("auth store: auth is nil")
	}
	if auth.Attributes != nil && auth.Attributes["path"] != "" {
		return canonicalStoredAuthID(baseDir, auth.Attributes["path"])
	}
	if auth.FileName != "" {
		return canonicalStoredAuthID(baseDir, auth.FileName)
	}
	if auth.ID == "" {
		return "", fmt.Errorf("auth store: missing id")
	}
	return canonicalStoredAuthID(baseDir, auth.ID)
}

func canonicalStoredAuthID(baseDir, candidate string) (string, error) {
	var id string
	var err error
	if filepath.IsAbs(candidate) {
		id, err = authid.FromFSPath(baseDir, candidate)
	} else {
		if err = authid.Validate(candidate); err == nil {
			_, err = authid.ToFSPath(baseDir, candidate)
		}
		id = candidate
	}
	if err != nil {
		return "", err
	}
	base := path.Base(id)
	if len(base) <= len(".json") || !strings.HasSuffix(strings.ToLower(base), ".json") {
		return "", fmt.Errorf("auth id must end in .json")
	}
	return id, nil
}

func validateStoredAuthPath(root *os.Root, id string) (bool, error) {
	parts := strings.Split(id, "/")
	parent := "."
	for i, segment := range parts {
		entries, err := fs.ReadDir(root.FS(), parent)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("auth store: inspect %q: %w", parent, err)
		}
		candidateKey, err := authid.FoldKey(segment)
		if err != nil {
			return false, fmt.Errorf("auth store: invalid id segment %q: %w", segment, err)
		}
		var exact fs.DirEntry
		for _, entry := range entries {
			if entry.Name() == segment {
				exact = entry
				continue
			}
			existingKey, errKey := authid.FoldKey(entry.Name())
			if errKey == nil && existingKey == candidateKey {
				return false, fmt.Errorf("auth store: id segment %q collides with existing %q", segment, entry.Name())
			}
		}
		if exact == nil {
			return false, nil
		}
		if i < len(parts)-1 {
			if exact.Type()&fs.ModeSymlink != 0 {
				return false, fmt.Errorf("auth store: intermediate id segment %q is a symlink", segment)
			}
			if !exact.IsDir() {
				return false, fmt.Errorf("auth store: intermediate id segment %q is not a directory", segment)
			}
			parent = path.Join(parent, segment)
		} else if exact.IsDir() {
			return false, fmt.Errorf("auth store: final id segment %q is a directory", segment)
		}
	}
	return true, nil
}

func marshalStoredAuth(auth *switchailocalauth.Auth) ([]byte, error) {
	if auth.Storage == nil {
		if auth.Metadata == nil {
			return nil, fmt.Errorf("auth store: nothing to persist for %s", auth.ID)
		}
		raw, err := json.Marshal(auth.Metadata)
		if err != nil {
			return nil, fmt.Errorf("auth store: marshal metadata: %w", err)
		}
		return raw, nil
	}
	marshaler, ok := auth.Storage.(rootSafeTokenMarshaler)
	if !ok {
		return nil, fmt.Errorf("auth store: token storage %T does not implement root-safe MarshalToken", auth.Storage)
	}
	if isNilMarshaler(marshaler) {
		return nil, fmt.Errorf("auth store: token storage %T is nil", auth.Storage)
	}
	raw, err := marshaler.MarshalToken()
	if err != nil {
		return nil, fmt.Errorf("auth store: marshal token: %w", err)
	}
	if raw != nil && len(raw) == 0 {
		return nil, fmt.Errorf("auth store: token storage %T returned an empty payload", auth.Storage)
	}
	return raw, nil
}

func isNilMarshaler(marshaler rootSafeTokenMarshaler) bool {
	value := reflect.ValueOf(marshaler)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func setStoredAuthLocation(auth *switchailocalauth.Auth, id, filePath string) {
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes["path"] = filePath
	if strings.TrimSpace(auth.FileName) == "" {
		auth.FileName = id
	}
}

func writeStoredAuthAtomic(root *os.Root, id string, raw []byte) error {
	rootID := filepath.FromSlash(id)
	parent := filepath.Dir(rootID)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("auth store: create parent dirs: %w", err)
		}
		if err := secureStoredAuthParents(root, parent); err != nil {
			return err
		}
	}
	tempName, err := randomStoredAuthName(storeAtomicTempPrefix)
	if err != nil {
		return fmt.Errorf("auth store: create temp name: %w", err)
	}
	tempID := filepath.Join(parent, tempName)
	file, err := root.OpenFile(tempID, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("auth store: create temp file: %w", err)
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = root.Remove(tempID)
		}
	}()
	if _, err = file.Write(raw); err != nil {
		return fmt.Errorf("auth store: write temp file: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("auth store: sync temp file: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("auth store: close temp file: %w", err)
	}
	if err = root.Rename(tempID, rootID); err != nil {
		return fmt.Errorf("auth store: replace auth file: %w", err)
	}
	removeTemp = false
	return nil
}

func secureStoredAuthParents(root *os.Root, parent string) error {
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(parent), "/") {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, filepath.FromSlash(segment))
		if err := root.Chmod(current, 0o700); err != nil {
			return fmt.Errorf("auth store: secure parent dir %q: %w", filepath.ToSlash(current), err)
		}
	}
	return nil
}

func sweepStoredAuthTemps(ctx context.Context, root *os.Root, now time.Time) error {
	return fs.WalkDir(root.FS(), ".", func(id string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !isStoredAuthTempName(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < storeTempMaxAge {
			return nil
		}
		if err = root.Remove(filepath.FromSlash(id)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("auth store: remove stale temp %q: %w", id, err)
		}
		return nil
	})
}

func isStoredAuthTempName(name string) bool {
	if !strings.HasPrefix(name, storeAtomicTempPrefix) || len(name) != len(storeAtomicTempPrefix)+24 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(name, storeAtomicTempPrefix))
	return err == nil
}

func randomStoredAuthName(prefix string) (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(suffix[:]), nil
}
