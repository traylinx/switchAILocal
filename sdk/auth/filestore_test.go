package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	claudeauth "github.com/traylinx/switchAILocal/internal/auth/claude"
	codexauth "github.com/traylinx/switchAILocal/internal/auth/codex"
	emptyauth "github.com/traylinx/switchAILocal/internal/auth/empty"
	geminiauth "github.com/traylinx/switchAILocal/internal/auth/gemini"
	iflowauth "github.com/traylinx/switchAILocal/internal/auth/iflow"
	qwenauth "github.com/traylinx/switchAILocal/internal/auth/qwen"
	vertexauth "github.com/traylinx/switchAILocal/internal/auth/vertex"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

type recordingTokenStorage struct {
	raw       []byte
	err       error
	saveCalls int
}

func (s *recordingTokenStorage) SaveTokenToFile(string) error {
	s.saveCalls++
	return errors.New("path-based serializer must not be called")
}

func (s *recordingTokenStorage) MarshalToken() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.raw, nil
}

func newTestFileStore(t *testing.T) (*FileTokenStore, string) {
	t.Helper()
	root := t.TempDir()
	store := NewFileTokenStore()
	store.SetBaseDir(root)
	return store, root
}

func metadataAuth(id string, metadata map[string]any) *switchailocalauth.Auth {
	return &switchailocalauth.Auth{ID: id, Metadata: metadata}
}

func TestFileTokenStoreNestedSaveListAndContainedAbsoluteDelete(t *testing.T) {
	store, root := newTestFileStore(t)
	auth := metadataAuth("provider/tenant/token.json", map[string]any{
		"type":  "test",
		"email": "user@example.com",
	})
	gotPath, err := store.Save(context.Background(), auth)
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(realRoot, "provider", "tenant", "token.json")
	if gotPath != wantPath || auth.Attributes["path"] != wantPath || auth.FileName != "provider/tenant/token.json" {
		t.Fatalf("saved path state = %q, %q, %q; want %q", gotPath, auth.Attributes["path"], auth.FileName, wantPath)
	}
	auth.Metadata["type"] = "test-updated"
	if _, err = store.Save(context.Background(), auth); err != nil {
		t.Fatalf("contained absolute path attribute rejected: %v", err)
	}
	deduplicated := metadataAuth("provider/tenant/token.json", map[string]any{
		"type":  "test-updated",
		"email": "user@example.com",
	})
	if _, err = store.Save(context.Background(), deduplicated); err != nil {
		t.Fatal(err)
	}
	if deduplicated.Attributes["path"] != wantPath || deduplicated.FileName != "provider/tenant/token.json" {
		t.Fatalf("deduplicated Save lost path state: %#v", deduplicated)
	}
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth file mode = %o, want 600", info.Mode().Perm())
	}
	for _, dir := range []string{filepath.Join(realRoot, "provider"), filepath.Join(realRoot, "provider", "tenant")} {
		info, err = os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("auth dir %q mode = %o, want 700", dir, info.Mode().Perm())
		}
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "provider/tenant/token.json" || listed[0].Provider != "test-updated" {
		t.Fatalf("List() = %#v", listed)
	}
	if err = store.Delete(context.Background(), gotPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("contained absolute delete left file: %v", err)
	}
}

func TestFileTokenStoreTightensExistingRootAndParentPermissions(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "auths")
	provider := filepath.Join(root, "provider")
	if err := os.MkdirAll(provider, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(provider, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewFileTokenStore()
	store.SetBaseDir(root)
	if _, err := store.Save(context.Background(), metadataAuth("provider/token.json", map[string]any{"type": "test"})); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, provider} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("directory %q mode = %o, want 700", dir, info.Mode().Perm())
		}
	}
}

func TestFileTokenStoreRejectsSaveAndDeleteEscapesWithoutMutation(t *testing.T) {
	store, root := newTestFileStore(t)
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.json")
	original := []byte(`{"type":"outside"}`)
	if err := os.WriteFile(outside, original, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, auth := range []*switchailocalauth.Auth{
		metadataAuth("", map[string]any{"type": "bad"}),
		metadataAuth(".", map[string]any{"type": "bad"}),
		metadataAuth("..", map[string]any{"type": "bad"}),
		metadataAuth("provider/", map[string]any{"type": "bad"}),
		metadataAuth(`provider\token.json`, map[string]any{"type": "bad"}),
		metadataAuth("provider/token\x00.json", map[string]any{"type": "bad"}),
		metadataAuth("provider/token\n.json", map[string]any{"type": "bad"}),
		metadataAuth("provider/token", map[string]any{"type": "bad"}),
		metadataAuth("../outside.json", map[string]any{"type": "bad"}),
		{ID: "safe.json", FileName: "provider/../../outside.json", Metadata: map[string]any{"type": "bad"}},
		{ID: "safe.json", Attributes: map[string]string{"path": outside}, Metadata: map[string]any{"type": "bad"}},
	} {
		if _, err := store.Save(context.Background(), auth); err == nil {
			t.Fatalf("Save(%#v) unexpectedly succeeded", auth)
		}
	}
	for _, id := range []string{"../outside.json", "provider/../../outside.json", outside, "/"} {
		if err := store.Delete(context.Background(), id); err == nil {
			t.Fatalf("Delete(%q) unexpectedly succeeded", id)
		}
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != string(original) {
		t.Fatalf("outside file mutated: %q, %v", got, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected operations created store entries: %v", entries)
	}
}

func TestFileTokenStoreRejectsNestedPortableCollisions(t *testing.T) {
	store, root := newTestFileStore(t)
	if _, err := store.Save(context.Background(), metadataAuth("github/alice.json", map[string]any{"type": "one"})); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"github/Alice.json", "GitHub/bob.json"} {
		if _, err := store.Save(context.Background(), metadataAuth(id, map[string]any{"type": "two"})); err == nil {
			t.Fatalf("colliding Save(%q) unexpectedly succeeded", id)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "github", "alice.json")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "github", "alice.json"))
	if err != nil || !strings.Contains(string(raw), `"one"`) {
		t.Fatalf("original credential changed: %q, %v", raw, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "github"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "alice.json" {
		t.Fatalf("colliding save changed directory entries: %v", entries)
	}
	rootEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 1 || rootEntries[0].Name() != "github" {
		t.Fatalf("colliding parent save changed root entries: %v", rootEntries)
	}
}

func TestFileTokenStoreRejectsUnicodeCollisionsAndAllowsExactRecovery(t *testing.T) {
	store, root := newTestFileStore(t)
	const exact = "provider/café.json"
	if _, err := store.Save(context.Background(), metadataAuth(exact, map[string]any{"type": "one"})); err != nil {
		t.Fatal(err)
	}
	for _, alternate := range []string{"provider/cafe\u0301.json", "provider/CAFÉ.json"} {
		if _, err := store.Save(context.Background(), metadataAuth(alternate, map[string]any{"type": "two"})); err == nil {
			t.Fatalf("Unicode-colliding Save(%q) unexpectedly succeeded", alternate)
		}
		if err := store.Delete(context.Background(), alternate); err == nil {
			t.Fatalf("Unicode-colliding Delete(%q) unexpectedly succeeded", alternate)
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(exact))); err != nil {
		t.Fatalf("exact credential missing after collision rejects: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(exact)))
	if err != nil || !strings.Contains(string(raw), `"one"`) {
		t.Fatalf("exact credential changed after collision rejects: %q, %v", raw, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "provider"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "café.json" {
		t.Fatalf("Unicode collision changed directory entries: %v", entries)
	}
	if err := store.Delete(context.Background(), exact); err != nil {
		t.Fatalf("exact Unicode ID could not be deleted: %v", err)
	}
}

func TestFileTokenStoreDeleteRejectsAlternateSpelling(t *testing.T) {
	store, root := newTestFileStore(t)
	if _, err := store.Save(context.Background(), metadataAuth("provider/alice.json", map[string]any{"type": "test"})); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "provider/Alice.json"); err == nil {
		t.Fatal("alternate-spelling delete unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "provider", "alice.json")); err != nil {
		t.Fatalf("exact credential was deleted: %v", err)
	}
	if err := store.Delete(context.Background(), "provider/alice.json"); err != nil {
		t.Fatalf("exact-spelling recovery delete failed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "provider"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("exact delete left entries: %v", entries)
	}
}

func TestFileTokenStoreRejectsIntermediateSymlinks(t *testing.T) {
	store, root := newTestFileStore(t)
	outside := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"outside-link": outside, "inside-link": real} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		id := name + "/token.json"
		if _, err := store.Save(context.Background(), metadataAuth(id, map[string]any{"type": "bad"})); err == nil {
			t.Fatalf("Save through %s unexpectedly succeeded", name)
		}
		if err := store.Delete(context.Background(), id); err == nil {
			t.Fatalf("Delete through %s unexpectedly succeeded", name)
		}
	}
	if _, err := store.Save(context.Background(), metadataAuth("real/token.json", map[string]any{"type": "good"})); err != nil {
		t.Fatalf("canonical in-root path rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "token.json")); !os.IsNotExist(err) {
		t.Fatalf("outside target was created: %v", err)
	}
}

func TestFileTokenStoreDeleteFinalSymlinkRemovesLinkOnly(t *testing.T) {
	store, root := newTestFileStore(t)
	outside := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(outside, []byte(`{"type":"outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := store.Delete(context.Background(), "link.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("final symlink remains: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target was deleted: %v", err)
	}
}

func TestFileTokenStoreSaveRejectsEscapingFinalSymlinkWithoutTouchingTarget(t *testing.T) {
	store, root := newTestFileStore(t)
	outside := filepath.Join(t.TempDir(), "target.json")
	original := []byte(`{"type":"outside"}`)
	if err := os.WriteFile(outside, original, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Save(context.Background(), metadataAuth("link.json", map[string]any{"type": "inside"})); err == nil {
		t.Fatal("Save through escaping final symlink unexpectedly succeeded")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("rejected Save replaced final symlink")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != string(original) {
		t.Fatalf("outside target mutated: %q, %v", got, err)
	}
}

func TestFileTokenStoreSaveAtomicallyReplacesContainedFinalSymlink(t *testing.T) {
	store, root := newTestFileStore(t)
	target := filepath.Join(root, "target.json")
	original := []byte(`{"type":"target"}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink("target.json", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Save(context.Background(), metadataAuth("link.json", map[string]any{"type": "replacement"})); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("Save followed final symlink instead of replacing it")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(original) {
		t.Fatalf("contained symlink target mutated: %q, %v", got, err)
	}
	replacement, err := os.ReadFile(link)
	if err != nil || !strings.Contains(string(replacement), `"replacement"`) {
		t.Fatalf("replacement payload missing: %q, %v", replacement, err)
	}
}

func TestFileTokenStoreListSkipsEscapingSymlinkAndAtomicTempResidue(t *testing.T) {
	store, root := newTestFileStore(t)
	for id, provider := range map[string]string{
		"alpha.json":             "alpha",
		"zz-provider/token.json": "omega",
	} {
		if _, err := store.Save(context.Background(), metadataAuth(id, map[string]any{"type": provider})); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"type":"outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "nested.json"), []byte(`{"type":"outside-dir"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "dir-link")); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	residue := filepath.Join(root, fileStoreAtomicTempPrefix+strings.Repeat("a", 24))
	if err := os.WriteFile(residue, []byte(`{"type":"residue"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(listed))
	for _, auth := range listed {
		got[auth.ID] = auth.Provider
	}
	want := map[string]string{"alpha.json": "alpha", "zz-provider/token.json": "omega"}
	if len(got) != len(want) {
		t.Fatalf("List returned wrong survivors: got %#v want %#v", got, want)
	}
	for id, provider := range want {
		if got[id] != provider {
			t.Fatalf("List survivor %q provider = %q, want %q; all=%#v", id, got[id], provider, got)
		}
	}
}

func TestFileTokenStoreSweepsOnlyStaleAtomicTemps(t *testing.T) {
	store, root := newTestFileStore(t)
	nested := filepath.Join(root, "provider")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(nested, fileStoreAtomicTempPrefix+strings.Repeat("a", 24))
	fresh := filepath.Join(nested, fileStoreAtomicTempPrefix+strings.Repeat("b", 24))
	for _, file := range []string{stale, fresh} {
		if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldCredential := filepath.Join(nested, "old.json")
	oldUnrelated := filepath.Join(nested, "old.txt")
	prefixCredential := filepath.Join(nested, fileStoreAtomicTempPrefix+strings.Repeat("c", 24)+".json")
	for _, file := range []string{oldCredential, oldUnrelated, prefixCredential} {
		if err := os.WriteFile(file, []byte(`{"type":"must-survive"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * fileStoreTempMaxAge)
	for _, file := range []string{stale, oldCredential, oldUnrelated, prefixCredential, nested} {
		if err := os.Chtimes(file, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Save(context.Background(), metadataAuth("provider/token.json", map[string]any{"type": "test"})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale atomic temp remains: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh atomic temp removed: %v", err)
	}
	for _, file := range []string{oldCredential, oldUnrelated, prefixCredential} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("non-temp file %q was swept: %v", file, err)
		}
	}
}

func TestFileTokenStoreStorageSerializationUsesRootSafeMarshalerAndPreservesBytes(t *testing.T) {
	store, _ := newTestFileStore(t)
	want := []byte("{\"type\":\"recording\",\"token\":\"secret\"}\n")
	storage := &recordingTokenStorage{raw: want}
	gotPath, err := store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/token.json",
		Storage: storage,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("serialized bytes changed: got %q want %q", got, want)
	}
	if storage.saveCalls != 0 {
		t.Fatalf("path-based SaveTokenToFile called %d times", storage.saveCalls)
	}
}

func TestBuiltInTokenMarshalersMatchLegacyFileBytes(t *testing.T) {
	type storage interface {
		SaveTokenToFile(string) error
		MarshalToken() ([]byte, error)
	}
	for name, value := range map[string]storage{
		"claude": &claudeauth.ClaudeTokenStorage{AccessToken: "secret"},
		"codex":  &codexauth.CodexTokenStorage{AccessToken: "secret"},
		"gemini": &geminiauth.GeminiTokenStorage{Token: map[string]any{"access_token": "secret"}},
		"iflow":  &iflowauth.IFlowTokenStorage{AccessToken: "secret"},
		"qwen":   &qwenauth.QwenTokenStorage{AccessToken: "secret"},
		"vertex": &vertexauth.VertexCredentialStorage{ServiceAccount: map[string]any{"private_key": "secret"}},
		"vibe":   &VibeTokenStorage{AccessToken: "secret"},
		"ollama": &OllamaTokenStorage{BaseURL: "http://localhost:11434"},
	} {
		t.Run(name, func(t *testing.T) {
			want, err := value.MarshalToken()
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "token.json")
			if err = value.SaveTokenToFile(file); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("MarshalToken bytes differ from SaveTokenToFile:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestEmptyTokenMarshalerPreservesNoFileSemantics(t *testing.T) {
	storage := &emptyauth.EmptyStorage{}
	raw, err := storage.MarshalToken()
	if err != nil || raw != nil {
		t.Fatalf("MarshalToken() = %q, %v; want nil, nil", raw, err)
	}
	legacyPath := filepath.Join(t.TempDir(), "empty.json")
	if err = storage.SaveTokenToFile(legacyPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy EmptyStorage created a file: %v", err)
	}
	store, root := newTestFileStore(t)
	auth := &switchailocalauth.Auth{ID: "empty.json", Storage: storage}
	gotPath, err := store.Save(context.Background(), auth)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath == "" || auth.Attributes["path"] != gotPath {
		t.Fatalf("empty Save path state = %q, %#v", gotPath, auth.Attributes)
	}
	if _, err = os.Stat(filepath.Join(root, "empty.json")); !os.IsNotExist(err) {
		t.Fatalf("FileTokenStore materialized empty storage: %v", err)
	}
}

func TestFileTokenStoreStorageMarshalFailureLeavesNoFiles(t *testing.T) {
	store, root := newTestFileStore(t)
	wantErr := errors.New("serialize failed")
	storage := &recordingTokenStorage{err: wantErr}
	if _, err := store.Save(context.Background(), &switchailocalauth.Auth{ID: "token.json", Storage: storage}); !errors.Is(err, wantErr) {
		t.Fatalf("Save error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed marshal left residue: %v", entries)
	}
}

type legacyOnlyTokenStorage struct{}

func (*legacyOnlyTokenStorage) SaveTokenToFile(string) error { return nil }

func TestFileTokenStoreRejectsPathOnlyTokenStorage(t *testing.T) {
	store, root := newTestFileStore(t)
	_, err := store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "token.json",
		Storage: &legacyOnlyTokenStorage{},
	})
	if err == nil || !strings.Contains(err.Error(), "root-safe MarshalToken") {
		t.Fatalf("Save error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected legacy storage left files: %v", entries)
	}
}

func TestFileTokenStoreRejectsTypedNilTokenMarshaler(t *testing.T) {
	store, root := newTestFileStore(t)
	var storage *recordingTokenStorage
	_, err := store.Save(context.Background(), &switchailocalauth.Auth{ID: "token.json", Storage: storage})
	if err == nil || !strings.Contains(err.Error(), "is nil") {
		t.Fatalf("Save error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("typed-nil storage left files: %v", entries)
	}
}

func TestFileTokenStoreConcurrentSavesNeverExposePartialJSON(t *testing.T) {
	store, root := newTestFileStore(t)
	const id = "provider/token.json"
	if _, err := store.Save(context.Background(), metadataAuth(id, map[string]any{"type": "initial", "value": strings.Repeat("x", 4096)})); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, filepath.FromSlash(id))
	stop := make(chan struct{})
	readErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readErr <- nil
				return
			default:
			}
			raw, err := os.ReadFile(filePath)
			if err != nil {
				readErr <- err
				return
			}
			var value map[string]any
			if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
				readErr <- errors.New("reader observed partial JSON")
				return
			}
		}
	}()

	var writers sync.WaitGroup
	writeErr := make(chan error, 20)
	for i := 0; i < 20; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			_, err := store.Save(context.Background(), metadataAuth(id, map[string]any{
				"type":  "test",
				"index": i,
				"value": strings.Repeat(string(rune('a'+i%20)), 4096),
			}))
			if err != nil {
				writeErr <- err
			}
		}(i)
	}
	writers.Wait()
	close(writeErr)
	for err := range writeErr {
		t.Fatalf("concurrent Save failed: %v", err)
	}
	close(stop)
	select {
	case err := <-readErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not stop")
	}
}

func TestFileTokenStoreHonorsCanceledContext(t *testing.T) {
	store, root := newTestFileStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Save(ctx, metadataAuth("token.json", map[string]any{"type": "test"})); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v", err)
	}
	if err := store.Delete(ctx, "token.json"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete error = %v", err)
	}
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled operations left files: %v", entries)
	}
}

func TestFileTokenStoreDeleteMissingRootIsIdempotent(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "missing")
	store := NewFileTokenStore()
	store.SetBaseDir(root)
	if err := store.Delete(context.Background(), "token.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("Delete created missing root: %v", err)
	}
}

func TestFileTokenStoreListMissingRootReturnsErrorWithoutCreatingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	store := NewFileTokenStore()
	store.SetBaseDir(root)
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("List on missing root unexpectedly succeeded")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("List created missing root: %v", err)
	}
}
