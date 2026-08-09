// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

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
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/authid"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

const (
	fileStoreAtomicTempPrefix = ".tmp-"
	fileStoreTempMaxAge       = time.Hour
)

// TokenMarshaler serializes token storage without performing path-based I/O.
// FileTokenStore requires this interface so credential writes remain confined
// to its os.Root boundary. A nil byte slice means the storage intentionally has
// no file payload, preserving no-op storage implementations.
type TokenMarshaler interface {
	MarshalToken() ([]byte, error)
}

// FileTokenStore persists token records and auth metadata using the filesystem as backing storage.
type FileTokenStore struct {
	mu      sync.Mutex
	dirLock sync.RWMutex
	baseDir string
}

// NewFileTokenStore creates a token store that saves credentials to disk through the
// TokenStorage implementation embedded in the token record.
func NewFileTokenStore() *FileTokenStore {
	return &FileTokenStore{}
}

// SetBaseDir updates the default directory used for auth JSON persistence when no explicit path is provided.
func (s *FileTokenStore) SetBaseDir(dir string) {
	s.dirLock.Lock()
	s.baseDir = strings.TrimSpace(dir)
	s.dirLock.Unlock()
}

// Save persists token storage and metadata to the resolved auth file path.
func (s *FileTokenStore) Save(ctx context.Context, auth *switchailocalauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("auth filestore: auth is nil")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	baseDir := s.baseDirSnapshot()
	if baseDir == "" {
		return "", fmt.Errorf("auth filestore: directory not configured")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return "", fmt.Errorf("auth filestore: create dir failed: %w", err)
	}
	if err := os.Chmod(baseDir, 0o700); err != nil {
		return "", fmt.Errorf("auth filestore: secure root dir failed: %w", err)
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return "", fmt.Errorf("auth filestore: open root failed: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err = sweepStaleStoreTemps(ctx, root, time.Now()); err != nil {
		return "", err
	}

	id, err := s.resolveAuthID(auth, baseDir)
	if err != nil {
		return "", err
	}
	exists, err := validateStorePath(root, id)
	if err != nil {
		return "", err
	}
	if auth.Disabled && !exists {
		return "", nil
	}

	filePath, err := authid.ToFSPath(baseDir, id)
	if err != nil {
		return "", fmt.Errorf("auth filestore: resolve auth id: %w", err)
	}

	var raw []byte
	switch {
	case auth.Storage != nil:
		marshaler, ok := auth.Storage.(TokenMarshaler)
		if !ok {
			return "", fmt.Errorf("auth filestore: token storage %T does not implement root-safe MarshalToken", auth.Storage)
		}
		if isNilTokenMarshaler(marshaler) {
			return "", fmt.Errorf("auth filestore: token storage %T is nil", auth.Storage)
		}
		raw, err = marshaler.MarshalToken()
		if err != nil {
			return "", fmt.Errorf("auth filestore: marshal token: %w", err)
		}
	case auth.Metadata != nil:
		raw, err = json.Marshal(auth.Metadata)
		if err != nil {
			return "", fmt.Errorf("auth filestore: marshal metadata failed: %w", err)
		}
		if existing, errRead := root.ReadFile(filepath.FromSlash(id)); errRead == nil {
			// Use metadataEqualIgnoringTimestamps to skip writes when only timestamp fields change.
			// This prevents the token refresh loop caused by timestamp/expired/expires_in changes.
			if metadataEqualIgnoringTimestamps(existing, raw) {
				setStoredAuthPath(auth, id, filePath)
				return filePath, nil
			}
		} else if !os.IsNotExist(errRead) {
			return "", fmt.Errorf("auth filestore: read existing failed: %w", errRead)
		}
	default:
		return "", fmt.Errorf("auth filestore: nothing to persist for %s", auth.ID)
	}

	if raw != nil {
		if err = writeRootFileAtomic(root, id, raw); err != nil {
			return "", err
		}
	}

	setStoredAuthPath(auth, id, filePath)

	return filePath, nil
}

func setStoredAuthPath(auth *switchailocalauth.Auth, id, filePath string) {
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes["path"] = filePath
	if strings.TrimSpace(auth.FileName) == "" {
		auth.FileName = id
	}
}

func isNilTokenMarshaler(marshaler TokenMarshaler) bool {
	value := reflect.ValueOf(marshaler)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func writeRootFileAtomic(root *os.Root, id string, raw []byte) error {
	rootID := filepath.FromSlash(id)
	parent := filepath.Dir(rootID)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("auth filestore: create parent dirs: %w", err)
		}
		if err := secureRootParents(root, parent); err != nil {
			return err
		}
	}
	tempName, err := randomStoreName(fileStoreAtomicTempPrefix)
	if err != nil {
		return fmt.Errorf("auth filestore: create temp name: %w", err)
	}
	tempID := filepath.Join(parent, tempName)
	f, err := root.OpenFile(tempID, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("auth filestore: create temp file: %w", err)
	}
	removeTemp := true
	defer func() {
		_ = f.Close()
		if removeTemp {
			_ = root.Remove(tempID)
		}
	}()
	if _, err = f.Write(raw); err != nil {
		return fmt.Errorf("auth filestore: write temp file: %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("auth filestore: sync temp file: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("auth filestore: close temp file: %w", err)
	}
	if err = root.Rename(tempID, rootID); err != nil {
		return fmt.Errorf("auth filestore: replace auth file: %w", err)
	}
	removeTemp = false
	return nil
}

func secureRootParents(root *os.Root, parent string) error {
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(parent), "/") {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, filepath.FromSlash(segment))
		if err := root.Chmod(current, 0o700); err != nil {
			return fmt.Errorf("auth filestore: secure parent dir %q: %w", filepath.ToSlash(current), err)
		}
	}
	return nil
}

func randomStoreName(prefix string) (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(suffix[:]), nil
}

func sweepStaleStoreTemps(ctx context.Context, root *os.Root, now time.Time) error {
	return fs.WalkDir(root.FS(), ".", func(id string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !isAtomicStoreTempName(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < fileStoreTempMaxAge {
			return nil
		}
		if err = root.Remove(filepath.FromSlash(id)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("auth filestore: remove stale temp %q: %w", id, err)
		}
		return nil
	})
}

func isAtomicStoreTempName(name string) bool {
	if !strings.HasPrefix(name, fileStoreAtomicTempPrefix) || len(name) != len(fileStoreAtomicTempPrefix)+24 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(name, fileStoreAtomicTempPrefix))
	return err == nil
}

// List enumerates all auth JSON files under the configured directory.
func (s *FileTokenStore) List(ctx context.Context) ([]*switchailocalauth.Auth, error) {
	baseDir := s.baseDirSnapshot()
	if baseDir == "" {
		return nil, fmt.Errorf("auth filestore: directory not configured")
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, fmt.Errorf("auth filestore: open root failed: %w", err)
	}
	defer func() { _ = root.Close() }()

	entries := make([]*switchailocalauth.Auth, 0)
	err = fs.WalkDir(root.FS(), ".", func(id string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if errContext := ctx.Err(); errContext != nil {
			return errContext
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}
		if errValidate := authid.Validate(id); errValidate != nil {
			return nil
		}
		auth, errRead := s.readAuthFile(root, baseDir, id)
		if errRead != nil {
			return nil
		}
		if auth != nil {
			entries = append(entries, auth)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// Delete removes the auth file.
func (s *FileTokenStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("auth filestore: id is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	baseDir := s.baseDirSnapshot()
	if baseDir == "" {
		return fmt.Errorf("auth filestore: directory not configured")
	}
	root, err := os.OpenRoot(baseDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth filestore: open root failed: %w", err)
	}
	defer func() { _ = root.Close() }()
	canonicalID, err := canonicalStoreID(baseDir, id)
	if err != nil {
		return fmt.Errorf("auth filestore: invalid delete id: %w", err)
	}

	exists, err := validateStorePath(root, canonicalID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err = root.Remove(filepath.FromSlash(canonicalID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("auth filestore: delete failed: %w", err)
	}
	return nil
}

func canonicalStoreID(baseDir, candidate string) (string, error) {
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
	if !strings.HasSuffix(strings.ToLower(path.Base(id)), ".json") {
		return "", fmt.Errorf("auth id must end in .json")
	}
	return id, nil
}

func validateStorePath(root *os.Root, id string) (bool, error) {
	parts := strings.Split(id, "/")
	parent := "."
	for i, segment := range parts {
		entries, err := fs.ReadDir(root.FS(), parent)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("auth filestore: inspect %q: %w", parent, err)
		}
		candidateKey, err := authid.FoldKey(segment)
		if err != nil {
			return false, fmt.Errorf("auth filestore: invalid id segment %q: %w", segment, err)
		}
		var exact fs.DirEntry
		for _, entry := range entries {
			if entry.Name() == segment {
				exact = entry
				continue
			}
			existingKey, errKey := authid.FoldKey(entry.Name())
			if errKey == nil && existingKey == candidateKey {
				return false, fmt.Errorf("auth filestore: id segment %q collides with existing %q", segment, entry.Name())
			}
		}
		if exact == nil {
			return false, nil
		}
		if i < len(parts)-1 {
			if exact.Type()&fs.ModeSymlink != 0 {
				return false, fmt.Errorf("auth filestore: intermediate id segment %q is a symlink", segment)
			}
			if !exact.IsDir() {
				return false, fmt.Errorf("auth filestore: intermediate id segment %q is not a directory", segment)
			}
			parent = path.Join(parent, segment)
		}
	}
	return true, nil
}

func (s *FileTokenStore) readAuthFile(root *os.Root, baseDir, id string) (*switchailocalauth.Auth, error) {
	rootID := filepath.FromSlash(id)
	data, err := root.ReadFile(rootID)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	metadata := make(map[string]any)
	if err = json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("unmarshal auth json: %w", err)
	}
	provider, _ := metadata["type"].(string)
	if provider == "" {
		provider = "unknown"
	}
	info, err := root.Stat(rootID)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	filePath, err := authid.ToFSPath(baseDir, id)
	if err != nil {
		return nil, fmt.Errorf("resolve file path: %w", err)
	}
	auth := &switchailocalauth.Auth{
		ID:               id,
		Provider:         provider,
		FileName:         id,
		Label:            s.labelFor(metadata),
		Status:           switchailocalauth.StatusActive,
		Attributes:       map[string]string{"path": filePath},
		Metadata:         metadata,
		CreatedAt:        info.ModTime(),
		UpdatedAt:        info.ModTime(),
		LastRefreshedAt:  time.Time{},
		NextRefreshAfter: time.Time{},
	}
	if email, ok := metadata["email"].(string); ok && email != "" {
		auth.Attributes["email"] = email
	}
	return auth, nil
}

func (s *FileTokenStore) resolveAuthID(auth *switchailocalauth.Auth, baseDir string) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("auth filestore: auth is nil")
	}
	if auth.Attributes != nil {
		if candidate := auth.Attributes["path"]; candidate != "" {
			id, err := canonicalStoreID(baseDir, candidate)
			if err != nil {
				return "", fmt.Errorf("auth filestore: invalid path attribute: %w", err)
			}
			return id, nil
		}
	}
	if auth.FileName != "" {
		id, err := canonicalStoreID(baseDir, auth.FileName)
		if err != nil {
			return "", fmt.Errorf("auth filestore: invalid file name: %w", err)
		}
		return id, nil
	}
	if auth.ID == "" {
		return "", fmt.Errorf("auth filestore: missing id")
	}
	id, err := canonicalStoreID(baseDir, auth.ID)
	if err != nil {
		return "", fmt.Errorf("auth filestore: invalid id: %w", err)
	}
	return id, nil
}

func (s *FileTokenStore) labelFor(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["label"].(string); ok && v != "" {
		return v
	}
	if v, ok := metadata["email"].(string); ok && v != "" {
		return v
	}
	if project, ok := metadata["project_id"].(string); ok && project != "" {
		return project
	}
	return ""
}

func (s *FileTokenStore) baseDirSnapshot() string {
	s.dirLock.RLock()
	defer s.dirLock.RUnlock()
	return s.baseDir
}

// metadataEqualIgnoringTimestamps compares two metadata JSON blobs,
// ignoring fields that change on every refresh but don't affect functionality.
// This prevents unnecessary file writes that would trigger watcher events and
// create refresh loops.
func metadataEqualIgnoringTimestamps(a, b []byte) bool {
	var objA, objB map[string]any
	if err := json.Unmarshal(a, &objA); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &objB); err != nil {
		return false
	}

	// Fields to ignore: these change on every refresh but don't affect authentication logic.
	// - timestamp, expired, expires_in, last_refresh: time-related fields that change on refresh
	// - access_token: Google OAuth returns a new access_token on each refresh, this is expected
	//   and shouldn't trigger file writes (the new token will be fetched again when needed)
	ignoredFields := []string{"timestamp", "expired", "expires_in", "last_refresh", "access_token"}
	for _, field := range ignoredFields {
		delete(objA, field)
		delete(objB, field)
	}

	return deepEqualJSON(objA, objB)
}

func deepEqualJSON(a, b any) bool {
	switch valA := a.(type) {
	case map[string]any:
		valB, ok := b.(map[string]any)
		if !ok || len(valA) != len(valB) {
			return false
		}
		for key, subA := range valA {
			subB, ok1 := valB[key]
			if !ok1 || !deepEqualJSON(subA, subB) {
				return false
			}
		}
		return true
	case []any:
		sliceB, ok := b.([]any)
		if !ok || len(valA) != len(sliceB) {
			return false
		}
		for i := range valA {
			if !deepEqualJSON(valA[i], sliceB[i]) {
				return false
			}
		}
		return true
	case float64:
		valB, ok := b.(float64)
		if !ok {
			return false
		}
		return valA == valB
	case string:
		valB, ok := b.(string)
		if !ok {
			return false
		}
		return valA == valB
	case bool:
		valB, ok := b.(bool)
		if !ok {
			return false
		}
		return valA == valB
	case nil:
		return b == nil
	default:
		return false
	}
}
