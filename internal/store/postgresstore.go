// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	_ "github.com/jackc/pgx/v5/stdlib"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/authid"
	"github.com/traylinx/switchAILocal/internal/misc"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

const (
	defaultConfigTable          = "config_store"
	defaultAuthTable            = "auth_store"
	defaultConfigKey            = "config"
	postgresAuthMutationLockKey = int64(-5817299818233539318) // SHA-256("switchailocal.auth.foldkey")[:8]
)

// DBExecutor defines the common interface for sql.DB and sql.Tx.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// PostgresStoreConfig captures configuration required to initialize a Postgres-backed store.
type PostgresStoreConfig struct {
	DSN         string
	Schema      string
	ConfigTable string
	AuthTable   string
	SpoolDir    string
}

// PostgresStore persists configuration and authentication metadata using PostgreSQL as backend
// while mirroring data to a local workspace so existing file-based workflows continue to operate.
type PostgresStore struct {
	db         *sql.DB
	cfg        PostgresStoreConfig
	spoolRoot  string
	configPath string
	authDir    string
	mu         sync.Mutex
}

// NewPostgresStore establishes a connection to PostgreSQL and prepares the local workspace.
func NewPostgresStore(ctx context.Context, cfg PostgresStoreConfig) (*PostgresStore, error) {
	trimmedDSN := strings.TrimSpace(cfg.DSN)
	if trimmedDSN == "" {
		return nil, fmt.Errorf("postgres store: DSN is required")
	}
	cfg.DSN = trimmedDSN
	if cfg.ConfigTable == "" {
		cfg.ConfigTable = defaultConfigTable
	}
	if cfg.AuthTable == "" {
		cfg.AuthTable = defaultAuthTable
	}

	spoolRoot := strings.TrimSpace(cfg.SpoolDir)
	if spoolRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			spoolRoot = filepath.Join(cwd, "pgstore")
		} else {
			spoolRoot = filepath.Join(os.TempDir(), "pgstore")
		}
	}
	absSpool, err := filepath.Abs(spoolRoot)
	if err != nil {
		return nil, fmt.Errorf("postgres store: resolve spool directory: %w", err)
	}
	configDir := filepath.Join(absSpool, "config")
	authDir := filepath.Join(absSpool, "auths")
	if err = os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("postgres store: create config directory: %w", err)
	}
	if err = os.MkdirAll(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("postgres store: create auth directory: %w", err)
	}

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres store: open database connection: %w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres store: ping database: %w", err)
	}

	store := &PostgresStore{
		db:         db,
		cfg:        cfg,
		spoolRoot:  absSpool,
		configPath: filepath.Join(configDir, "config.yaml"),
		authDir:    authDir,
	}
	return store, nil
}

// Close releases the underlying database connection.
func (s *PostgresStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// EnsureSchema creates the required tables (and schema when provided).
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres store: not initialized")
	}
	if schema := strings.TrimSpace(s.cfg.Schema); schema != "" {
		query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(schema))
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("postgres store: create schema: %w", err)
		}
	}
	configTable := s.fullTableName(s.cfg.ConfigTable)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, configTable)); err != nil {
		return fmt.Errorf("postgres store: create config table: %w", err)
	}
	authTable := s.fullTableName(s.cfg.AuthTable)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, authTable)); err != nil {
		return fmt.Errorf("postgres store: create auth table: %w", err)
	}
	return nil
}

// Bootstrap synchronizes configuration and auth records between PostgreSQL and the local workspace.
func (s *PostgresStore) Bootstrap(ctx context.Context, exampleConfigPath string) error {
	if err := s.EnsureSchema(ctx); err != nil {
		return err
	}
	// Run the data migration only inside Bootstrap, where the following database-to-
	// mirror sync removes any legacy-named credential files before rehydration.
	if err := s.migrateLegacyAuthIDs(ctx); err != nil {
		return err
	}
	if err := s.syncConfigFromDatabase(ctx, exampleConfigPath); err != nil {
		return err
	}
	if err := s.syncAuthFromDatabase(ctx); err != nil {
		return err
	}
	return nil
}

// ConfigPath returns the managed configuration file path inside the spool directory.
func (s *PostgresStore) ConfigPath() string {
	if s == nil {
		return ""
	}
	return s.configPath
}

// AuthDir returns the local directory containing mirrored auth files.
func (s *PostgresStore) AuthDir() string {
	if s == nil {
		return ""
	}
	return s.authDir
}

// WorkDir exposes the root spool directory used for mirroring.
func (s *PostgresStore) WorkDir() string {
	if s == nil {
		return ""
	}
	return s.spoolRoot
}

// SetBaseDir implements the optional interface used by authenticators; it is a no-op because
// the Postgres-backed store controls its own workspace.
func (s *PostgresStore) SetBaseDir(string) {}

// Save persists authentication metadata to disk and PostgreSQL.
func (s *PostgresStore) Save(ctx context.Context, auth *switchailocalauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("postgres store: auth is nil")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := openPostgresAuthRoot(s.authDir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	if err = sweepStoredAuthTemps(ctx, root, time.Now()); err != nil {
		log.WithError(err).Warn("postgres store: stale auth temp cleanup incomplete")
	}

	id, err := resolveStoredAuthID(auth, s.authDir)
	if err != nil {
		return "", err
	}
	exists, err := validateStoredAuthPath(root, id)
	if err != nil {
		return "", err
	}
	filePath, err := authid.ToFSPath(s.authDir, id)
	if err != nil {
		return "", fmt.Errorf("postgres store: resolve auth id: %w", err)
	}
	var tx *sql.Tx
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	if auth.Disabled && !exists {
		tx, err = s.beginAuthMutation(ctx)
		if err != nil {
			return "", err
		}
		databaseExists, errExists := s.databaseAuthIDExists(ctx, tx, id)
		if errExists != nil {
			return "", errExists
		}
		if !databaseExists {
			return "", nil
		}
	}
	raw, err := marshalStoredAuth(auth)
	if err != nil {
		return "", err
	}
	writeRequired := raw != nil
	if raw == nil {
		// EmptyStorage intentionally has no file or database representation.
		setStoredAuthLocation(auth, id, filePath)
		return filePath, nil
	}
	rootID := filepath.FromSlash(id)
	info, errLstat := root.Lstat(rootID)
	switch {
	case errLstat == nil && info.Mode()&os.ModeSymlink != 0:
		// A contained final symlink is replaced as a link. An escaping link is
		// rejected before the database mutation rather than followed or replaced.
		if _, errStat := root.Stat(rootID); errStat != nil && !errors.Is(errStat, fs.ErrNotExist) {
			return "", fmt.Errorf("postgres store: inspect final symlink target: %w", errStat)
		}
		writeRequired = true
	case auth.Metadata != nil && auth.Storage == nil && errLstat == nil:
		existing, errRead := root.ReadFile(rootID)
		if errRead != nil {
			return "", fmt.Errorf("postgres store: read existing metadata: %w", errRead)
		}
		writeRequired = !jsonEqual(existing, raw) || info.Mode().Perm() != 0o600
	case errLstat != nil && !errors.Is(errLstat, fs.ErrNotExist):
		return "", fmt.Errorf("postgres store: inspect existing metadata: %w", errLstat)
	}
	if tx == nil {
		tx, err = s.beginAuthMutation(ctx)
		if err != nil {
			return "", err
		}
	}
	if err = s.ensureDatabaseAuthIDAvailable(ctx, tx, id); err != nil {
		return "", err
	}
	if err = s.persistAuth(ctx, tx, id, raw); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("postgres store: commit auth save: %w", err)
	}
	if writeRequired {
		if err = writeStoredAuthAtomic(root, id, raw); err != nil {
			return "", fmt.Errorf("postgres store: database committed auth %q but mirror update failed: %w", id, err)
		}
	}
	setStoredAuthLocation(auth, id, filePath)
	return filePath, nil
}

// List enumerates all auth records stored in PostgreSQL.
func (s *PostgresStore) List(ctx context.Context) ([]*switchailocalauth.Auth, error) {
	query := fmt.Sprintf(`SELECT id, content, created_at, updated_at FROM %s ORDER BY id COLLATE "C"`, s.fullTableName(s.cfg.AuthTable))
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres store: list auth: %w", err)
	}
	defer rows.Close()

	auths := make([]*switchailocalauth.Auth, 0, 32)
	seenFoldKeys := make(map[string]string)
	for rows.Next() {
		var (
			id        string
			payload   []byte
			createdAt time.Time
			updatedAt time.Time
		)
		if err = rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("postgres store: scan auth row: %w", err)
		}
		canonicalID, errID := canonicalStoredDatabaseAuthID(id)
		if errID != nil {
			log.WithError(errID).Warnf("postgres store: skipping invalid auth id %s", id)
			continue
		}
		filePath, errPath := authid.ToFSPath(s.authDir, canonicalID)
		if errPath != nil {
			log.WithError(errPath).Warnf("postgres store: skipping auth %s outside spool", id)
			continue
		}
		metadata := make(map[string]any)
		if err = json.Unmarshal(payload, &metadata); err != nil {
			log.WithError(err).Warnf("postgres store: skipping auth %s with invalid json", id)
			continue
		}
		if errID = rememberStoredDatabaseAuthID(seenFoldKeys, canonicalID); errID != nil {
			log.WithError(errID).Warnf("postgres store: skipping colliding auth id %s", id)
			continue
		}
		provider := strings.TrimSpace(valueAsString(metadata["type"]))
		if provider == "" {
			provider = "unknown"
		}
		attr := map[string]string{"path": filePath}
		if email := strings.TrimSpace(valueAsString(metadata["email"])); email != "" {
			attr["email"] = email
		}
		auth := &switchailocalauth.Auth{
			ID:               canonicalID,
			Provider:         provider,
			FileName:         canonicalID,
			Label:            labelFor(metadata),
			Status:           switchailocalauth.StatusActive,
			Attributes:       attr,
			Metadata:         metadata,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
			LastRefreshedAt:  time.Time{},
			NextRefreshAfter: time.Time{},
		}
		auths = append(auths, auth)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres store: iterate auth rows: %w", err)
	}
	return auths, nil
}

// Delete removes an auth file and the corresponding database record.
func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("postgres store: id is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := openPostgresAuthRoot(s.authDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	canonicalID, err := canonicalStoredAuthID(s.authDir, id)
	if err != nil {
		return fmt.Errorf("postgres store: invalid delete id: %w", err)
	}
	if _, err = validateStoredAuthPath(root, canonicalID); err != nil {
		return err
	}
	tx, err := s.beginAuthMutation(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.deleteAuthRecord(ctx, tx, canonicalID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("postgres store: commit auth delete: %w", err)
	}
	if err = root.Remove(filepath.FromSlash(canonicalID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("postgres store: database deleted auth %q but mirror removal failed: %w", canonicalID, err)
	}
	return nil
}

// PersistAuthFiles stores the provided auth file changes in PostgreSQL.
func (s *PostgresStore) PersistAuthFiles(ctx context.Context, _ string, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := openPostgresAuthRoot(s.authDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	tx, err := s.beginAuthMutation(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		id, err := canonicalStoredAuthID(s.authDir, trimmed)
		if err != nil {
			log.WithError(err).Warnf("postgres store: ignoring auth path %s", trimmed)
			continue
		}
		if _, err = validateStoredAuthPath(root, id); err != nil {
			log.WithError(err).Warnf("postgres store: ignoring unsafe auth path %s", trimmed)
			continue
		}
		if err = s.syncAuthFile(ctx, tx, root, id); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("postgres store: commit transaction: %w", err)
	}
	return nil
}

// PersistConfig mirrors the local configuration file to PostgreSQL.
func (s *PostgresStore) PersistConfig(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s.deleteConfigRecord(ctx, s.db)
		}
		return fmt.Errorf("postgres store: read config file: %w", err)
	}
	return s.persistConfig(ctx, s.db, data)
}

// syncConfigFromDatabase writes the database-stored config to disk or seeds the database from template.
func (s *PostgresStore) syncConfigFromDatabase(ctx context.Context, exampleConfigPath string) error {
	query := fmt.Sprintf("SELECT content FROM %s WHERE id = $1", s.fullTableName(s.cfg.ConfigTable))
	var content string
	err := s.db.QueryRowContext(ctx, query, defaultConfigKey).Scan(&content)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, errStat := os.Stat(s.configPath); errors.Is(errStat, fs.ErrNotExist) {
			if exampleConfigPath != "" {
				if errCopy := misc.CopyConfigTemplate(exampleConfigPath, s.configPath); errCopy != nil {
					return fmt.Errorf("postgres store: copy example config: %w", errCopy)
				}
			} else {
				if errCreate := os.MkdirAll(filepath.Dir(s.configPath), 0o700); errCreate != nil {
					return fmt.Errorf("postgres store: prepare config directory: %w", errCreate)
				}
				if errWrite := os.WriteFile(s.configPath, []byte{}, 0o600); errWrite != nil {
					return fmt.Errorf("postgres store: create empty config: %w", errWrite)
				}
			}
		}
		data, errRead := os.ReadFile(s.configPath)
		if errRead != nil {
			return fmt.Errorf("postgres store: read local config: %w", errRead)
		}
		if errPersist := s.persistConfig(ctx, s.db, data); errPersist != nil {
			return errPersist
		}
	case err != nil:
		return fmt.Errorf("postgres store: load config from database: %w", err)
	default:
		if err = os.MkdirAll(filepath.Dir(s.configPath), 0o700); err != nil {
			return fmt.Errorf("postgres store: prepare config directory: %w", err)
		}
		normalized := normalizeLineEndings(content)
		if err = os.WriteFile(s.configPath, []byte(normalized), 0o600); err != nil {
			return fmt.Errorf("postgres store: write config to spool: %w", err)
		}
	}
	return nil
}

// syncAuthFromDatabase populates the local auth directory from PostgreSQL data.
func (s *PostgresStore) syncAuthFromDatabase(ctx context.Context) error {
	if err := os.RemoveAll(s.authDir); err != nil {
		return fmt.Errorf("postgres store: reset auth directory: %w", err)
	}
	if err := os.MkdirAll(s.authDir, 0o700); err != nil {
		return fmt.Errorf("postgres store: recreate auth directory: %w", err)
	}
	root, err := openPostgresAuthRoot(s.authDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	const batchSize = 100
	var lastID string
	firstBatch := true
	seenFoldKeys := make(map[string]string)

	for {
		var (
			rows *sql.Rows
			err  error
		)

		if firstBatch {
			query := fmt.Sprintf(`SELECT id, content FROM %s ORDER BY id COLLATE "C" LIMIT %d`, s.fullTableName(s.cfg.AuthTable), batchSize)
			rows, err = s.db.QueryContext(ctx, query)
		} else {
			query := fmt.Sprintf(`SELECT id, content FROM %s WHERE id COLLATE "C" > $1 ORDER BY id COLLATE "C" LIMIT %d`, s.fullTableName(s.cfg.AuthTable), batchSize)
			rows, err = s.db.QueryContext(ctx, query, lastID)
		}

		if err != nil {
			return fmt.Errorf("postgres store: load auth from database: %w", err)
		}

		count := 0
		for rows.Next() {
			count++
			var (
				id      string
				payload string
			)
			if err = rows.Scan(&id, &payload); err != nil {
				rows.Close()
				return fmt.Errorf("postgres store: scan auth row: %w", err)
			}
			lastID = id

			canonicalID, errID := canonicalStoredDatabaseAuthID(id)
			if errID != nil {
				log.WithError(errID).Warnf("postgres store: skipping invalid auth id %s", id)
				continue
			}
			if errID = rememberStoredDatabaseAuthID(seenFoldKeys, canonicalID); errID != nil {
				log.WithError(errID).Warnf("postgres store: skipping colliding auth id %s", id)
				continue
			}
			if _, err = validateStoredAuthPath(root, canonicalID); err != nil {
				log.WithError(err).Warnf("postgres store: skipping unsafe auth mirror path %s", id)
				continue
			}
			if err = writeStoredAuthAtomic(root, canonicalID, []byte(payload)); err != nil {
				rows.Close()
				return fmt.Errorf("postgres store: write auth file: %w", err)
			}
		}

		if err = rows.Close(); err != nil {
			return fmt.Errorf("postgres store: close rows: %w", err)
		}
		if err = rows.Err(); err != nil {
			return fmt.Errorf("postgres store: iterate auth rows: %w", err)
		}

		if count < batchSize {
			break
		}
		firstBatch = false
	}
	return nil
}

func (s *PostgresStore) syncAuthFile(ctx context.Context, exec DBExecutor, root *os.Root, id string) error {
	data, err := root.ReadFile(filepath.FromSlash(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s.deleteAuthRecord(ctx, exec, id)
		}
		return fmt.Errorf("postgres store: read auth file: %w", err)
	}
	if len(data) == 0 {
		return s.deleteAuthRecord(ctx, exec, id)
	}
	if err = s.ensureDatabaseAuthIDAvailable(ctx, exec, id); err != nil {
		return err
	}
	return s.persistAuth(ctx, exec, id, data)
}

func (s *PostgresStore) beginAuthMutation(ctx context.Context) (*sql.Tx, error) {
	// READ COMMITTED is mandatory: after waiting for the advisory lock, the
	// following collision scan must see mutations committed by the prior holder.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("postgres store: begin auth mutation: %w", err)
	}
	if err = lockPostgresAuthMutations(ctx, tx); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (s *PostgresStore) migrateLegacyAuthIDs(ctx context.Context) error {
	tx, err := s.beginAuthMutation(ctx)
	if err != nil {
		return fmt.Errorf("postgres store: begin legacy auth id migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := fmt.Sprintf(`SELECT id FROM %s ORDER BY id COLLATE "C"`, s.fullTableName(s.cfg.AuthTable))
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgres store: query legacy auth ids: %w", err)
	}
	ids := make([]string, 0, 32)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("postgres store: scan legacy auth id: %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil {
		return fmt.Errorf("postgres store: close legacy auth ids: %w", err)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("postgres store: iterate legacy auth ids: %w", err)
	}

	type migrationCandidate struct {
		from, to       string
		foldKey        string
		needsMigration bool
	}
	candidates := make([]migrationCandidate, 0, len(ids))
	candidatesByFoldKey := make(map[string][]migrationCandidate, len(ids))
	for _, id := range ids {
		targetID, errID := canonicalStoredDatabaseAuthID(id)
		needsMigration := errID != nil
		if errID != nil {
			targetID, errID = migratedLegacyAuthID(id)
			if errID != nil {
				log.WithError(errID).Warnf("postgres store: cannot migrate invalid database auth id %s", id)
				continue
			}
		}
		foldKey, errFold := authid.FoldKey(targetID)
		if errFold != nil {
			return fmt.Errorf("postgres store: fold migrated auth id %q: %w", targetID, errFold)
		}
		candidate := migrationCandidate{from: id, to: targetID, foldKey: foldKey, needsMigration: needsMigration}
		candidates = append(candidates, candidate)
		candidatesByFoldKey[foldKey] = append(candidatesByFoldKey[foldKey], candidate)
	}

	migrations := make([]migrationCandidate, 0)
	warnedFoldKeys := make(map[string]bool)
	for _, candidate := range candidates {
		group := candidatesByFoldKey[candidate.foldKey]
		if len(group) > 1 {
			if !warnedFoldKeys[candidate.foldKey] {
				sources := make([]string, 0, len(group))
				for _, item := range group {
					sources = append(sources, item.from)
				}
				log.Warnf("postgres store: database auth ids %q are portable aliases; leaving ambiguous legacy rows unmigrated", sources)
				warnedFoldKeys[candidate.foldKey] = true
			}
			continue
		}
		if candidate.needsMigration {
			migrations = append(migrations, candidate)
		}
	}

	update := fmt.Sprintf("UPDATE %s SET id = $1 WHERE id = $2", s.fullTableName(s.cfg.AuthTable))
	for _, item := range migrations {
		result, errExec := tx.ExecContext(ctx, update, item.to, item.from)
		if errExec != nil {
			return fmt.Errorf("postgres store: migrate auth id %q to %q: %w", item.from, item.to, errExec)
		}
		affected, errAffected := result.RowsAffected()
		if errAffected != nil {
			return fmt.Errorf("postgres store: count migrated auth id %q: %w", item.from, errAffected)
		}
		if affected != 1 {
			return fmt.Errorf("postgres store: migrate auth id %q affected %d rows, want 1", item.from, affected)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("postgres store: commit legacy auth id migration: %w", err)
	}
	return nil
}

func lockPostgresAuthMutations(ctx context.Context, exec DBExecutor) error {
	// Row locks do not block inserts of a new, fold-colliding ID. A single
	// transaction-scoped lock serializes the small global auth-ID set across replicas.
	if _, err := exec.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", postgresAuthMutationLockKey); err != nil {
		return fmt.Errorf("postgres store: lock auth mutations: %w", err)
	}
	return nil
}

func (s *PostgresStore) databaseAuthIDExists(ctx context.Context, exec DBExecutor, id string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)", s.fullTableName(s.cfg.AuthTable))
	var exists bool
	if err := exec.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres store: check auth id existence: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) ensureDatabaseAuthIDAvailable(ctx context.Context, exec DBExecutor, id string) error {
	wantKey, err := authid.FoldKey(id)
	if err != nil {
		return fmt.Errorf("postgres store: fold auth id: %w", err)
	}
	query := fmt.Sprintf("SELECT id FROM %s", s.fullTableName(s.cfg.AuthTable))
	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgres store: query auth ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var existingID string
		if err = rows.Scan(&existingID); err != nil {
			return fmt.Errorf("postgres store: scan auth id: %w", err)
		}
		if existingID == id {
			continue
		}
		existingCanonicalID, errExisting := canonicalStoredDatabaseAuthID(existingID)
		if errExisting != nil {
			existingCanonicalID, errExisting = migratedLegacyAuthID(existingID)
			if errExisting != nil {
				log.WithError(errExisting).Warnf("postgres store: ignoring invalid database auth id %s during collision check", existingID)
				continue
			}
		}
		existingKey, errFold := authid.FoldKey(existingCanonicalID)
		if errFold != nil {
			log.WithError(errFold).Warnf("postgres store: cannot fold database auth id %s during collision check", existingID)
			continue
		}
		if existingKey == wantKey {
			return fmt.Errorf("postgres store: auth id %q collides with database id %q", id, existingID)
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("postgres store: iterate auth ids: %w", err)
	}
	return nil
}

func openPostgresAuthRoot(authDir string) (*os.Root, error) {
	if authDir == "" {
		return nil, fmt.Errorf("postgres store: auth directory not configured")
	}
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("postgres store: create auth directory: %w", err)
	}
	info, err := os.Lstat(authDir)
	if err != nil {
		return nil, fmt.Errorf("postgres store: inspect auth directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("postgres store: auth directory must not be a symlink")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("postgres store: auth directory is not a directory")
	}
	root, err := os.OpenRoot(authDir)
	if err != nil {
		return nil, fmt.Errorf("postgres store: open auth root: %w", err)
	}
	// Credential spools are private by contract, including pre-existing roots.
	if err = root.Chmod(".", 0o700); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("postgres store: secure auth root: %w", err)
	}
	return root, nil
}

func (s *PostgresStore) persistAuth(ctx context.Context, exec DBExecutor, relID string, data []byte) error {
	jsonPayload := json.RawMessage(data)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, content, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (id)
		DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()
	`, s.fullTableName(s.cfg.AuthTable))
	if _, err := exec.ExecContext(ctx, query, relID, jsonPayload); err != nil {
		return fmt.Errorf("postgres store: upsert auth record: %w", err)
	}
	return nil
}

func (s *PostgresStore) deleteAuthRecord(ctx context.Context, exec DBExecutor, relID string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", s.fullTableName(s.cfg.AuthTable))
	if _, err := exec.ExecContext(ctx, query, relID); err != nil {
		return fmt.Errorf("postgres store: delete auth record: %w", err)
	}
	return nil
}

func (s *PostgresStore) persistConfig(ctx context.Context, exec DBExecutor, data []byte) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (id, content, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (id)
		DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()
	`, s.fullTableName(s.cfg.ConfigTable))
	normalized := normalizeLineEndings(string(data))
	if _, err := exec.ExecContext(ctx, query, defaultConfigKey, normalized); err != nil {
		return fmt.Errorf("postgres store: upsert config: %w", err)
	}
	return nil
}

func (s *PostgresStore) deleteConfigRecord(ctx context.Context, exec DBExecutor) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", s.fullTableName(s.cfg.ConfigTable))
	if _, err := exec.ExecContext(ctx, query, defaultConfigKey); err != nil {
		return fmt.Errorf("postgres store: delete config: %w", err)
	}
	return nil
}

func canonicalStoredDatabaseAuthID(id string) (string, error) {
	if filepath.IsAbs(id) {
		return "", fmt.Errorf("postgres store: database auth id must be relative")
	}
	if err := authid.Validate(id); err != nil {
		return "", err
	}
	base := filepath.Base(filepath.FromSlash(id))
	if len(base) <= len(".json") || !strings.HasSuffix(strings.ToLower(base), ".json") {
		return "", fmt.Errorf("auth id must end in .json")
	}
	return id, nil
}

func migratedLegacyAuthID(id string) (string, error) {
	if filepath.IsAbs(id) {
		return "", fmt.Errorf("postgres store: database auth id must be relative")
	}
	if err := authid.Validate(id); err != nil {
		return "", err
	}
	base := filepath.Base(filepath.FromSlash(id))
	if len(base) > len(".json") && strings.HasSuffix(strings.ToLower(base), ".json") {
		return "", fmt.Errorf("postgres store: database auth id %q already has a JSON suffix", id)
	}
	return canonicalStoredDatabaseAuthID(id + ".json")
}

func rememberStoredDatabaseAuthID(seen map[string]string, id string) error {
	key, err := authid.FoldKey(id)
	if err != nil {
		return err
	}
	if existingID, ok := seen[key]; ok && existingID != id {
		return fmt.Errorf("database auth id %q collides with %q", id, existingID)
	}
	seen[key] = id
	return nil
}

func (s *PostgresStore) fullTableName(name string) string {
	if strings.TrimSpace(s.cfg.Schema) == "" {
		return quoteIdentifier(name)
	}
	return quoteIdentifier(s.cfg.Schema) + "." + quoteIdentifier(name)
}

func quoteIdentifier(identifier string) string {
	replaced := strings.ReplaceAll(identifier, "\"", "\"\"")
	return "\"" + replaced + "\""
}

func valueAsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func labelFor(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v := strings.TrimSpace(valueAsString(metadata["label"])); v != "" {
		return v
	}
	if v := strings.TrimSpace(valueAsString(metadata["email"])); v != "" {
		return v
	}
	if v := strings.TrimSpace(valueAsString(metadata["project_id"])); v != "" {
		return v
	}
	return ""
}

func normalizeAuthID(id string) string {
	return filepath.ToSlash(filepath.Clean(id))
}

func normalizeLineEndings(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
