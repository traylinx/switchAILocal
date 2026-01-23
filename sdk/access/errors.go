// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package access

import "errors"

var (
	// ErrNoCredentials indicates no recognizable credentials were supplied.
	ErrNoCredentials = errors.New("access: no credentials provided")
	// ErrInvalidCredential signals that supplied credentials were rejected by a provider.
	ErrInvalidCredential = errors.New("access: invalid credential")
	// ErrNotHandled tells the manager to continue trying other providers.
	ErrNotHandled = errors.New("access: not handled")
)
