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
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
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
	models := make([]string, 0, 6)
	pathModel := ""
	if r.URL != nil {
		models = append(models, r.URL.Query()["model"]...)
		pathModel = modelFromPath(r.URL.Path)
		models = append(models, pathModel)
	}
	queryAndPathModel, err := resolveModelSources(models)
	if err != nil {
		return "", 0, err
	}
	if r.Body == nil {
		if RequiresExplicitModel(r) && pathModel == "" {
			// Inference handlers read their model from the body. A query parameter
			// may conflict with that model, but is never sufficient by itself.
			return "", 0, nil
		}
		return queryAndPathModel, 0, nil
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
		if RequiresExplicitModel(r) && pathModel == "" {
			return "", 0, nil
		}
		return queryAndPathModel, 0, nil
	}

	// Inference handlers use GetRawData followed by gjson and do not select a
	// parser from Content-Type. Inspect the same raw bytes first so a misleading
	// content type or JSON with trailing garbage cannot hide the executed model.
	bodyModelFound := false
	if rawModel := gjson.GetBytes(data, "model"); rawModel.Exists() {
		models = append(models, rawModel.String())
		bodyModelFound = true
	}

	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mediaType {
	case "application/json", "":
		var payload any
		if err := json.Unmarshal(data, &payload); err != nil {
			model, resolveErr := resolveModelSources(models)
			if resolveErr != nil {
				return "", 0, resolveErr
			}
			if !bodyModelFound && pathModel == "" && RequiresExplicitModel(r) {
				return "", 0, errors.New("inference request body does not expose a model")
			}
			return model, estimateBytes(len(data)), nil
		}
		bodyModel, count, err := topLevelJSONModel(data)
		if err != nil {
			return "", 0, fmt.Errorf("inspect request model for caller policy: %w", err)
		}
		if count > 1 {
			return "", 0, &DecisionError{Code: CodeModelConflict}
		}
		if count > 0 {
			bodyModelFound = true
		}
		model, err := resolveModelSources(append(models, bodyModel))
		if err != nil {
			return "", 0, err
		}
		if !bodyModelFound && pathModel == "" && RequiresExplicitModel(r) {
			return "", 0, errors.New("inference request body does not expose a model")
		}
		return model, estimateJSONTokens(payload, ""), nil
	case "multipart/form-data":
		boundary := params["boundary"]
		var textBytes int
		if boundary != "" {
			reader := multipart.NewReader(bytes.NewReader(data), boundary)
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
					bodyModelFound = true
				} else if name == "prompt" || name == "input" || name == "instructions" {
					textBytes += len(value)
				}
			}
		}
		model, err := resolveModelSources(models)
		if err == nil && !bodyModelFound && pathModel == "" && RequiresExplicitModel(r) {
			err = errors.New("inference request body does not expose a model")
		}
		return model, estimateBytes(textBytes), err
	case "application/x-www-form-urlencoded":
		values, parseErr := url.ParseQuery(string(data))
		if parseErr != nil {
			return "", 0, fmt.Errorf("inspect urlencoded request for caller policy: %w", parseErr)
		}
		for _, formModel := range values["model"] {
			models = append(models, formModel)
			bodyModelFound = true
		}
		model, err := resolveModelSources(models)
		if err == nil && !bodyModelFound && pathModel == "" && RequiresExplicitModel(r) {
			err = errors.New("inference request body does not expose a model")
		}
		return model, estimateFormTokens(values), err
	default:
		model, resolveErr := resolveModelSources(models)
		if resolveErr != nil {
			return "", 0, resolveErr
		}
		if !bodyModelFound && pathModel == "" && RequiresExplicitModel(r) {
			return "", 0, errors.New("inference request body does not expose a model")
		}
		return model, estimateBytes(len(data)), nil
	}
}

func estimateFormTokens(values url.Values) int64 {
	textBytes := 0
	for _, field := range []string{"prompt", "input", "instructions"} {
		for _, value := range values[field] {
			textBytes += len(value)
		}
	}
	return estimateBytes(textBytes)
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
