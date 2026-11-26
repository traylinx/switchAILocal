// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

// MaxInspectedBodyBytes bounds the duplicate policy-inspection copy. Requests
// larger than this are refused before they can consume unbounded process memory.
const MaxInspectedBodyBytes int64 = 16 << 20

// InspectRequest extracts the requested model and a conservative input-token
// estimate while restoring the body byte-for-byte for the downstream handler.
// The body content is never retained by the policy manager or logged.
func InspectRequest(r *http.Request) (string, int64, error) {
	if r == nil {
		return "", 0, nil
	}
	models := make([]string, 0, 4)
	if r.URL != nil {
		models = append(models, r.URL.Query()["model"]...)
		models = append(models, modelFromPath(r.URL.Path))
	}
	model, err := resolveModelSources(models)
	if err != nil {
		return "", 0, err
	}
	if r.Body == nil {
		return model, 0, nil
	}
	limited := http.MaxBytesReader(nil, r.Body, MaxInspectedBodyBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		_ = r.Body.Close()
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
		bodyModel, count, err := topLevelJSONModel(data)
		if err != nil {
			return "", 0, fmt.Errorf("inspect request model for caller policy: %w", err)
		}
		if count > 1 {
			return "", 0, &DecisionError{Code: CodeModelConflict}
		}
		model, err = resolveModelSources(append(models, bodyModel))
		if err != nil {
			return "", 0, err
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
			if name == "model" {
				models = append(models, string(value))
			} else if name == "prompt" || name == "input" || name == "instructions" {
				textBytes += len(value)
			}
		}
		model, err = resolveModelSources(models)
		return model, estimateBytes(textBytes), err
	default:
		return model, estimateBytes(len(data)), nil
	}
}

// RequiresExplicitModel defaults to requiring a model for policed callers.
// Only the explicit read-only catalog routes below are safe without one; new
// and provider-specific routes therefore fail closed until classified.
func RequiresExplicitModel(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return true
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/v1/models", "/openai/v1/models", "/v1beta/models", "/v1/providers":
		return false
	}
	// Amp exposes provider-specific model catalogs at both /models and
	// /v1/models. Exact suffix matching avoids treating Gemini model actions as
	// catalogs.
	return !strings.HasPrefix(path, "/api/provider/") || !strings.HasSuffix(path, "/models")
}

func resolveModelSources(sources []string) (string, error) {
	model := ""
	for _, source := range sources {
		candidate := strings.TrimSpace(source)
		if candidate == "" {
			continue
		}
		if model == "" {
			model = candidate
			continue
		}
		if candidate != model {
			return "", &DecisionError{Code: CodeModelConflict}
		}
	}
	return model, nil
}

// topLevelJSONModel preserves the first-match semantics used by downstream
// handlers while rejecting duplicate top-level model keys altogether.
func topLevelJSONModel(data []byte) (string, int, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return "", 0, err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return "", 0, nil
	}
	model := ""
	count := 0
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return "", 0, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return "", 0, errors.New("non-string JSON object key")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return "", 0, err
		}
		if key != "model" {
			continue
		}
		count++
		if count == 1 {
			var configured string
			if json.Unmarshal(raw, &configured) == nil {
				model = strings.TrimSpace(configured)
			}
		}
	}
	return model, count, nil
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
