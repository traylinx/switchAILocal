// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	"golang.org/x/net/proxy"
)

// defaultPooledTransport is a shared, high-performance HTTP transport with connection
// pooling enabled. All non-proxy requests reuse this transport, enabling TCP/TLS
// connection reuse and HTTP/2 multiplexing across concurrent requests.
//
// Previously, every request created a new http.Client with DisableKeepAlives: true,
// forcing a fresh TCP+TLS handshake per request (~100-300ms overhead each time).
//
// Limits are set for high-concurrency proxy workloads (50+ parallel sessions):
//   - MaxConnsPerHost=100: prevents TCP-level queuing to a single API provider
//   - MaxIdleConnsPerHost=50: keeps warm connections ready for burst traffic
//   - MaxIdleConns=200: global ceiling across all providers
var defaultPooledTransport = &http.Transport{
	MaxIdleConns:        200,
	MaxIdleConnsPerHost: 50,
	MaxConnsPerHost:     100,
	IdleConnTimeout:     120 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
	ForceAttemptHTTP2:   true,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
}

// defaultHTTPClient is the shared http.Client for non-proxy requests.
// Go's http.Client is safe for concurrent use; sharing it enables connection pooling.
var defaultHTTPClient = &http.Client{
	Transport: defaultPooledTransport,
}

// proxyClientCache caches http.Client instances keyed by proxy URL.
// This avoids re-creating proxy transports for every request while still
// supporting per-auth and per-config proxy overrides.
var proxyClientCache sync.Map // map[string]*http.Client

// applyProviderTimeout wraps httpReq's context with a per-provider deadline
// resolved from cfg.Performance.ProviderTimeouts. The returned cancel func must
// be deferred by the caller. Safe to call with nil cfg; returns the request
// unchanged and a no-op cancel when no timeout applies.
//
// IMPORTANT: only use this on NON-streaming requests. For SSE streams, a
// context deadline would terminate healthy long-running bodies; use
// http.Transport.ResponseHeaderTimeout + the SSE stall watchdog instead.
func applyProviderTimeout(ctx context.Context, cfg *config.Config, provider string, req *http.Request) (*http.Request, context.CancelFunc) {
	if cfg == nil || req == nil {
		return req, func() {}
	}
	timeout := cfg.Performance.ProviderTimeouts.Resolve(provider)
	if timeout <= 0 {
		return req, func() {}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	return req.WithContext(reqCtx), cancel
}

// newProxyAwareHTTPClient returns an HTTP client with proper proxy configuration priority:
// 1. Use auth.ProxyURL if configured (highest priority)
// 2. Use cfg.ProxyURL if auth proxy is not configured
// 3. Use RoundTripper from context if neither are configured
// 4. Fall back to the shared pooled client (connection reuse + HTTP/2)
//
// Clients are cached by proxy URL to avoid per-request allocation overhead.
// The default (no-proxy) path returns a shared client with no allocations.
//
// Parameters:
//   - ctx: The context containing optional RoundTripper
//   - cfg: The application configuration
//   - auth: The authentication information
//   - timeout: The client timeout (0 means no timeout)
//
// Returns:
//   - *http.Client: An HTTP client with configured proxy or transport
func newProxyAwareHTTPClient(ctx context.Context, cfg *config.Config, auth *switchailocalauth.Auth, timeout time.Duration) *http.Client {
	// Priority 1: Use auth.ProxyURL if configured
	var proxyURL string
	if auth != nil {
		proxyURL = strings.TrimSpace(auth.ProxyURL)
	}

	// Priority 2: Use cfg.ProxyURL if auth proxy is not configured
	if proxyURL == "" && cfg != nil {
		proxyURL = strings.TrimSpace(cfg.ProxyURL)
	}

	// If we have a proxy URL configured, use a cached proxy-aware client
	if proxyURL != "" {
		return getOrCreateProxyClient(proxyURL, timeout)
	}

	// Priority 3: Use RoundTripper from context (typically from RoundTripperFor)
	if rt, ok := ctx.Value("switchailocal.roundtripper").(http.RoundTripper); ok && rt != nil {
		// Context-provided transports can't be cached (per-request), allocate minimally
		client := &http.Client{Transport: rt}
		if timeout > 0 {
			client.Timeout = timeout
		}
		return client
	}

	// Priority 4: Use the shared pooled client (no allocation, connection reuse)
	if timeout > 0 {
		// Only allocate a new client if a custom timeout is needed
		return &http.Client{
			Transport: defaultPooledTransport,
			Timeout:   timeout,
		}
	}
	return defaultHTTPClient
}

// getOrCreateProxyClient returns a cached http.Client for the given proxy URL,
// creating one if it doesn't exist. This ensures proxy transports are reused
// across requests to the same proxy, enabling connection pooling even through proxies.
func getOrCreateProxyClient(proxyURL string, timeout time.Duration) *http.Client {
	if cached, ok := proxyClientCache.Load(proxyURL); ok {
		client := cached.(*http.Client)
		if timeout > 0 && client.Timeout != timeout {
			// Timeout mismatch — create a new client sharing the same transport
			return &http.Client{Transport: client.Transport, Timeout: timeout}
		}
		return client
	}

	transport := buildPooledProxyTransport(proxyURL)
	if transport == nil {
		// Proxy setup failed, fall back to the shared pooled client
		log.Debugf("failed to setup proxy from URL: %s, falling back to pooled transport", proxyURL)
		return defaultHTTPClient
	}

	client := &http.Client{Transport: transport}
	if timeout > 0 {
		client.Timeout = timeout
	}
	proxyClientCache.Store(proxyURL, client)
	return client
}

// buildPooledProxyTransport creates an HTTP transport configured for the given proxy URL
// with connection pooling settings applied. It supports SOCKS5, HTTP, and HTTPS proxies.
//
// Unlike the previous buildProxyTransport, this version applies the same pooling parameters
// as defaultPooledTransport (MaxIdleConns, MaxConnsPerHost, etc.) to proxy transports,
// ensuring connection reuse even through proxies.
func buildPooledProxyTransport(proxyURL string) *http.Transport {
	if proxyURL == "" {
		return nil
	}

	parsedURL, errParse := url.Parse(proxyURL)
	if errParse != nil {
		log.Errorf("parse proxy URL failed: %v", errParse)
		return nil
	}

	var transport *http.Transport

	// Handle different proxy schemes
	if parsedURL.Scheme == "socks5" {
		// Configure SOCKS5 proxy with optional authentication
		var proxyAuth *proxy.Auth
		if parsedURL.User != nil {
			username := parsedURL.User.Username()
			password, _ := parsedURL.User.Password()
			proxyAuth = &proxy.Auth{User: username, Password: password}
		}
		dialer, errSOCKS5 := proxy.SOCKS5("tcp", parsedURL.Host, proxyAuth, proxy.Direct)
		if errSOCKS5 != nil {
			log.Errorf("create SOCKS5 dialer failed: %v", errSOCKS5)
			return nil
		}
		// Set up a custom transport using the SOCKS5 dialer with pooling
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 5,
			MaxConnsPerHost:     15,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		}
	} else if parsedURL.Scheme == "http" || parsedURL.Scheme == "https" {
		// Configure HTTP or HTTPS proxy with pooling
		transport = &http.Transport{
			Proxy:               http.ProxyURL(parsedURL),
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 5,
			MaxConnsPerHost:     15,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			ForceAttemptHTTP2:   true,
		}
	} else {
		log.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
		return nil
	}

	return transport
}
