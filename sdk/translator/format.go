// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package translator

// Format identifies a request/response schema used inside the proxy.
type Format string

// FromString converts an arbitrary identifier to a translator format.
func FromString(v string) Format {
	return Format(v)
}

// String returns the raw schema identifier.
func (f Format) String() string {
	return string(f)
}
