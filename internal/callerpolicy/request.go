// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

// InspectRequest extracts the requested model and a conservative input-token
// estimate while restoring the body byte-for-byte for the downstream handler.
// The body content is never retained by the policy manager or logged.
func InspectRequest(r *http.Request) (string, int64, error) {
	if r == nil {
		return "", 0, nil
	}
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if model == "" {
		model = modelFromPath(r.URL.Path)
	}
	if r.Body == nil {
		return model, 0, nil
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read request body for caller policy: %w", err)
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	if len(data) == 0 {
		return model, 0, nil
	}

	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mediaType {
	case "application/json", "":
		var payload any
		if err := json.Unmarshal(data, &payload); err != nil {
			// The downstream handler owns malformed-payload errors. Preserve the
			// body and use its size only as a fallback accounting estimate.
			return model, estimateBytes(len(data)), nil
		}
		if model == "" {
			if object, ok := payload.(map[string]any); ok {
				if configured, ok := object["model"].(string); ok {
					model = strings.TrimSpace(configured)
				}
			}
		}
		return model, estimateJSONTokens(payload, ""), nil
	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return model, 0, nil
		}
		reader := multipart.NewReader(bytes.NewReader(data), boundary)
		var textBytes int
		for {
			part, nextErr := reader.NextPart()
			if nextErr == io.EOF {
				break
			}
			if nextErr != nil {
				break
			}
			name := part.FormName()
			if part.FileName() != "" {
				_ = part.Close()
				continue
			}
			value, _ := io.ReadAll(io.LimitReader(part, 1<<20))
			_ = part.Close()
			if name == "model" && model == "" {
				model = strings.TrimSpace(string(value))
			} else if name == "prompt" || name == "input" || name == "instructions" {
				textBytes += len(value)
			}
		}
		return model, estimateBytes(textBytes), nil
	default:
		return model, estimateBytes(len(data)), nil
	}
}

// RequiresExplicitModel reports whether the route can invoke a model and must
// therefore name it before an allowlist decision can be made. Non-inference
// routes such as model catalogs, provider status, and notification relays stay
// available without inventing a model identity.
func RequiresExplicitModel(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	path := r.URL.Path
	if strings.HasPrefix(path, "/v1beta/models/") {
		return true
	}
	for _, prefix := range []string{"/v1", "/openai/v1"} {
		if !strings.HasPrefix(path, prefix+"/") {
			continue
		}
		relative := strings.TrimPrefix(path, prefix)
		switch relative {
		case "/chat/completions", "/completions", "/embeddings",
			"/images/generations", "/images/edits", "/audio/transcriptions",
			"/audio/translations", "/audio/speech", "/music/generations",
			"/music/lyrics", "/messages", "/messages/count_tokens", "/responses":
			return true
		}
	}
	return false
}

func modelFromPath(path string) string {
	const marker = "/models/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	model := path[idx+len(marker):]
	if end := strings.IndexAny(model, ":/"); end >= 0 {
		model = model[:end]
	}
	return strings.TrimSpace(model)
}

func estimateBytes(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64((n + 3) / 4)
}

func estimateJSONTokens(value any, field string) int64 {
	var bytesCount int
	var visit func(any, string)
	visit = func(current any, key string) {
		switch typed := current.(type) {
		case map[string]any:
			for childKey, child := range typed {
				visit(child, childKey)
			}
		case []any:
			for _, child := range typed {
				visit(child, key)
			}
		case string:
			switch strings.ToLower(key) {
			case "model", "url", "image_url", "audio_url", "response_format":
				return
			default:
				bytesCount += len(typed)
			}
		}
	}
	visit(value, field)
	return estimateBytes(bytesCount)
}
