// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSyncAuthFromDatabase(t *testing.T) {
	// Create a temporary directory for spooling
	tmpDir, err := os.MkdirTemp("", "pgstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	cfg := PostgresStoreConfig{
		AuthTable: "auth_store",
		SpoolDir:  tmpDir,
	}

	store := &PostgresStore{
		db:        db,
		cfg:       cfg,
		spoolRoot: tmpDir,
		authDir:   filepath.Join(tmpDir, "auths"),
	}

	// Setup expectations
	// New implementation uses batching. We expect a query with ORDER BY id LIMIT 100.
	// Since we return 2 rows (less than 100), it should stop after one query.
	rows := sqlmock.NewRows([]string{"id", "content"}).
		AddRow("auth1", `{"id":"auth1","type":"test"}`).
		AddRow("auth2", `{"id":"auth2","type":"test"}`)

	query := fmt.Sprintf("SELECT id, content FROM %s ORDER BY id LIMIT 100", store.fullTableName(cfg.AuthTable))
	mock.ExpectQuery(query).WillReturnRows(rows)

	// Run sync
	err = store.syncAuthFromDatabase(context.Background())
	if err != nil {
		t.Errorf("error was not expected while updating stats: %s", err)
	}

	// Verify expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}

	// Verify files were created
	files, err := os.ReadDir(store.authDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestSyncAuthFromDatabase_Pagination(t *testing.T) {
	// Create a temporary directory for spooling
	tmpDir, err := os.MkdirTemp("", "pgstore-test-pagination")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	cfg := PostgresStoreConfig{
		AuthTable: "auth_store",
		SpoolDir:  tmpDir,
	}

	store := &PostgresStore{
		db:        db,
		cfg:       cfg,
		spoolRoot: tmpDir,
		authDir:   filepath.Join(tmpDir, "auths"),
	}

	// Batch size is 100. We mock 101 items to trigger 2 queries.
	// Query 1: LIMIT 100. Returns 100 items. Last ID "auth100".
	// Query 2: WHERE id > "auth100" LIMIT 100. Returns 1 item.

	// Rows 1
	rows1 := sqlmock.NewRows([]string{"id", "content"})
	for i := 1; i <= 100; i++ {
		// Use padded numbers to ensure lexical order matches numeric order for simplicity
		id := fmt.Sprintf("auth%03d", i)
		rows1.AddRow(id, fmt.Sprintf(`{"id":"%s"}`, id))
	}

	// Rows 2
	rows2 := sqlmock.NewRows([]string{"id", "content"}).
		AddRow("auth101", `{"id":"auth101"}`)

	query1 := fmt.Sprintf("SELECT id, content FROM %s ORDER BY id LIMIT 100", store.fullTableName(cfg.AuthTable))
	mock.ExpectQuery(query1).WillReturnRows(rows1)

	query2 := fmt.Sprintf("SELECT id, content FROM %s WHERE id > \\$1 ORDER BY id LIMIT 100", store.fullTableName(cfg.AuthTable))
	// Note: sqlmock uses regex matching. $1 needs escaping in regex if it was interpreted as such, but ExpectQuery takes a string which is converted to regex?
	// Wait, ExpectQuery expects a regex string. $ is end of line anchor. So $1 needs to be escaped.
	// Actually sqlmock usually escapes the expected string for us? No, we provide regex.
	// "WHERE id > $1" -> regex `WHERE id > \$1`

	mock.ExpectQuery(query2).WithArgs("auth100").WillReturnRows(rows2)

	// Run sync
	err = store.syncAuthFromDatabase(context.Background())
	if err != nil {
		t.Errorf("error was not expected: %s", err)
	}

	// Verify expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}

	// Verify files were created
	files, err := os.ReadDir(store.authDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 101 {
		t.Errorf("expected 101 files, got %d", len(files))
	}
}
