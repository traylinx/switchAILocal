//go:build !race

package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// The pinned go-git file pack transport has an upstream stderr-buffer race.
// Keep one non-race smoke test for the real empty-remote bootstrap and push
// path; the security suite uses a deterministic test transport so it can run
// under the repository-wide -race CI gate.
func TestGitTokenStoreRealFileTransportPushesSaveAndDelete(t *testing.T) {
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

	id := "provider/tenant/token.json"
	if _, err := store.Save(context.Background(), gitMetadataAuth(id, "test")); err != nil {
		t.Fatal(err)
	}
	remoteTree := gitRemoteHeadTree(t, remoteDir)
	remotePath := filepath.ToSlash(filepath.Join("auths", filepath.FromSlash(id)))
	if _, err := remoteTree.File(remotePath); err != nil {
		t.Fatalf("saved credential absent from remote HEAD: %v", err)
	}

	if err := store.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	remoteTree = gitRemoteHeadTree(t, remoteDir)
	if _, err := remoteTree.File(remotePath); !errors.Is(err, object.ErrFileNotFound) {
		t.Fatalf("deleted credential remote state error = %v", err)
	}
}

func gitRemoteHeadTree(t *testing.T, remoteDir string) *object.Tree {
	t.Helper()
	repo, err := git.PlainOpen(remoteDir)
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
	return tree
}
