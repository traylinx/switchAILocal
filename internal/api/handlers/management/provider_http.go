// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func parseProviderHTTPURL(rawURL, field string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("%s is empty", field)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%s is malformed", field)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https", field)
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%s must include a host", field)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%s must not include URL userinfo", field)
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must not include a fragment", field)
	}
	return parsed, nil
}

func inferProviderModelsURL(rawBaseURL string) (*url.URL, error) {
	baseURL, err := parseProviderHTTPURL(rawBaseURL, "baseUrl")
	if err != nil {
		return nil, err
	}
	endpoint := "/models"
	if strings.Contains(strings.ToLower(rawBaseURL), "ollama") || baseURL.Port() == "11434" {
		endpoint = "/api/tags"
	}
	baseURL.Path = strings.TrimSuffix(baseURL.Path, "/") + endpoint
	if baseURL.RawPath != "" {
		baseURL.RawPath = strings.TrimSuffix(baseURL.RawPath, "/") + endpoint
	}
	return baseURL, nil
}

func providerUsesOpenAIBearerAuth(providerURL *url.URL) bool {
	hostname := strings.ToLower(providerURL.Hostname())
	return hostname == "openai.com" || strings.HasSuffix(hostname, ".openai.com")
}

func providerHTTPClient(timeout time.Duration, proxyURL *url.URL) *http.Client {
	transport := http.DefaultTransport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		clone := defaultTransport.Clone()
		if proxyURL != nil {
			clone.Proxy = http.ProxyURL(proxyURL)
		}
		transport = clone
	} else if proxyURL != nil {
		transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: validateProviderRedirect,
	}
}

func validateProviderRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("provider redirect limit exceeded")
	}
	_, err := parseProviderHTTPURL(req.URL.String(), "redirect URL")
	return err
}
