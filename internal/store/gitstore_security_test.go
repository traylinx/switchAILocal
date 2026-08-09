package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	emptyauth "github.com/traylinx/switchAILocal/internal/auth/empty"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

type gitRecordingTokenStorage struct {
	raw       []byte
	err       error
	saveCalls int
}

func (s *gitRecordingTokenStorage) SaveTokenToFile(string) error {
	s.saveCalls++
	return errors.New("path-based serializer must not be called")
}

func (s *gitRecordingTokenStorage) MarshalToken() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.raw, nil
}

type gitLegacyOnlyTokenStorage struct{}

func (*gitLegacyOnlyTokenStorage) SaveTokenToFile(string) error { return nil }

func newSecurityTestGitStore(t *testing.T) (*GitTokenStore, string) {
	t.Helper()
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(t.TempDir(), "worktree")
	authDir := filepath.Join(repoDir, "auths")
	store := NewGitTokenStore(remoteDir, "", "")
	store.SetBaseDir(authDir)
	if err := store.EnsureRepository(); err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(authDir); err == nil {
		authDir = resolved
	}
	return store, authDir
}

func gitMetadataAuth(id, provider string) *switchailocalauth.Auth {
	return &switchailocalauth.Auth{ID: id, Metadata: map[string]any{"type": provider}}
}

func gitHeadFileState(t *testing.T, authDir, id string) (plumbing.Hash, bool) {
	t.Helper()
	repoRoot := filepath.Dir(authDir)
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(authDir, filepath.FromSlash(id))
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tree.File(filepath.ToSlash(rel))
	if err == nil {
		return head.Hash(), true
	}
	if errors.Is(err, object.ErrFileNotFound) {
		return head.Hash(), false
	}
	t.Fatal(err)
	return plumbing.ZeroHash, false
}

func TestGitTokenStoreNestedSaveListAndContainedAbsoluteDelete(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	auth := gitMetadataAuth("provider/tenant/token.json", "test")
	filePath, err := store.Save(context.Background(), auth)
	if err != nil {
		t.Fatal(err)
	}
	if filePath != filepath.Join(authDir, "provider", "tenant", "token.json") {
		t.Fatalf("Save path = %q", filePath)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	savedHead, tracked := gitHeadFileState(t, authDir, auth.ID)
	if !tracked {
		t.Fatal("saved credential is absent from git HEAD")
	}
	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "provider/tenant/token.json" || listed[0].Provider != "test" {
		t.Fatalf("List() = %#v", listed)
	}
	if err = store.Delete(context.Background(), filePath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("Delete left file: %v", err)
	}
	repo, err := git.PlainOpen(filepath.Dir(authDir))
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	status, err := worktree.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsClean() {
		t.Fatalf("git worktree dirty after Delete: %s", status.String())
	}
	deletedHead, tracked := gitHeadFileState(t, authDir, auth.ID)
	if tracked {
		t.Fatal("deleted credential remains in git HEAD")
	}
	if deletedHead == savedHead {
		t.Fatal("Delete did not advance git HEAD")
	}
}

func TestGitTokenStoreRejectsEscapesAndPortableAliases(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	original := []byte(`{"type":"outside"}`)
	if err := os.WriteFile(outside, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), &switchailocalauth.Auth{
		ID:         "safe.json",
		Attributes: map[string]string{"path": outside},
		Metadata:   map[string]any{"type": "bad"},
	}); err == nil {
		t.Fatal("outside absolute Save unexpectedly succeeded")
	}
	if err := store.Delete(context.Background(), outside); err == nil {
		t.Fatal("outside absolute Delete unexpectedly succeeded")
	}
	if _, err := store.Save(context.Background(), gitMetadataAuth("github/alice.json", "one")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), gitMetadataAuth("github/café.json", "one")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"github/Alice.json", "GitHub/bob.json", "github/cafe\u0301.json"} {
		if _, err := store.Save(context.Background(), gitMetadataAuth(id, "two")); err == nil {
			t.Fatalf("colliding Save(%q) unexpectedly succeeded", id)
		}
	}
	if err := store.Delete(context.Background(), "github/Alice.json"); err == nil {
		t.Fatal("alternate-spelling Delete unexpectedly succeeded")
	}
	raw, err := os.ReadFile(filepath.Join(authDir, "github", "alice.json"))
	if err != nil || !strings.Contains(string(raw), `"one"`) {
		t.Fatalf("exact credential changed: %q, %v", raw, err)
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != string(original) {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("rejected aliases created extra credentials: %#v", listed)
	}
}

func TestGitTokenStoreRejectsInvalidIDsWithoutMutation(t *testing.T) {
	for _, id := range []string{
		"../outside.json",
		"provider/../../outside.json",
		"provider/../outside.json",
		"./outside.json",
		"provider//outside.json",
		"provider\\outside.json",
		"provider/\x00.json",
		"provider/token.txt",
		"provider/.json",
		"",
		".",
		"..",
		"provider/",
		"provider/./token.json",
		"/absolute.json",
	} {
		t.Run(strings.ReplaceAll(id, "/", "_"), func(t *testing.T) {
			store, authDir := newSecurityTestGitStore(t)
			if _, err := store.Save(context.Background(), gitMetadataAuth(id, "bad")); err == nil {
				t.Fatalf("Save(%q) unexpectedly succeeded", id)
			}
			if err := store.Delete(context.Background(), id); err == nil {
				t.Fatalf("Delete(%q) unexpectedly succeeded", id)
			}
			listed, err := store.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(listed) != 0 {
				t.Fatalf("invalid ID %q left auth records: %#v", id, listed)
			}
			if _, err = os.Stat(filepath.Join(filepath.Dir(authDir), "outside.json")); !os.IsNotExist(err) {
				t.Fatalf("invalid ID %q mutated outside auth root: %v", id, err)
			}
		})
	}
}

func TestGitTokenStoreRejectsIntermediateSymlinks(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	outside := t.TempDir()
	real := filepath.Join(authDir, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"outside-link": outside, "inside-link": real} {
		if err := os.Symlink(target, filepath.Join(authDir, name)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		id := name + "/token.json"
		if _, err := store.Save(context.Background(), gitMetadataAuth(id, "bad")); err == nil {
			t.Fatalf("Save through %s unexpectedly succeeded", name)
		}
		if err := store.Delete(context.Background(), id); err == nil {
			t.Fatalf("Delete through %s unexpectedly succeeded", name)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "token.json")); !os.IsNotExist(err) {
		t.Fatalf("outside target created: %v", err)
	}
}

func TestGitTokenStoreFinalSymlinksStayContained(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	targetID := "provider/target.json"
	targetPath, err := store.Save(context.Background(), gitMetadataAuth(targetID, "target"))
	if err != nil {
		t.Fatal(err)
	}
	targetBefore, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(authDir, "provider", "link.json")
	if err = os.Symlink("target.json", linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err = store.Save(context.Background(), gitMetadataAuth("provider/link.json", "replacement")); err != nil {
		t.Fatal(err)
	}
	if info, errLstat := os.Lstat(linkPath); errLstat != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("contained final symlink was not atomically replaced: %v, %v", info, errLstat)
	}
	if targetAfter, errRead := os.ReadFile(targetPath); errRead != nil || string(targetAfter) != string(targetBefore) {
		t.Fatalf("contained symlink target changed: %q, %v", targetAfter, errRead)
	}

	outside := filepath.Join(t.TempDir(), "outside.json")
	outsideBefore := []byte(`{"type":"outside"}`)
	if err = os.WriteFile(outside, outsideBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	escapePath := filepath.Join(authDir, "provider", "escape.json")
	if err = os.Symlink(outside, escapePath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err = store.Save(context.Background(), gitMetadataAuth("provider/escape.json", "bad")); err == nil {
		t.Fatal("Save through escaping final symlink unexpectedly succeeded")
	}
	if got, errRead := os.ReadFile(outside); errRead != nil || string(got) != string(outsideBefore) {
		t.Fatalf("escaping symlink target changed: %q, %v", got, errRead)
	}
	if err = store.Delete(context.Background(), "provider/escape.json"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(escapePath); !os.IsNotExist(err) {
		t.Fatalf("final escaping symlink remains: %v", err)
	}
	if got, errRead := os.ReadFile(outside); errRead != nil || string(got) != string(outsideBefore) {
		t.Fatalf("Delete touched escaping symlink target: %q, %v", got, errRead)
	}
}

func TestGitTokenStoreRejectsFinalDirectory(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	dirPath := filepath.Join(authDir, "provider", "token.json")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), gitMetadataAuth("provider/token.json", "bad")); err == nil {
		t.Fatal("Save over final directory unexpectedly succeeded")
	}
	if err := store.Delete(context.Background(), "provider/token.json"); err == nil {
		t.Fatal("Delete of final directory unexpectedly succeeded")
	}
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Fatalf("final directory mutated: %v, %v", info, err)
	}
}

func TestGitTokenStoreDeleteCommitsAlreadyMissingTrackedFile(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	id := "provider/token.json"
	filePath, err := store.Save(context.Background(), gitMetadataAuth(id, "test"))
	if err != nil {
		t.Fatal(err)
	}
	if _, tracked := gitHeadFileState(t, authDir, id); !tracked {
		t.Fatal("precondition failed: credential was never tracked in git HEAD")
	}
	if err = os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainOpen(filepath.Dir(authDir))
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	status, err := worktree.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsClean() {
		t.Fatalf("already-missing tracked Delete left dirty worktree: %s", status.String())
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tree.File(filepath.ToSlash(filepath.Join("auths", filepath.FromSlash(id)))); err == nil {
		t.Fatal("already-missing credential remains in git HEAD")
	}
}

func TestGitTokenStoreUsesRootSafeMarshalerAndPreservesEmptySemantics(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	want := []byte("{\"type\":\"recording\",\"token\":\"secret\"}\n")
	storage := &gitRecordingTokenStorage{raw: want}
	filePath, err := store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/token.json",
		Storage: storage,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil || string(got) != string(want) {
		t.Fatalf("stored bytes = %q, %v; want %q", got, err, want)
	}
	if storage.saveCalls != 0 {
		t.Fatalf("path-based SaveTokenToFile called %d times", storage.saveCalls)
	}

	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/legacy.json",
		Storage: &gitLegacyOnlyTokenStorage{},
	}); err == nil || !strings.Contains(err.Error(), "root-safe MarshalToken") {
		t.Fatalf("path-only storage error = %v", err)
	}
	if _, err = os.Stat(filepath.Join(authDir, "provider", "legacy.json")); !os.IsNotExist(err) {
		t.Fatalf("path-only storage left file: %v", err)
	}
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/zero.json",
		Storage: &gitRecordingTokenStorage{raw: []byte{}},
	}); err == nil || !strings.Contains(err.Error(), "empty payload") {
		t.Fatalf("empty non-nil payload error = %v", err)
	}
	if _, err = os.Stat(filepath.Join(authDir, "provider", "zero.json")); !os.IsNotExist(err) {
		t.Fatalf("empty non-nil payload left file: %v", err)
	}
	wantMarshalErr := errors.New("serialize failed")
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/error.json",
		Storage: &gitRecordingTokenStorage{err: wantMarshalErr},
	}); !errors.Is(err, wantMarshalErr) {
		t.Fatalf("marshal error = %v; want %v", err, wantMarshalErr)
	}
	if _, err = os.Stat(filepath.Join(authDir, "provider", "error.json")); !os.IsNotExist(err) {
		t.Fatalf("marshal failure left file: %v", err)
	}
	providerEntries, err := os.ReadDir(filepath.Join(authDir, "provider"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range providerEntries {
		if isStoredAuthTempName(entry.Name()) {
			t.Fatalf("marshal failure left atomic temp %q", entry.Name())
		}
	}

	empty := &switchailocalauth.Auth{ID: "provider/empty.json", Storage: &emptyauth.EmptyStorage{}}
	emptyPath, err := store.Save(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	if emptyPath == "" || empty.Attributes["path"] != emptyPath {
		t.Fatalf("empty storage path state = %q, %#v", emptyPath, empty.Attributes)
	}
	if _, err = os.Stat(emptyPath); !os.IsNotExist(err) {
		t.Fatalf("empty storage materialized a file: %v", err)
	}
}

func TestGitTokenStoreSecuresPermissionsAndSweepsOnlyStaleAtomicTemps(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	if err := os.Chmod(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(authDir, "provider")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("a", 24))
	fresh := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("b", 24))
	unrelated := filepath.Join(nested, ".tmp-unrelated")
	for _, file := range []string{stale, fresh, unrelated} {
		if err := os.WriteFile(file, []byte("residue"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * storeTempMaxAge)
	for _, file := range []string{stale, unrelated} {
		if err := os.Chtimes(file, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Save(context.Background(), gitMetadataAuth("provider/token.json", "test")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(authDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("auth root mode = %v, %v", info, err)
	}
	if info, err := os.Stat(nested); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("nested auth dir mode = %v, %v", info, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale atomic temp remains: %v", err)
	}
	for _, file := range []string{fresh, unrelated} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("non-stale temp %q removed: %v", file, err)
		}
	}
	entries, err := os.ReadDir(nested)
	if err != nil {
		t.Fatal(err)
	}
	exactTemps := 0
	for _, entry := range entries {
		if isStoredAuthTempName(entry.Name()) {
			exactTemps++
		}
	}
	if exactTemps != 1 {
		t.Fatalf("exact atomic temp count = %d; want only fresh residue", exactTemps)
	}
	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "provider/token.json" {
		t.Fatalf("List surfaced temp residue: %#v", listed)
	}
}

func TestGitTokenStoreOverwriteRestoresCredentialMode(t *testing.T) {
	store, _ := newSecurityTestGitStore(t)
	id := "provider/token.json"
	filePath, err := store.Save(context.Background(), gitMetadataAuth(id, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(filePath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(context.Background(), gitMetadataAuth(id, "second")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("overwritten credential mode = %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(filePath)
	if err != nil || !strings.Contains(string(raw), `"second"`) {
		t.Fatalf("overwritten credential payload = %q, %v", raw, err)
	}
}

func TestGitTokenStoreEnsureRepositoryRecreatesMissingAuthDirBeforeSave(t *testing.T) {
	store, authDir := newSecurityTestGitStore(t)
	if err := os.RemoveAll(authDir); err != nil {
		t.Fatal(err)
	}
	filePath, err := store.Save(context.Background(), gitMetadataAuth("provider/token.json", "test"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filePath); err != nil {
		t.Fatalf("Save did not recreate auth directory: %v", err)
	}
}

func TestGitTokenStoreHonorsCanceledContextBeforeRepositoryWork(t *testing.T) {
	store := NewGitTokenStore(filepath.Join(t.TempDir(), "missing.git"), "", "")
	store.SetBaseDir(filepath.Join(t.TempDir(), "worktree", "auths"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, call := range map[string]func() error{
		"save": func() error {
			_, err := store.Save(ctx, gitMetadataAuth("token.json", "test"))
			return err
		},
		"list": func() error {
			_, err := store.List(ctx)
			return err
		},
		"delete": func() error {
			return store.Delete(ctx, "token.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v; want context.Canceled", err)
			}
		})
	}
}
