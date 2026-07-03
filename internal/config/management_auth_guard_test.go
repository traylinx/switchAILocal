// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"testing"
)

// writeTempConfig writes body to a temp yaml file and returns its path.
func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	f, err := os.CreateTemp("", "config_authguard_*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// The management middleware bypasses auth when the secret is empty. Starting with
// allow-remote: true in that state exposes the management API to any remote host,
// so LoadConfig must refuse it.
func TestLoadConfig_RejectsRemoteManagementWithoutSecret(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	path := writeTempConfig(t, "remote-management:\n  allow-remote: true\n  secret-key: \"\"\n")

	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for allow-remote with no secret, got nil")
	}
}

func TestLoadConfig_AllowsRemoteManagementWithSecret(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	path := writeTempConfig(t, "remote-management:\n  allow-remote: true\n  secret-key: \"s3cret-value\"\n")

	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("expected no error when secret-key is set, got: %v", err)
	}
}

func TestLoadConfig_AllowsRemoteManagementWithEnvPassword(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "from-env")
	path := writeTempConfig(t, "remote-management:\n  allow-remote: true\n  secret-key: \"\"\n")

	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("expected no error when MANAGEMENT_PASSWORD is set, got: %v", err)
	}
}

// The localhost-only default (allow-remote: false) must remain usable with no secret.
func TestLoadConfig_AllowsLocalhostDefaultWithoutSecret(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	path := writeTempConfig(t, "remote-management:\n  allow-remote: false\n  secret-key: \"\"\n")

	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("expected no error for localhost default, got: %v", err)
	}
}
