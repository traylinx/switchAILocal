// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package failover

import (
	"errors"
	"fmt"
)

// FailoverError is the typed error the conductor wraps an executor failure
// in once it has been classified. It is the only error type the structured
// failover log line (P4) and any future P1 retry policy need to inspect.
//
// It implements:
//
//	error           — to flow through normal error paths
//	Unwrap() error  — so errors.Is / errors.As reach the original cause
//	StatusCode()    — so the existing statusCodeFromError helper still works
//
// FailoverError MUST NOT be created speculatively by executors; it is the
// conductor's job to classify after the fact. Executors keep returning the
// raw upstream error; the conductor wraps once at the boundary.
type FailoverError struct {
	Class    ErrorClass
	Provider string
	HTTPCode int
	Wrapped  error
}

// Error implements the error interface.
func (e *FailoverError) Error() string {
	if e == nil {
		return ""
	}
	if e.Wrapped == nil {
		return fmt.Sprintf("failover[%s/%s]: status=%d", e.Provider, e.Class, e.HTTPCode)
	}
	return fmt.Sprintf("failover[%s/%s]: status=%d: %s", e.Provider, e.Class, e.HTTPCode, e.Wrapped.Error())
}

// Unwrap returns the underlying error so errors.Is and errors.As keep
// working transparently.
func (e *FailoverError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Wrapped
}

// StatusCode preserves the HTTP-status interface contract so existing code
// paths (handlers.go status extraction, conductor cooldown logic) keep
// behaving identically when the wrapped error is a FailoverError.
func (e *FailoverError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.HTTPCode
}

// AsFailoverError extracts a *FailoverError from anywhere in the error
// chain. Returns (nil, false) if not present.
func AsFailoverError(err error) (*FailoverError, bool) {
	if err == nil {
		return nil, false
	}
	var fe *FailoverError
	if errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}
