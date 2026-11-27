// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package handlers

import (
	"bytes"
	"net/http"
	"testing"
)

func TestResolvedModelRefusalUsesCallerPolicyErrorShape(t *testing.T) {
	body := BuildErrorResponseBody(http.StatusForbidden, "caller_model_not_allowed")
	for _, field := range [][]byte{
		[]byte(`"type":"caller_policy_error"`),
		[]byte(`"code":"caller_model_not_allowed"`),
	} {
		if !bytes.Contains(body, field) {
			t.Fatalf("error body %s missing %s", body, field)
		}
	}
}
