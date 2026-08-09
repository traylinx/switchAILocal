// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	gitindex "github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/traylinx/switchAILocal/internal/authid"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// GitTokenStore persists token records and auth metadata using git as the backing storage.
type GitTokenStore struct {
	mu        sync.Mutex
	dirLock   sync.RWMutex
	baseDir   string
	repoDir   string
	configDir string
	remote    string
	username  string
	password  string
}

// NewGitTokenStore creates a token store that saves credentials to disk through the
// TokenStorage implementation embedded in the token record.
func NewGitTokenStore(remote, username, password string) *GitTokenStore {
	return &GitTokenStore{
		remote:   remote,
		username: username,
		password: password,
	}
}

// SetBaseDir updates the default directory used for auth JSON persistence when no explicit path is provided.
func (s *GitTokenStore) SetBaseDir(dir string) {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		s.dirLock.Lock()
		s.baseDir = ""
		s.repoDir = ""
		s.configDir = ""
		s.dirLock.Unlock()
		return
	}
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	repoDir := filepath.Dir(clean)
	if repoDir == "" || repoDir == "." {
		repoDir = clean
	}
	configDir := filepath.Join(repoDir, "config")
	s.dirLock.Lock()
	s.baseDir = clean
	s.repoDir = repoDir
	s.configDir = configDir
	s.dirLock.Unlock()
}

// AuthDir returns the directory used for auth persistence.
func (s *GitTokenStore) AuthDir() string {
	return s.baseDirSnapshot()
}

// ConfigPath returns the managed config file path.
func (s *GitTokenStore) ConfigPath() string {
	s.dirLock.RLock()
	defer s.dirLock.RUnlock()
	if s.configDir == "" {
		return ""
	}
	return filepath.Join(s.configDir, "config.yaml")
}

// EnsureRepository prepares the local git working tree by cloning or opening the repository.
func (s *GitTokenStore) EnsureRepository() error {
	s.dirLock.Lock()
	if s.remote == "" {
		s.dirLock.Unlock()
		return fmt.Errorf("git token store: remote not configured")
	}
	if s.baseDir == "" {
		s.dirLock.Unlock()
		return fmt.Errorf("git token store: base directory not configured")
	}
	repoDir := s.repoDir
	if repoDir == "" {
		repoDir = filepath.Dir(s.baseDir)
		if repoDir == "" || repoDir == "." {
			repoDir = s.baseDir
		}
		s.repoDir = repoDir
	}
	if s.configDir == "" {
		s.configDir = filepath.Join(repoDir, "config")
	}
	authDir := filepath.Join(repoDir, "auths")
	configDir := filepath.Join(repoDir, "config")
	gitDir := filepath.Join(repoDir, ".git")
	authMethod := s.gitAuth()
	var initPaths []string
	if _, err := os.Stat(gitDir); errors.Is(err, fs.ErrNotExist) {
		if errMk := os.MkdirAll(repoDir, 0o700); errMk != nil {
			s.dirLock.Unlock()
			return fmt.Errorf("git token store: create repo dir: %w", errMk)
		}
		if _, errClone := git.PlainClone(repoDir, &git.CloneOptions{Auth: authMethod, URL: s.remote}); errClone != nil {
			if errors.Is(errClone, transport.ErrEmptyRemoteRepository) {
				_ = os.RemoveAll(gitDir)
				repo, errInit := git.PlainInit(repoDir, false)
				if errInit != nil {
					s.dirLock.Unlock()
					return fmt.Errorf("git token store: init empty repo: %w", errInit)
				}
				if _, errRemote := repo.Remote("origin"); errRemote != nil {
					if _, errCreate := repo.CreateRemote(&config.RemoteConfig{
						Name: "origin",
						URLs: []string{s.remote},
					}); errCreate != nil && !errors.Is(errCreate, git.ErrRemoteExists) {
						s.dirLock.Unlock()
						return fmt.Errorf("git token store: configure remote: %w", errCreate)
					}
				}
				if err := os.MkdirAll(authDir, 0o700); err != nil {
					s.dirLock.Unlock()
					return fmt.Errorf("git token store: create auth dir: %w", err)
				}
				if err := os.MkdirAll(configDir, 0o700); err != nil {
					s.dirLock.Unlock()
					return fmt.Errorf("git token store: create config dir: %w", err)
				}
				if err := ensureEmptyFile(filepath.Join(authDir, ".gitkeep")); err != nil {
					s.dirLock.Unlock()
					return fmt.Errorf("git token store: create auth placeholder: %w", err)
				}
				if err := ensureEmptyFile(filepath.Join(configDir, ".gitkeep")); err != nil {
					s.dirLock.Unlock()
					return fmt.Errorf("git token store: create config placeholder: %w", err)
				}
				initPaths = []string{
					filepath.Join("auths", ".gitkeep"),
					filepath.Join("config", ".gitkeep"),
				}
			} else {
				s.dirLock.Unlock()
				return fmt.Errorf("git token store: clone remote: %w", errClone)
			}
		}
	} else if err != nil {
		s.dirLock.Unlock()
		return fmt.Errorf("git token store: stat repo: %w", err)
	} else {
		repo, errOpen := git.PlainOpen(repoDir)
		if errOpen != nil {
			s.dirLock.Unlock()
			return fmt.Errorf("git token store: open repo: %w", errOpen)
		}
		worktree, errWorktree := repo.Worktree()
		if errWorktree != nil {
			s.dirLock.Unlock()
			return fmt.Errorf("git token store: worktree: %w", errWorktree)
		}
		if errPull := worktree.Pull(&git.PullOptions{Auth: authMethod, RemoteName: "origin"}); errPull != nil {
			switch {
			case errors.Is(errPull, git.NoErrAlreadyUpToDate),
				errors.Is(errPull, git.ErrUnstagedChanges),
				errors.Is(errPull, git.ErrNonFastForwardUpdate):
				// Ignore clean syncs, local edits, and remote divergence—local changes win.
			case errors.Is(errPull, transport.ErrAuthenticationRequired),
				errors.Is(errPull, plumbing.ErrReferenceNotFound),
				errors.Is(errPull, transport.ErrEmptyRemoteRepository):
				// Ignore authentication prompts and empty remote references on initial sync.
			default:
				s.dirLock.Unlock()
				return fmt.Errorf("git token store: pull: %w", errPull)
			}
		}
	}
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		s.dirLock.Unlock()
		return fmt.Errorf("git token store: create auth dir: %w", err)
	}
	if err := os.MkdirAll(s.configDir, 0o700); err != nil {
		s.dirLock.Unlock()
		return fmt.Errorf("git token store: create config dir: %w", err)
	}
	s.dirLock.Unlock()
	if len(initPaths) > 0 {
		s.mu.Lock()
		err := s.commitAndPushLocked("Initialize git token store", initPaths...)
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// Save persists token storage and metadata to the resolved auth file path.
func (s *GitTokenStore) Save(ctx context.Context, auth *switchailocalauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("auth filestore: auth is nil")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := s.EnsureRepository(); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	baseDir := s.baseDirSnapshot()
	if baseDir == "" {
		return "", fmt.Errorf("auth filestore: directory not configured")
	}
	if err := os.Chmod(baseDir, 0o700); err != nil {
		return "", fmt.Errorf("auth filestore: secure auth dir: %w", err)
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return "", fmt.Errorf("auth filestore: open root: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err = sweepStoredAuthTemps(ctx, root, time.Now()); err != nil {
		return "", err
	}
	id, err := resolveStoredAuthID(auth, baseDir)
	if err != nil {
		return "", err
	}
	exists, err := validateStoredAuthPath(root, id)
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

	raw, err := marshalStoredAuth(auth)
	if err != nil {
		return "", err
	}
	if auth.Metadata != nil && auth.Storage == nil {
		if existing, errRead := root.ReadFile(filepath.FromSlash(id)); errRead == nil {
			if jsonEqual(existing, raw) {
				setStoredAuthLocation(auth, id, filePath)
				return filePath, nil
			}
		} else if !os.IsNotExist(errRead) {
			return "", fmt.Errorf("auth filestore: read existing failed: %w", errRead)
		}
	}
	if raw != nil {
		if err = writeStoredAuthAtomic(root, id, raw); err != nil {
			return "", err
		}
	}
	setStoredAuthLocation(auth, id, filePath)
	if raw == nil {
		return filePath, nil
	}

	relPath, errRel := s.relativeToRepo(filePath)
	if errRel != nil {
		return "", errRel
	}
	if errCommit := s.commitAndPushLocked(fmt.Sprintf("Update auth %s", id), relPath); errCommit != nil {
		return "", errCommit
	}

	return filePath, nil
}

// List enumerates all auth JSON files under the configured directory.
func (s *GitTokenStore) List(ctx context.Context) ([]*switchailocalauth.Auth, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.EnsureRepository(); err != nil {
		return nil, err
	}
	baseDir := s.baseDirSnapshot()
	if baseDir == "" {
		return nil, fmt.Errorf("auth filestore: directory not configured")
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, fmt.Errorf("auth filestore: open root: %w", err)
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
func (s *GitTokenStore) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("auth filestore: id is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.EnsureRepository(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	baseDir := s.baseDirSnapshot()
	if baseDir == "" {
		return fmt.Errorf("auth filestore: directory not configured")
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return fmt.Errorf("auth filestore: open root: %w", err)
	}
	defer func() { _ = root.Close() }()
	canonicalID, err := canonicalStoredAuthID(baseDir, id)
	if err != nil {
		return fmt.Errorf("auth filestore: invalid delete id: %w", err)
	}
	exists, err := validateStoredAuthPath(root, canonicalID)
	if err != nil {
		return err
	}
	filePath, err := authid.ToFSPath(baseDir, canonicalID)
	if err != nil {
		return err
	}
	if !exists {
		rel, errRel := s.relativeToRepo(filePath)
		if errRel != nil {
			return errRel
		}
		return s.commitAndPushLocked(fmt.Sprintf("Delete auth %s", canonicalID), rel)
	}
	if err = root.Remove(filepath.FromSlash(canonicalID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("auth filestore: delete failed: %w", err)
	}
	if err == nil {
		rel, errRel := s.relativeToRepo(filePath)
		if errRel != nil {
			return errRel
		}
		if errCommit := s.commitAndPushLocked(fmt.Sprintf("Delete auth %s", canonicalID), rel); errCommit != nil {
			return errCommit
		}
	}
	return nil
}

// PersistAuthFiles commits and pushes the provided paths to the remote repository.
// It no-ops when the store is not fully configured or when there are no paths.
func (s *GitTokenStore) PersistAuthFiles(_ context.Context, message string, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	if err := s.EnsureRepository(); err != nil {
		return err
	}

	filtered := make([]string, 0, len(paths))
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		rel, err := s.relativeToRepo(trimmed)
		if err != nil {
			return err
		}
		filtered = append(filtered, rel)
	}
	if len(filtered) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(message) == "" {
		message = "Sync watcher updates"
	}
	return s.commitAndPushLocked(message, filtered...)
}

func (s *GitTokenStore) readAuthFile(root *os.Root, baseDir, id string) (*switchailocalauth.Auth, error) {
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

func (s *GitTokenStore) labelFor(metadata map[string]any) string {
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

func (s *GitTokenStore) baseDirSnapshot() string {
	s.dirLock.RLock()
	defer s.dirLock.RUnlock()
	return s.baseDir
}

func (s *GitTokenStore) repoDirSnapshot() string {
	s.dirLock.RLock()
	defer s.dirLock.RUnlock()
	return s.repoDir
}

func (s *GitTokenStore) gitAuth() transport.AuthMethod {
	if s.username == "" && s.password == "" {
		return nil
	}
	user := s.username
	if user == "" {
		user = "git"
	}
	return &http.BasicAuth{Username: user, Password: s.password}
}

func (s *GitTokenStore) relativeToRepo(path string) (string, error) {
	repoDir := s.repoDirSnapshot()
	if repoDir == "" {
		return "", fmt.Errorf("git token store: repository path not configured")
	}
	absRepo := repoDir
	if abs, err := filepath.Abs(repoDir); err == nil {
		absRepo = abs
	}
	// authid.ToFSPath resolves existing parent symlinks before returning a path.
	// Canonicalize the repository root the same way so macOS's /var ->
	// /private/var alias (and equivalent trusted-root aliases) cannot turn a
	// contained auth path into a false "outside repository" result.
	if resolved, err := filepath.EvalSymlinks(absRepo); err == nil {
		absRepo = resolved
	}
	cleanPath := path
	if abs, err := filepath.Abs(path); err == nil {
		cleanPath = abs
	}
	rel, err := filepath.Rel(absRepo, cleanPath)
	if err != nil {
		return "", fmt.Errorf("git token store: relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("git token store: path outside repository")
	}
	return rel, nil
}

func (s *GitTokenStore) commitAndPushLocked(message string, relPaths ...string) error {
	repoDir := s.repoDirSnapshot()
	if repoDir == "" {
		return fmt.Errorf("git token store: repository path not configured")
	}
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("git token store: open repo: %w", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("git token store: worktree: %w", err)
	}
	added := false
	for _, rel := range relPaths {
		if strings.TrimSpace(rel) == "" {
			continue
		}
		if _, err = worktree.Add(rel); err != nil {
			if errors.Is(err, gitindex.ErrEntryNotFound) {
				// The path was never tracked (for example, an injected final
				// symlink removed safely by Delete), so there is no git mutation
				// to stage.
				diskPath := filepath.Join(repoDir, filepath.FromSlash(rel))
				if _, statErr := os.Lstat(diskPath); statErr == nil {
					return fmt.Errorf("git token store: add %s: %w", rel, err)
				} else if !os.IsNotExist(statErr) {
					return fmt.Errorf("git token store: inspect %s: %w", rel, statErr)
				}
				continue
			}
			if errors.Is(err, os.ErrNotExist) {
				if _, errRemove := worktree.Remove(rel); errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
					return fmt.Errorf("git token store: remove %s: %w", rel, errRemove)
				}
			} else {
				return fmt.Errorf("git token store: add %s: %w", rel, err)
			}
		}
		added = true
	}
	if !added {
		return nil
	}
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("git token store: status: %w", err)
	}
	if status.IsClean() {
		return nil
	}
	if strings.TrimSpace(message) == "" {
		message = "Update auth store"
	}
	signature := &object.Signature{
		Name:  "switchAILocal",
		Email: "switchailocal@local",
		When:  time.Now(),
	}
	commitHash, err := worktree.Commit(message, &git.CommitOptions{
		Author: signature,
	})
	if err != nil {
		if errors.Is(err, git.ErrEmptyCommit) {
			return nil
		}
		return fmt.Errorf("git token store: commit: %w", err)
	}
	headRef, errHead := repo.Head()
	if errHead != nil {
		if !errors.Is(errHead, plumbing.ErrReferenceNotFound) {
			return fmt.Errorf("git token store: get head: %w", errHead)
		}
	} else if errRewrite := s.rewriteHeadAsSingleCommit(repo, headRef.Name(), commitHash, message, signature); errRewrite != nil {
		return errRewrite
	}
	if err = repo.Push(&git.PushOptions{Auth: s.gitAuth(), Force: true}); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return fmt.Errorf("git token store: push: %w", err)
	}
	return nil
}

// rewriteHeadAsSingleCommit rewrites the current branch tip to a single-parentless commit and leaves history squashed.
func (s *GitTokenStore) rewriteHeadAsSingleCommit(repo *git.Repository, branch plumbing.ReferenceName, commitHash plumbing.Hash, message string, signature *object.Signature) error {
	commitObj, err := repo.CommitObject(commitHash)
	if err != nil {
		return fmt.Errorf("git token store: inspect head commit: %w", err)
	}
	squashed := &object.Commit{
		Author:       *signature,
		Committer:    *signature,
		Message:      message,
		TreeHash:     commitObj.TreeHash,
		ParentHashes: nil,
		Encoding:     commitObj.Encoding,
		ExtraHeaders: commitObj.ExtraHeaders,
	}
	mem := &plumbing.MemoryObject{}
	mem.SetType(plumbing.CommitObject)
	if err := squashed.Encode(mem); err != nil {
		return fmt.Errorf("git token store: encode squashed commit: %w", err)
	}
	newHash, err := repo.Storer.SetEncodedObject(mem)
	if err != nil {
		return fmt.Errorf("git token store: write squashed commit: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branch, newHash)); err != nil {
		return fmt.Errorf("git token store: update branch reference: %w", err)
	}
	return nil
}

// PersistConfig commits and pushes configuration changes to git.
func (s *GitTokenStore) PersistConfig(_ context.Context) error {
	if err := s.EnsureRepository(); err != nil {
		return err
	}
	configPath := s.ConfigPath()
	if configPath == "" {
		return fmt.Errorf("git token store: config path not configured")
	}
	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("git token store: stat config: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, err := s.relativeToRepo(configPath)
	if err != nil {
		return err
	}
	return s.commitAndPushLocked("Update config", rel)
}

func ensureEmptyFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return os.WriteFile(path, []byte{}, 0o600)
		}
		return err
	}
	return nil
}

func jsonEqual(a, b []byte) bool {
	var objA any
	var objB any
	if err := json.Unmarshal(a, &objA); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &objB); err != nil {
		return false
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
