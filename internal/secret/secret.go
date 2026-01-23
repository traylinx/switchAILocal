// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package secret

import "os"

// GetEnv returns the value of the environment variable named by the key,
// or fallback if the variable is not present.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Gemini Client Credentials
func GeminiClientID() string {
	return GetEnv("GEMINI_CLIENT_ID", "")
}

func GeminiClientSecret() string {
	return GetEnv("GEMINI_CLIENT_SECRET", "")
}

// iFlow Client Credentials
func IFlowClientID() string {
	return GetEnv("IFLOW_CLIENT_ID", "")
}

func IFlowClientSecret() string {
	return GetEnv("IFLOW_CLIENT_SECRET", "")
}

// Qwen Client Credentials
func QwenClientID() string {
	return GetEnv("QWEN_CLIENT_ID", "")
}

// Antigravity Client Credentials
func AntigravityClientID() string {
	return GetEnv("ANTIGRAVITY_CLIENT_ID", "")
}

func AntigravityClientSecret() string {
	return GetEnv("ANTIGRAVITY_CLIENT_SECRET", "")
}
