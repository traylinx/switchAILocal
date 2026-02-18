// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"strings"
	"testing"
)

func TestMaskSensitiveBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMask bool // true if we expect masking to happen
		check    func(t *testing.T, input, output string)
	}{
		{
			name:     "JSON API Key",
			input:    `{"api_key": "sk-1234567890abcdef"}`,
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "sk-1234567890abcdef") {
					t.Errorf("API key was not masked")
				}
				if !strings.Contains(output, "sk-1...cdef") {
					t.Errorf("API key masking format incorrect: %s", output)
				}
			},
		},
		{
			name:     "JSON Password",
			input:    `{"password": "superSecretPassword"}`,
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "superSecretPassword") {
					t.Errorf("Password was not masked")
				}
				if !strings.Contains(output, "******") {
					t.Errorf("Password should be masked with stars")
				}
			},
		},
		{
			name:     "Form Data Password",
			input:    "username=admin&password=superSecretPassword&action=login",
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "superSecretPassword") {
					t.Errorf("Form password was not masked")
				}
				if !strings.Contains(output, "password=******") {
					t.Errorf("Expected password=******, got: %s", output)
				}
				if !strings.Contains(output, "username=admin") {
					t.Errorf("Other fields should be preserved")
				}
			},
		},
		{
			name:     "Form Data API Key",
			input:    "api_key=sk-1234567890abcdef&model=gpt-4",
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "sk-1234567890abcdef") {
					t.Errorf("Form API key was not masked")
				}
				if !strings.Contains(output, "api_key=sk-1...cdef") {
					t.Errorf("API key masking format incorrect: %s", output)
				}
			},
		},
		{
			name:     "Mixed JSON with URL query",
			input:    `{"url": "https://api.example.com?token=secretToken123&v=1"}`,
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "secretToken123") {
					t.Errorf("Token in URL was not masked")
				}
				// Should be masked by the form regex
				if !strings.Contains(output, "token=secr...n123") {
					t.Errorf("Token masking incorrect: %s", output)
				}
			},
		},
		{
			name:     "JSON with spaces",
			input:    `{ "secret" : "hiddenValue" }`,
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "hiddenValue") {
					t.Errorf("JSON with spaces was not masked")
				}
				if !strings.Contains(output, "******") {
					t.Errorf("Secret should be masked with stars")
				}
			},
		},
		{
			name:     "Form Data at End",
			input:    "foo=bar&secret=endOfLine",
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "endOfLine") {
					t.Errorf("Secret at end of line was not masked")
				}
				if !strings.Contains(output, "secret=******") {
					t.Errorf("Expected secret=******")
				}
			},
		},
		{
			name:     "Form Data URL Encoded",
			input:    "password=my%20secret%20pass&other=val",
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "my%20secret%20pass") {
					t.Errorf("Encoded password was not masked")
				}
				if !strings.Contains(output, "password=******") {
					t.Errorf("Expected password=******")
				}
			},
		},
		{
			name:     "XML-like Attribute",
			input:    `<user password="mySecret" />`,
			wantMask: true,
			check: func(t *testing.T, input, output string) {
				if strings.Contains(output, "mySecret") {
					t.Errorf("XML attribute was not masked")
				}
				if !strings.Contains(output, "password=******") {
					t.Errorf("Expected password=******, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := MaskSensitiveBody([]byte(tt.input))
			outputStr := string(output)

			if tt.wantMask && outputStr == tt.input {
				t.Errorf("Input was not masked at all")
			}

			if tt.check != nil {
				tt.check(t, tt.input, outputStr)
			}
		})
	}
}

// Ensure backward compatibility
func TestMaskSensitiveJSONBody_Compat(t *testing.T) {
	input := `{"password": "oldSecret"}`
	output := MaskSensitiveJSONBody([]byte(input))
	if strings.Contains(string(output), "oldSecret") {
		t.Errorf("Deprecated function failed to mask")
	}
}
