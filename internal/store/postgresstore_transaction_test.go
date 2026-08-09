// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
)

type postgresTxCaptureConnector struct {
	mu        sync.Mutex
	options   driver.TxOptions
	lockQuery string
	lockKey   int64
}

func (connector *postgresTxCaptureConnector) Connect(context.Context) (driver.Conn, error) {
	return &postgresTxCaptureConn{connector: connector}, nil
}

func (*postgresTxCaptureConnector) Driver() driver.Driver { return postgresTxCaptureDriver{} }

type postgresTxCaptureDriver struct{}

func (postgresTxCaptureDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("postgres tx capture driver requires OpenDB")
}

type postgresTxCaptureConn struct {
	connector *postgresTxCaptureConnector
}

func (*postgresTxCaptureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (*postgresTxCaptureConn) Close() error { return nil }

func (conn *postgresTxCaptureConn) Begin() (driver.Tx, error) {
	return conn.BeginTx(context.Background(), driver.TxOptions{})
}

func (conn *postgresTxCaptureConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	conn.connector.mu.Lock()
	conn.connector.options = options
	conn.connector.mu.Unlock()
	return postgresTxCaptureTx{}, nil
}

func (conn *postgresTxCaptureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if len(args) != 1 {
		return nil, errors.New("unexpected advisory-lock argument count")
	}
	key, ok := args[0].Value.(int64)
	if !ok {
		return nil, errors.New("unexpected advisory-lock key type")
	}
	conn.connector.mu.Lock()
	conn.connector.lockQuery = query
	conn.connector.lockKey = key
	conn.connector.mu.Unlock()
	return driver.RowsAffected(1), nil
}

type postgresTxCaptureTx struct{}

func (postgresTxCaptureTx) Commit() error   { return nil }
func (postgresTxCaptureTx) Rollback() error { return nil }

func TestPostgresStoreAuthMutationPinsIsolationAndAdvisoryKey(t *testing.T) {
	connector := &postgresTxCaptureConnector{}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })
	store := &PostgresStore{db: db}
	tx, err := store.beginAuthMutation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	connector.mu.Lock()
	options := connector.options
	lockQuery := connector.lockQuery
	lockKey := connector.lockKey
	connector.mu.Unlock()
	if options.Isolation != driver.IsolationLevel(sql.LevelReadCommitted) || options.ReadOnly {
		t.Fatalf("tx options = %#v; want READ COMMITTED read-write", options)
	}
	if lockQuery != "SELECT pg_advisory_xact_lock($1)" {
		t.Fatalf("lock query = %q", lockQuery)
	}
	const wantLockKey int64 = -5817299818233539318
	if postgresAuthMutationLockKey != wantLockKey || lockKey != wantLockKey {
		t.Fatalf("lock keys: constant=%d call=%d want=%d", postgresAuthMutationLockKey, lockKey, wantLockKey)
	}
}
