// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPFetcher implements the discovery.Fetcher interface using standard HTTP.
type HTTPFetcher struct {
	client  *http.Client
	headers map[string]string
}

// NewHTTPFetcher creates a new fetcher with a default 5-second timeout.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		headers: make(map[string]string),
	}
}

// SetHeader sets a default header for all fetch requests.
func (f *HTTPFetcher) SetHeader(key, value string) {
	if f.headers == nil {
		f.headers = make(map[string]string)
	}
	f.headers[key] = value
}

// Fetch retrieves the content from the given URL.
func (f *HTTPFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	return f.FetchWithAuth(ctx, url, "")
}

// FetchWithAuth retrieves the content from the given URL with an optional Authorization header.
func (f *HTTPFetcher) FetchWithAuth(ctx context.Context, url string, authHeader string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	req.Header.Set("User-Agent", "switchAILocal/1.0 (internal-discovery)")
	for k, v := range f.headers {
		req.Header.Set(k, v)
	}

	// Set override auth header if provided
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %d %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}
