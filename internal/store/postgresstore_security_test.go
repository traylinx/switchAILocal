// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	emptyauth "github.com/traylinx/switchAILocal/internal/auth/empty"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

type postgresRecordingTokenStorage struct {
	raw       []byte
	err       error
	saveCalls int
}

func (s *postgresRecordingTokenStorage) SaveTokenToFile(string) error {
	s.saveCalls++
	return errors.New("path-based serializer must not be called")
}

func (s *postgresRecordingTokenStorage) MarshalToken() ([]byte, error) {
	return s.raw, s.err
}

type postgresLegacyOnlyTokenStorage struct{}

func (*postgresLegacyOnlyTokenStorage) SaveTokenToFile(string) error { return nil }

type postgresNilTokenStorage struct{}

func (*postgresNilTokenStorage) SaveTokenToFile(string) error  { return nil }
func (*postgresNilTokenStorage) MarshalToken() ([]byte, error) { return []byte(`{"type":"nil"}`), nil }

const postgresAuthUpsertPattern = `(?s)INSERT INTO "auth_store".*ON CONFLICT \(id\).*DO UPDATE SET content = EXCLUDED\.content`

func postgresMetadataAuth(id, provider string) *switchailocalauth.Auth {
	return &switchailocalauth.Auth{ID: id, Metadata: map[string]any{"type": provider}}
}

type postgresJSONArgument string

func (want postgresJSONArgument) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	return ok && jsonEqual(raw, []byte(want))
}

type postgresSignalingArgument struct {
	match   sqlmock.Argument
	once    sync.Once
	ready   chan struct{}
	proceed chan struct{}
}

func (argument *postgresSignalingArgument) Match(value driver.Value) bool {
	matched := argument.match.Match(value)
	if matched {
		argument.once.Do(func() { close(argument.ready) })
		<-argument.proceed
	}
	return matched
}

func newPostgresSecurityTestStore(t *testing.T) (*PostgresStore, sqlmock.Sqlmock, string) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	authDir := filepath.Join(t.TempDir(), "auths")
	if err = os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if resolved, errResolve := filepath.EvalSymlinks(authDir); errResolve == nil {
		authDir = resolved
	}
	store := &PostgresStore{
		db:        db,
		cfg:       PostgresStoreConfig{AuthTable: defaultAuthTable, ConfigTable: defaultConfigTable},
		spoolRoot: filepath.Dir(authDir),
		authDir:   authDir,
	}
	return store, mock, authDir
}

func expectPostgresAuthUpsert(mock sqlmock.Sqlmock, id string) {
	expectPostgresAuthUpsertArgument(mock, id, sqlmock.AnyArg())
}

func expectPostgresAuthUpsertPayload(mock sqlmock.Sqlmock, id string, payload string) {
	expectPostgresAuthUpsertArgument(mock, id, postgresJSONArgument(payload))
}

func expectPostgresAuthUpsertArgument(mock sqlmock.Sqlmock, id string, payload sqlmock.Argument) {
	mock.ExpectExec(postgresAuthUpsertPattern).
		WithArgs(id, payload).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectPostgresAuthDelete(mock sqlmock.Sqlmock, id string) {
	mock.ExpectExec(`DELETE FROM "auth_store" WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectPostgresAuthMutationStart(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(postgresAuthMutationLockKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectPostgresAuthFoldScan(mock sqlmock.Sqlmock, ids ...string) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	mock.ExpectQuery(`SELECT id FROM "auth_store"`).WillReturnRows(rows)
}

func expectPostgresAuthExists(mock sqlmock.Sqlmock, id string, exists bool) {
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM "auth_store" WHERE id = \$1\)`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func expectPostgresSaveUpsert(mock sqlmock.Sqlmock, id string) {
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock)
	expectPostgresAuthUpsert(mock, id)
	mock.ExpectCommit()
}

func expectPostgresDeleteMutation(mock sqlmock.Sqlmock, id string) {
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthDelete(mock, id)
	mock.ExpectCommit()
}

func requirePostgresSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable on Windows: %v", err)
		}
		t.Fatalf("create symlink %s -> %s: %v", link, target, err)
	}
}

func postgresAuthTreeFiles(t *testing.T, root string) []string {
	t.Helper()
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func requirePostgresErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v; want substring %q", err, want)
	}
}

func TestPostgresStoreNestedSaveAndContainedAbsoluteDelete(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	id := "provider/tenant/token.json"
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock)
	expectPostgresAuthUpsertPayload(mock, id, `{"type":"test"}`)
	mock.ExpectCommit()
	auth := postgresMetadataAuth(id, "test")
	filePath, err := store.Save(context.Background(), auth)
	if err != nil {
		t.Fatal(err)
	}
	if filePath != filepath.Join(authDir, filepath.FromSlash(id)) {
		t.Fatalf("Save path = %q", filePath)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode = %o", info.Mode().Perm())
	}
	for _, dir := range []string{
		filepath.Join(authDir, "provider"),
		filepath.Join(authDir, "provider", "tenant"),
	} {
		info, err = os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("credential parent %s = %v, %v", dir, info, err)
		}
	}
	if auth.Attributes["path"] != filePath || auth.FileName != id {
		t.Fatalf("saved auth path state = %#v, %q", auth.Attributes, auth.FileName)
	}
	expectPostgresDeleteMutation(mock, id)
	if err = store.Delete(context.Background(), filePath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("Delete left credential: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreSaveUnchangedMetadataStillUpsertsDatabase(t *testing.T) {
	store, mock, _ := newPostgresSecurityTestStore(t)
	id := "provider/token.json"
	expectPostgresSaveUpsert(mock, id)
	filePath, err := store.Save(context.Background(), postgresMetadataAuth(id, "test"))
	if err != nil {
		t.Fatal(err)
	}
	oldModTime := time.Unix(1_700_000_000, 0)
	if err = os.Chtimes(filePath, oldModTime, oldModTime); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock, id)
	expectPostgresAuthUpsert(mock, id)
	mock.ExpectCommit()
	if _, err = store.Save(context.Background(), postgresMetadataAuth(id, "test")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.ModTime().Equal(oldModTime) {
		t.Fatalf("unchanged mirror was rewritten: %v, %v; want mtime %v", info, err, oldModTime)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreSaveRepairsLooseUnchangedMirrorMode(t *testing.T) {
	store, mock, _ := newPostgresSecurityTestStore(t)
	id := "provider/token.json"
	expectPostgresSaveUpsert(mock, id)
	filePath, err := store.Save(context.Background(), postgresMetadataAuth(id, "test"))
	if err != nil {
		t.Fatal(err)
	}
	oldModTime := time.Unix(1_700_000_000, 0)
	if err = os.Chmod(filePath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(filePath, oldModTime, oldModTime); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock, id)
	expectPostgresAuthUpsert(mock, id)
	mock.ExpectCommit()
	if _, err = store.Save(context.Background(), postgresMetadataAuth(id, "test")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil || info.Mode().Perm() != 0o600 || info.ModTime().Equal(oldModTime) {
		t.Fatalf("loose mirror was not repaired: %v, %v", info, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreDeleteMissingFileStillDeletesDatabaseRecord(t *testing.T) {
	store, mock, _ := newPostgresSecurityTestStore(t)
	expectPostgresDeleteMutation(mock, "provider/missing.json")
	if err := store.Delete(context.Background(), "provider/missing.json"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreDisabledMissingMirrorUsesDatabaseAuthority(t *testing.T) {
	t.Run("absent in database is a no-op", func(t *testing.T) {
		store, mock, authDir := newPostgresSecurityTestStore(t)
		expectPostgresAuthMutationStart(mock)
		expectPostgresAuthExists(mock, "provider/disabled.json", false)
		mock.ExpectRollback()
		got, err := store.Save(context.Background(), &switchailocalauth.Auth{ID: "provider/disabled.json", Disabled: true})
		if err != nil || got != "" {
			t.Fatalf("disabled absent Save = %q, %v", got, err)
		}
		if _, err = os.Stat(filepath.Join(authDir, "provider")); !os.IsNotExist(err) {
			t.Fatalf("disabled absent Save mutated mirror: %v", err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("existing database row is updated and mirrored", func(t *testing.T) {
		store, mock, authDir := newPostgresSecurityTestStore(t)
		id := "provider/disabled.json"
		expectPostgresAuthMutationStart(mock)
		expectPostgresAuthExists(mock, id, true)
		expectPostgresAuthFoldScan(mock, id)
		expectPostgresAuthUpsert(mock, id)
		mock.ExpectCommit()
		auth := postgresMetadataAuth(id, "test")
		auth.Disabled = true
		if _, err := store.Save(context.Background(), auth); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(authDir, filepath.FromSlash(id)))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("disabled existing Save mirror = %v, %v", info, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPostgresStoreRejectsDatabaseFoldCollisionWithoutMirrorMutation(t *testing.T) {
	for _, tc := range []struct {
		name, existing, candidate string
	}{
		{name: "leaf case", existing: "github/alice.json", candidate: "github/Alice.json"},
		{name: "directory case", existing: "GitHub/bob.json", candidate: "github/bob.json"},
		{name: "unicode normalization", existing: "github/caf\u00e9.json", candidate: "github/cafe\u0301.json"},
		{name: "legacy suffix", existing: "github/alice", candidate: "github/Alice.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, authDir := newPostgresSecurityTestStore(t)
			expectPostgresAuthMutationStart(mock)
			expectPostgresAuthFoldScan(mock, tc.existing)
			mock.ExpectRollback()
			_, err := store.Save(context.Background(), postgresMetadataAuth(tc.candidate, "test"))
			requirePostgresErrorContains(t, err, "collides")
			entries, errRead := os.ReadDir(authDir)
			if errRead != nil || len(entries) != 0 {
				t.Fatalf("database collision mutated mirror: %v, %v", entries, errRead)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresStoreDatabaseFailureLeavesMirrorUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		expectFail func(sqlmock.Sqlmock)
	}{
		{
			name: "begin", want: "begin auth mutation",
			expectFail: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
			},
		},
		{
			name: "advisory lock", want: "lock auth mutations",
			expectFail: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
					WithArgs(postgresAuthMutationLockKey).
					WillReturnError(errors.New("lock failed"))
				mock.ExpectRollback()
			},
		},
		{
			name: "fold scan", want: "query auth ids",
			expectFail: func(mock sqlmock.Sqlmock) {
				expectPostgresAuthMutationStart(mock)
				mock.ExpectQuery(`SELECT id FROM "auth_store"`).WillReturnError(errors.New("scan failed"))
				mock.ExpectRollback()
			},
		},
		{
			name: "fold scan row error", want: "iterate auth ids",
			expectFail: func(mock sqlmock.Sqlmock) {
				expectPostgresAuthMutationStart(mock)
				rows := sqlmock.NewRows([]string{"id"}).
					AddRow("provider/other.json").
					RowError(0, errors.New("row failed"))
				mock.ExpectQuery(`SELECT id FROM "auth_store"`).WillReturnRows(rows)
				mock.ExpectRollback()
			},
		},
		{
			name: "fold scan value error", want: "scan auth id",
			expectFail: func(mock sqlmock.Sqlmock) {
				expectPostgresAuthMutationStart(mock)
				rows := sqlmock.NewRows([]string{"id"}).AddRow(nil)
				mock.ExpectQuery(`SELECT id FROM "auth_store"`).WillReturnRows(rows)
				mock.ExpectRollback()
			},
		},
		{
			name: "upsert", want: "upsert auth record",
			expectFail: func(mock sqlmock.Sqlmock) {
				expectPostgresAuthMutationStart(mock)
				expectPostgresAuthFoldScan(mock)
				mock.ExpectExec(postgresAuthUpsertPattern).
					WithArgs("provider/token.json", sqlmock.AnyArg()).
					WillReturnError(errors.New("upsert failed"))
				mock.ExpectRollback()
			},
		},
		{
			name: "commit", want: "commit auth save",
			expectFail: func(mock sqlmock.Sqlmock) {
				expectPostgresAuthMutationStart(mock)
				expectPostgresAuthFoldScan(mock)
				expectPostgresAuthUpsert(mock, "provider/token.json")
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, authDir := newPostgresSecurityTestStore(t)
			tc.expectFail(mock)
			_, err := store.Save(context.Background(), postgresMetadataAuth("provider/token.json", "test"))
			requirePostgresErrorContains(t, err, tc.want)
			entries, errRead := os.ReadDir(authDir)
			if errRead != nil || len(entries) != 0 {
				t.Fatalf("database failure mutated mirror: %v, %v", entries, errRead)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresStorePostCommitMirrorFailureIsExplicitAndRecoverable(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	id := "provider/token.json"
	ready := make(chan struct{})
	proceed := make(chan struct{})
	defer func() {
		select {
		case <-proceed:
		default:
			close(proceed)
		}
	}()
	payload := &postgresSignalingArgument{
		match:   postgresJSONArgument(`{"type":"test"}`),
		ready:   ready,
		proceed: proceed,
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock)
	mock.ExpectExec(postgresAuthUpsertPattern).
		WithArgs(id, payload).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	result := make(chan error, 1)
	go func() {
		_, err := store.Save(context.Background(), postgresMetadataAuth(id, "test"))
		result <- err
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("Save did not reach authoritative upsert")
	}
	filePath := filepath.Join(authDir, filepath.FromSlash(id))
	if err := os.MkdirAll(filePath, 0o700); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	var err error
	select {
	case err = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("Save did not return after mirror failure")
	}
	requirePostgresErrorContains(t, err, "database committed auth")
	if info, errStat := os.Stat(filePath); errStat != nil || !info.IsDir() {
		t.Fatalf("mirror failure fixture changed: %v, %v", info, errStat)
	}
	if err = os.RemoveAll(filePath); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock, id)
	expectPostgresAuthUpsertPayload(mock, id, `{"type":"test"}`)
	mock.ExpectCommit()
	if _, err = store.Save(context.Background(), postgresMetadataAuth(id, "test")); err != nil {
		t.Fatalf("retry did not repair mirror: %v", err)
	}
	if info, errStat := os.Stat(filePath); errStat != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("repaired mirror = %v, %v", info, errStat)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreDeleteDatabaseFailurePreservesMirror(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		expectFail func(sqlmock.Sqlmock, string)
	}{
		{
			name: "delete", want: "delete auth record",
			expectFail: func(mock sqlmock.Sqlmock, id string) {
				expectPostgresAuthMutationStart(mock)
				mock.ExpectExec(`DELETE FROM "auth_store" WHERE id = \$1`).
					WithArgs(id).
					WillReturnError(errors.New("delete failed"))
				mock.ExpectRollback()
			},
		},
		{
			name: "commit", want: "commit auth delete",
			expectFail: func(mock sqlmock.Sqlmock, id string) {
				expectPostgresAuthMutationStart(mock)
				expectPostgresAuthDelete(mock, id)
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, authDir := newPostgresSecurityTestStore(t)
			id := "provider/token.json"
			filePath := filepath.Join(authDir, filepath.FromSlash(id))
			if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filePath, []byte(`{"type":"test"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			tc.expectFail(mock, id)
			requirePostgresErrorContains(t, store.Delete(context.Background(), id), tc.want)
			if _, err := os.Stat(filePath); err != nil {
				t.Fatalf("database delete failure removed mirror: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresStoreRejectsFinalDirectory(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	id := "provider/directory.json"
	if err := os.MkdirAll(filepath.Join(authDir, filepath.FromSlash(id)), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := store.Save(context.Background(), postgresMetadataAuth(id, "bad"))
	requirePostgresErrorContains(t, err, "final id segment")
	requirePostgresErrorContains(t, store.Delete(context.Background(), id), "final id segment")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreRejectsSymlinkedAuthRootWithoutTouchingTarget(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	outside := t.TempDir()
	if err := os.Chmod(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(authDir); err != nil {
		t.Fatal(err)
	}
	requirePostgresSymlink(t, outside, authDir)
	if _, err := store.Save(context.Background(), postgresMetadataAuth("provider/token.json", "test")); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("symlinked auth root error = %v", err)
	}
	info, err := os.Stat(outside)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("symlink target mode = %v, %v; want 0755", info, err)
	}
	if _, err = os.Stat(filepath.Join(outside, "provider")); !os.IsNotExist(err) {
		t.Fatalf("symlink target received auth data: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreCanceledContextMutatesNothing(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Save(ctx, postgresMetadataAuth("provider/token.json", "test")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v; want context.Canceled", err)
	}
	if err := store.Delete(ctx, "provider/token.json"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete error = %v; want context.Canceled", err)
	}
	if err := store.PersistAuthFiles(ctx, "", "provider/token.json"); !errors.Is(err, context.Canceled) {
		t.Fatalf("PersistAuthFiles error = %v; want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(authDir, "provider")); !os.IsNotExist(err) {
		t.Fatalf("canceled operations mutated auth root: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreRejectsEscapesAndPortableAliases(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	original := []byte(`{"type":"outside"}`)
	if err := os.WriteFile(outside, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), &switchailocalauth.Auth{
		ID:         "safe.json",
		Attributes: map[string]string{"path": outside},
		Metadata:   map[string]any{"type": "bad"},
	}); err == nil || !strings.Contains(err.Error(), "outside managed root") {
		t.Fatalf("outside absolute Save error = %v", err)
	}
	requirePostgresErrorContains(t, store.Delete(context.Background(), outside), "outside managed root")
	expectPostgresSaveUpsert(mock, "github/alice.json")
	if _, err := store.Save(context.Background(), postgresMetadataAuth("github/alice.json", "one")); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock, "github/alice.json")
	expectPostgresAuthUpsert(mock, "github/café.json")
	mock.ExpectCommit()
	if _, err := store.Save(context.Background(), postgresMetadataAuth("github/café.json", "one")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"github/Alice.json", "GitHub/bob.json", "github/cafe\u0301.json"} {
		_, err := store.Save(context.Background(), postgresMetadataAuth(id, "two"))
		requirePostgresErrorContains(t, err, "collides with existing")
	}
	requirePostgresErrorContains(t, store.Delete(context.Background(), "github/Alice.json"), "collides with existing")
	raw, err := os.ReadFile(filepath.Join(authDir, "github", "alice.json"))
	if err != nil || !strings.Contains(string(raw), `"one"`) {
		t.Fatalf("exact credential changed: %q, %v", raw, err)
	}
	if got, errRead := os.ReadFile(outside); errRead != nil || string(got) != string(original) {
		t.Fatalf("outside file changed: %q, %v", got, errRead)
	}
	expectPostgresDeleteMutation(mock, "github/alice.json")
	if err = store.Delete(context.Background(), "github/alice.json"); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreRejectsInvalidIDsWithoutDatabaseMutation(t *testing.T) {
	for _, tc := range []struct {
		id                   string
		wantSave, wantDelete string
	}{
		{id: "", wantSave: "missing id", wantDelete: "id is empty"},
		{id: ".", wantSave: "traversal segment", wantDelete: "traversal segment"},
		{id: "..", wantSave: "traversal segment", wantDelete: "traversal segment"},
		{id: "../outside.json", wantSave: "traversal segment", wantDelete: "traversal segment"},
		{id: "provider/../outside.json", wantSave: "traversal segment", wantDelete: "traversal segment"},
		{id: "provider//token.json", wantSave: "empty segment", wantDelete: "empty segment"},
		{id: "provider\\token.json", wantSave: "forward-slash", wantDelete: "forward-slash"},
		{id: "provider/\x00.json", wantSave: "control character", wantDelete: "control character"},
		{id: "provider/token.txt", wantSave: "end in .json", wantDelete: "end in .json"},
		{id: "provider/.json", wantSave: "end in .json", wantDelete: "end in .json"},
		{id: "/absolute.json", wantSave: "outside managed root", wantDelete: "outside managed root"},
	} {
		t.Run(strings.ReplaceAll(tc.id, "/", "_"), func(t *testing.T) {
			store, mock, authDir := newPostgresSecurityTestStore(t)
			_, err := store.Save(context.Background(), postgresMetadataAuth(tc.id, "bad"))
			requirePostgresErrorContains(t, err, tc.wantSave)
			requirePostgresErrorContains(t, store.Delete(context.Background(), tc.id), tc.wantDelete)
			if _, err := os.Stat(filepath.Join(filepath.Dir(authDir), "outside.json")); !os.IsNotExist(err) {
				t.Fatalf("invalid ID %q mutated outside root: %v", tc.id, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresStoreSymlinksStayContained(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	outDir := t.TempDir()
	realDir := filepath.Join(authDir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, target, want string }{
		{"outside-link", outDir, "outside managed root"},
		{"inside-link", realDir, "noncanonical path"},
	} {
		name, target := tc.name, tc.target
		requirePostgresSymlink(t, target, filepath.Join(authDir, name))
		id := name + "/token.json"
		_, err := store.Save(context.Background(), postgresMetadataAuth(id, "bad"))
		requirePostgresErrorContains(t, err, tc.want)
		requirePostgresErrorContains(t, store.Delete(context.Background(), id), tc.want)
	}

	targetID := "provider/target.json"
	expectPostgresSaveUpsert(mock, targetID)
	targetPath, err := store.Save(context.Background(), postgresMetadataAuth(targetID, "target"))
	if err != nil {
		t.Fatal(err)
	}
	targetBefore, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(authDir, "provider", "link.json")
	requirePostgresSymlink(t, "target.json", linkPath)
	expectPostgresSaveUpsert(mock, "provider/link.json")
	if _, err = store.Save(context.Background(), postgresMetadataAuth("provider/link.json", "target")); err != nil {
		t.Fatal(err)
	}
	if info, errLstat := os.Lstat(linkPath); errLstat != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("contained final symlink was not replaced: %v, %v", info, errLstat)
	}
	if got, errRead := os.ReadFile(targetPath); errRead != nil || string(got) != string(targetBefore) {
		t.Fatalf("contained symlink target changed: %q, %v", got, errRead)
	}
	danglingPath := filepath.Join(authDir, "provider", "dangling-storage.json")
	requirePostgresSymlink(t, "missing-storage.json", danglingPath)
	expectPostgresSaveUpsert(mock, "provider/dangling-storage.json")
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/dangling-storage.json",
		Storage: &postgresRecordingTokenStorage{raw: []byte(`{"type":"storage"}`)},
	}); err != nil {
		t.Fatal(err)
	}
	if info, errLstat := os.Lstat(danglingPath); errLstat != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("contained dangling symlink was not replaced: %v, %v", info, errLstat)
	}
	if _, err = os.Stat(filepath.Join(authDir, "provider", "missing-storage.json")); !os.IsNotExist(err) {
		t.Fatalf("dangling target unexpectedly materialized: %v", err)
	}

	outFile := filepath.Join(outDir, "outside.json")
	outBefore := []byte(`{"type":"outside"}`)
	if err = os.WriteFile(outFile, outBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	escapePath := filepath.Join(authDir, "provider", "escape.json")
	requirePostgresSymlink(t, outFile, escapePath)
	_, err = store.Save(context.Background(), postgresMetadataAuth("provider/escape.json", "bad"))
	requirePostgresErrorContains(t, err, "inspect final symlink target")
	storageEscapePath := filepath.Join(authDir, "provider", "storage-escape.json")
	requirePostgresSymlink(t, outFile, storageEscapePath)
	_, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/storage-escape.json",
		Storage: &postgresRecordingTokenStorage{raw: []byte(`{"type":"bad"}`)},
	})
	requirePostgresErrorContains(t, err, "inspect final symlink target")
	if info, errLstat := os.Lstat(storageEscapePath); errLstat != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("storage-backed escaping symlink changed: %v, %v", info, errLstat)
	}
	expectPostgresDeleteMutation(mock, "provider/escape.json")
	if err = store.Delete(context.Background(), "provider/escape.json"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(escapePath); !os.IsNotExist(err) {
		t.Fatalf("escaping final symlink remains: %v", err)
	}
	if got, errRead := os.ReadFile(outFile); errRead != nil || string(got) != string(outBefore) {
		t.Fatalf("escaping symlink target changed: %q, %v", got, errRead)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreRootSafeMarshalerAndEmptySemantics(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	want := []byte("{\"type\":\"recording\",\"token\":\"secret\"}\n")
	storage := &postgresRecordingTokenStorage{raw: want}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock)
	expectPostgresAuthUpsertPayload(mock, "provider/token.json", string(want))
	mock.ExpectCommit()
	filePath, err := store.Save(context.Background(), &switchailocalauth.Auth{ID: "provider/token.json", Storage: storage})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil || string(got) != string(want) {
		t.Fatalf("stored bytes = %q, %v; want %q", got, err, want)
	}
	if storage.saveCalls != 0 {
		t.Fatalf("path-based serializer called %d times", storage.saveCalls)
	}
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{ID: "provider/nothing.json"}); err == nil || !strings.Contains(err.Error(), "nothing to persist") {
		t.Fatalf("missing payload error = %v", err)
	}
	_, err = store.Save(context.Background(), &switchailocalauth.Auth{ID: "provider/legacy.json", Storage: &postgresLegacyOnlyTokenStorage{}})
	requirePostgresErrorContains(t, err, "root-safe MarshalToken")
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/zero.json",
		Storage: &postgresRecordingTokenStorage{raw: []byte{}},
	}); err == nil || !strings.Contains(err.Error(), "empty payload") {
		t.Fatalf("empty non-nil payload error = %v", err)
	}
	var nilStorage *postgresNilTokenStorage
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{
		ID:      "provider/nil.json",
		Storage: nilStorage,
	}); err == nil || !strings.Contains(err.Error(), "is nil") {
		t.Fatalf("typed-nil storage error = %v", err)
	}
	wantErr := errors.New("serialize failed")
	if _, err = store.Save(context.Background(), &switchailocalauth.Auth{ID: "provider/error.json", Storage: &postgresRecordingTokenStorage{err: wantErr}}); !errors.Is(err, wantErr) {
		t.Fatalf("marshal error = %v; want %v", err, wantErr)
	}
	empty := &switchailocalauth.Auth{ID: "provider/empty.json", Storage: &emptyauth.EmptyStorage{}}
	// No SQL expectation is queued: EmptyStorage deliberately has no file or DB row.
	emptyPath, err := store.Save(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	if emptyPath == "" || empty.Attributes["path"] != emptyPath {
		t.Fatalf("empty storage path state = %q, %#v", emptyPath, empty.Attributes)
	}
	for _, id := range []string{
		"provider/legacy.json", "provider/zero.json", "provider/nil.json",
		"provider/error.json", "provider/empty.json",
	} {
		if _, err = os.Stat(filepath.Join(authDir, filepath.FromSlash(id))); !os.IsNotExist(err) {
			t.Fatalf("rejected/no-file storage materialized %s: %v", id, err)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreSecuresPermissionsAndSweepsOnlyStaleAtomicTemps(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	// The odd-length prefix makes an unprefixed, exact-total-length name fail
	// hex decoding by parity; the explicit prefix branch has no non-equivalent mutant.
	if storeAtomicTempPrefix != ".tmp-" {
		t.Fatalf("storeAtomicTempPrefix = %q; want .tmp-", storeAtomicTempPrefix)
	}
	if storeTempMaxAge != time.Hour {
		t.Fatalf("storeTempMaxAge = %s; want 1h", storeTempMaxAge)
	}
	if err := os.Chmod(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(authDir, "provider")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("a", 24))
	fresh := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("b", 24))
	unrelated := filepath.Join(nested, storeAtomicTempPrefix+"unrelated")
	invalidHex := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("z", 24))
	shortHex := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("d", 22))
	longHex := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("e", 26))
	oldCredential := filepath.Join(nested, "old-token.json")
	prefixCredential := filepath.Join(nested, storeAtomicTempPrefix+strings.Repeat("c", 24)+".json")
	for _, file := range []string{stale, fresh, unrelated, invalidHex, shortHex, longHex, oldCredential, prefixCredential} {
		if err := os.WriteFile(file, []byte("residue"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, file := range []string{stale, unrelated, invalidHex, shortHex, longHex, oldCredential, prefixCredential} {
		if err := os.Chtimes(file, old, old); err != nil {
			t.Fatal(err)
		}
	}
	expectPostgresSaveUpsert(mock, "provider/token.json")
	filePath, err := store.Save(context.Background(), postgresMetadataAuth("provider/token.json", "test"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{{authDir, 0o700}, {nested, 0o700}, {filePath, 0o600}} {
		info, errStat := os.Stat(tc.path)
		if errStat != nil || info.Mode().Perm() != tc.mode {
			t.Fatalf("mode %s = %v, %v; want %o", tc.path, info, errStat, tc.mode)
		}
	}
	if _, err = os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale atomic temp remains: %v", err)
	}
	for _, path := range []string{fresh, unrelated, invalidHex, shortHex, longHex, oldCredential, prefixCredential} {
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("unrelated/fresh file removed: %s: %v", path, err)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSweepStoredAuthTempsBoundaryAndDirectorySafety(t *testing.T) {
	authDir := t.TempDir()
	root, err := os.OpenRoot(authDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	fixedNow := time.Unix(2_000_000_000, 0)
	boundary := filepath.Join(authDir, storeAtomicTempPrefix+strings.Repeat("a", 24))
	tempDirectory := filepath.Join(authDir, storeAtomicTempPrefix+strings.Repeat("b", 24))
	if err = os.WriteFile(boundary, []byte("residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(tempDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	boundaryTime := fixedNow.Add(-storeTempMaxAge)
	if err = os.Chtimes(boundary, boundaryTime, boundaryTime); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(tempDirectory, boundaryTime, boundaryTime); err != nil {
		t.Fatal(err)
	}
	if err = sweepStoredAuthTemps(context.Background(), root, fixedNow); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(boundary); !os.IsNotExist(err) {
		t.Fatalf("exact-age temp was not swept: %v", err)
	}
	if info, errStat := os.Stat(tempDirectory); errStat != nil || !info.IsDir() {
		t.Fatalf("temp-shaped directory was removed: %v, %v", info, errStat)
	}
}

func TestPostgresStoreListSkipsInvalidDatabaseIDs(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "content", "created_at", "updated_at"}).
		AddRow("provider/token.json", []byte(`{"type":"valid"}`), now, now).
		AddRow("../outside.json", []byte(`{"type":"bad"}`), now, now).
		AddRow("/absolute.json", []byte(`{"type":"bad"}`), now, now).
		AddRow("provider/token.txt", []byte(`{"type":"bad"}`), now, now)
	mock.ExpectQuery(`SELECT id, content, created_at, updated_at FROM "auth_store" ORDER BY id COLLATE "C"`).WillReturnRows(rows)
	auths, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(auths) != 1 || auths[0].ID != "provider/token.json" || auths[0].Attributes["path"] != filepath.Join(authDir, "provider", "token.json") {
		t.Fatalf("List() = %#v", auths)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreListSkipsPortableDatabaseAliases(t *testing.T) {
	store, mock, _ := newPostgresSecurityTestStore(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "content", "created_at", "updated_at"}).
		AddRow("github/Alice.json", []byte(`{"type":"first"}`), now, now).
		AddRow("github/alice.json", []byte(`{"type":"second"}`), now, now).
		AddRow("provider/token.json", []byte(`{"type":"distinct"}`), now, now)
	mock.ExpectQuery(`SELECT id, content, created_at, updated_at FROM "auth_store" ORDER BY id COLLATE "C"`).WillReturnRows(rows)
	auths, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(auths) != 2 || auths[0].ID != "github/Alice.json" || auths[1].ID != "provider/token.json" {
		t.Fatalf("List portable aliases = %#v", auths)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncAuthFromDatabaseSkipsInvalidDatabaseIDs(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	outside := filepath.Join(filepath.Dir(authDir), "outside.json")
	original := []byte("outside")
	if err := os.WriteFile(outside, original, 0o600); err != nil {
		t.Fatal(err)
	}
	rows := sqlmock.NewRows([]string{"id", "content"}).
		AddRow("provider/token.json", `{"type":"valid"}`).
		AddRow("../outside.json", `{"type":"bad"}`).
		AddRow("/absolute.json", `{"type":"bad"}`).
		AddRow("provider/token.txt", `{"type":"bad"}`).
		AddRow("provider//duplicate.json", `{"type":"bad"}`)
	mock.ExpectQuery(`SELECT id, content FROM "auth_store" ORDER BY id COLLATE "C" LIMIT 100`).WillReturnRows(rows)
	if err := store.syncAuthFromDatabase(context.Background()); err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(authDir, "provider", "token.json")
	if got, err := os.ReadFile(validPath); err != nil || string(got) != `{"type":"valid"}` {
		t.Fatalf("valid database auth = %q, %v", got, err)
	}
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{{authDir, 0o700}, {filepath.Join(authDir, "provider"), 0o700}, {validPath, 0o600}} {
		info, err := os.Stat(tc.path)
		if err != nil || info.Mode().Perm() != tc.mode {
			t.Fatalf("sync mode %s = %v, %v; want %o", tc.path, info, err, tc.mode)
		}
	}
	if files := postgresAuthTreeFiles(t, authDir); len(files) != 1 || files[0] != "provider/token.json" {
		t.Fatalf("sync materialized unexpected files: %v", files)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != string(original) {
		t.Fatalf("invalid database ID changed outside file: %q, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncAuthFromDatabaseSkipsPortableAliasesAndUnsafeLocalEntries(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	rows := sqlmock.NewRows([]string{"id", "content"}).
		AddRow("github/Alice.json", `{"type":"first"}`).
		AddRow("github/alice.json", `{"type":"second"}`).
		AddRow("provider/directory.json", `{"type":"parent"}`).
		AddRow("provider/directory.json/child.json", `{"type":"unsafe"}`).
		AddRow("provider/good.json", `{"type":"good"}`)
	mock.ExpectQuery(`SELECT id, content FROM "auth_store" ORDER BY id COLLATE "C" LIMIT 100`).WillReturnRows(rows)
	if err := store.syncAuthFromDatabase(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{
		"github/Alice.json",
		"provider/directory.json",
		"provider/good.json",
	}
	if files := postgresAuthTreeFiles(t, authDir); strings.Join(files, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("sync alias/unsafe files = %v, want %v", files, wantFiles)
	}
	if got, err := os.ReadFile(filepath.Join(authDir, "github", "Alice.json")); err != nil || string(got) != `{"type":"first"}` {
		t.Fatalf("first portable alias = %q, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncAuthFromDatabaseAdvancesPastSkippedLastRowAcrossBatches(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	firstBatch := sqlmock.NewRows([]string{"id", "content"})
	for i := 0; i < 98; i++ {
		id := fmt.Sprintf("auth/%03d.json", i)
		firstBatch.AddRow(id, fmt.Sprintf(`{"type":"%03d"}`, i))
	}
	firstBatch.
		AddRow("github/Alice.json", `{"type":"first"}`).
		AddRow("github/alice.json", `{"type":"skipped"}`)
	mock.ExpectQuery(`SELECT id, content FROM "auth_store" ORDER BY id COLLATE "C" LIMIT 100`).
		WillReturnRows(firstBatch)
	secondBatch := sqlmock.NewRows([]string{"id", "content"}).
		AddRow("provider/good.json", `{"type":"good"}`)
	mock.ExpectQuery(`SELECT id, content FROM "auth_store" WHERE id COLLATE "C" > \$1 ORDER BY id COLLATE "C" LIMIT 100`).
		WithArgs("github/alice.json").
		WillReturnRows(secondBatch)
	if err := store.syncAuthFromDatabase(context.Background()); err != nil {
		t.Fatal(err)
	}
	files := postgresAuthTreeFiles(t, authDir)
	if len(files) != 100 || files[98] != "github/Alice.json" || files[99] != "provider/good.json" {
		t.Fatalf("cross-batch sync files = %v", files)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreMigratesLegacyAuthIDs(t *testing.T) {
	store, mock, _ := newPostgresSecurityTestStore(t)
	expectPostgresAuthMutationStart(mock)
	rows := sqlmock.NewRows([]string{"id"}).
		AddRow("already.JSON").
		AddRow("github/Alice.json").
		AddRow("github/alice.json").
		AddRow("legacy").
		AddRow("nested/token.txt")
	mock.ExpectQuery(`SELECT id FROM "auth_store" ORDER BY id COLLATE "C"`).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE "auth_store" SET id = \$1 WHERE id = \$2`).
		WithArgs("legacy.json", "legacy").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "auth_store" SET id = \$1 WHERE id = \$2`).
		WithArgs("nested/token.txt.json", "nested/token.txt").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.migrateLegacyAuthIDs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigratedLegacyAuthIDOnlyAppendsMissingJSONSuffix(t *testing.T) {
	got, err := migratedLegacyAuthID("nested/token.txt")
	if err != nil || got != "nested/token.txt.json" {
		t.Fatalf("migrated legacy id = %q, %v", got, err)
	}
	for _, id := range []string{"already.json", "already.JSON"} {
		if got, err = migratedLegacyAuthID(id); err == nil || got != "" || !strings.Contains(err.Error(), "already has") {
			t.Fatalf("already-suffixed legacy id %q = %q, %v", id, got, err)
		}
	}
}

func TestPostgresStoreBootstrapMigratesLegacyIDsBeforeMirrorSync(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	store.configPath = filepath.Join(store.spoolRoot, "config", "config.yaml")
	legacyMirror := filepath.Join(authDir, "legacy")
	if err := os.WriteFile(legacyMirror, []byte(`{"type":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS "config_store"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS "auth_store"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectPostgresAuthMutationStart(mock)
	mock.ExpectQuery(`SELECT id FROM "auth_store" ORDER BY id COLLATE "C"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("legacy"))
	mock.ExpectExec(`UPDATE "auth_store" SET id = \$1 WHERE id = \$2`).
		WithArgs("legacy.json", "legacy").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT content FROM "config_store" WHERE id = \$1`).
		WithArgs(defaultConfigKey).
		WillReturnRows(sqlmock.NewRows([]string{"content"}).AddRow("service: test\n"))
	mock.ExpectQuery(`SELECT id, content FROM "auth_store" ORDER BY id COLLATE "C" LIMIT 100`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content"}).AddRow("legacy.json", `{"type":"legacy"}`))
	if err := store.Bootstrap(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacyMirror); !os.IsNotExist(err) {
		t.Fatalf("legacy mirror remains after bootstrap: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(authDir, "legacy.json")); err != nil || string(got) != `{"type":"legacy"}` {
		t.Fatalf("migrated mirror = %q, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreLegacyAuthIDMigrationQuarantinesPortableCollision(t *testing.T) {
	store, mock, _ := newPostgresSecurityTestStore(t)
	expectPostgresAuthMutationStart(mock)
	rows := sqlmock.NewRows([]string{"id"}).
		AddRow("github/Alice").
		AddRow("github/alice.json").
		AddRow("provider/safe")
	mock.ExpectQuery(`SELECT id FROM "auth_store" ORDER BY id COLLATE "C"`).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE "auth_store" SET id = \$1 WHERE id = \$2`).
		WithArgs("provider/safe.json", "provider/safe").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.migrateLegacyAuthIDs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncAuthFromDatabaseReplacesSymlinkedRootWithoutTouchingTarget(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	outDir := t.TempDir()
	marker := filepath.Join(outDir, "marker")
	if err := os.WriteFile(marker, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(authDir); err != nil {
		t.Fatal(err)
	}
	requirePostgresSymlink(t, outDir, authDir)
	rows := sqlmock.NewRows([]string{"id", "content"}).AddRow("provider/token.json", `{"type":"valid"}`)
	mock.ExpectQuery(`SELECT id, content FROM "auth_store" ORDER BY id COLLATE "C" LIMIT 100`).WillReturnRows(rows)
	if err := store.syncAuthFromDatabase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "outside" {
		t.Fatalf("sync changed symlink target: %q, %v", got, err)
	}
	info, err := os.Lstat(authDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("sync auth root = %v, %v", info, err)
	}
	if files := postgresAuthTreeFiles(t, authDir); len(files) != 1 || files[0] != "provider/token.json" {
		t.Fatalf("sync files = %v", files)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStorePersistAuthFilesContainsPaths(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	id := "provider/token.json"
	filePath := filepath.Join(authDir, filepath.FromSlash(id))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(`{"type":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock)
	expectPostgresAuthUpsert(mock, id)
	mock.ExpectCommit()
	if err := store.PersistAuthFiles(context.Background(), "", filePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthDelete(mock, id)
	mock.ExpectCommit()
	if err := store.PersistAuthFiles(context.Background(), "", filePath); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"type":"outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	mock.ExpectCommit()
	if err := store.PersistAuthFiles(context.Background(), "", outside); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != `{"type":"outside"}` {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
	directoryID := "provider/directory.json"
	if err := os.MkdirAll(filepath.Join(authDir, filepath.FromSlash(directoryID)), 0o700); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	mock.ExpectCommit()
	if err := store.PersistAuthFiles(context.Background(), "", directoryID); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStorePersistAuthFilesRecreatesMissingRoot(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	if err := os.RemoveAll(authDir); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthDelete(mock, "provider/missing.json")
	mock.ExpectCommit()
	if err := store.PersistAuthFiles(context.Background(), "", "provider/missing.json"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(authDir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("recreated auth root = %v, %v", info, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStorePersistAuthFilesRejectsDatabaseFoldCollision(t *testing.T) {
	store, mock, authDir := newPostgresSecurityTestStore(t)
	id := "github/Alice.json"
	filePath := filepath.Join(authDir, filepath.FromSlash(id))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(`{"type":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	expectPostgresAuthMutationStart(mock)
	expectPostgresAuthFoldScan(mock, "github/alice.json")
	mock.ExpectRollback()
	if err := store.PersistAuthFiles(context.Background(), "", filePath); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("PersistAuthFiles collision error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
