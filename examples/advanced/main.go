// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package main demonstrates how to create a custom AI provider executor
// and integrate it with the switchAILocal server. This example shows how to:
// - Create a custom executor that implements the Executor interface
// - Register custom translators for request/response transformation
// - Integrate the custom provider with the SDK server
// - Register custom models in the model registry
//
// This example uses a simple echo service (httpbin.org) as the upstream API
// for demonstration purposes. In a real implementation, you would replace
// this with your actual AI service provider.
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	configaccess "github.com/traylinx/switchAILocal/internal/access/config_access"
	"github.com/traylinx/switchAILocal/sdk/api"
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
	"github.com/traylinx/switchAILocal/sdk/config"
	"github.com/traylinx/switchAILocal/sdk/logging"
	"github.com/traylinx/switchAILocal/sdk/switchailocal"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	clipexec "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
	sdktr "github.com/traylinx/switchAILocal/sdk/translator"
)

const (
	// providerKey is the identifier for our custom provider.
	providerKey = "myprov"

	// fOpenAI represents the OpenAI chat format.
	fOpenAI = sdktr.Format("openai.chat")

	// fMyProv represents our custom provider's chat format.
	fMyProv = sdktr.Format("myprov.chat")
)

// init registers trivial translators for demonstration purposes.
// In a real implementation, you would implement proper request/response
// transformation logic between OpenAI format and your provider's format.
func init() {
	sdktr.Register(fOpenAI, fMyProv,
		func(model string, raw []byte, stream bool) []byte { return raw },
		sdktr.ResponseTransform{
			Stream: func(ctx context.Context, model string, originalReq, translatedReq, raw []byte, param *any) []string {
				return []string{string(raw)}
			},
			NonStream: func(ctx context.Context, model string, originalReq, translatedReq, raw []byte, param *any) string {
				return string(raw)
			},
		},
	)
}

// MyExecutor is a minimal provider implementation for demonstration purposes.
// It implements the Executor interface to handle requests to a custom AI provider.
type MyExecutor struct{}

// Identifier returns the unique identifier for this executor.
func (MyExecutor) Identifier() string { return providerKey }

// PrepareRequest optionally injects credentials to raw HTTP requests.
// This method is called before each request to allow the executor to modify
// the HTTP request with authentication headers or other necessary modifications.
//
// Parameters:
//   - req: The HTTP request to prepare
//   - a: The authentication information
//
// Returns:
//   - error: An error if request preparation fails
func (MyExecutor) PrepareRequest(req *http.Request, a *coreauth.Auth) error {
	if req == nil || a == nil {
		return nil
	}
	if a.Attributes != nil {
		if ak := strings.TrimSpace(a.Attributes["api_key"]); ak != "" {
			req.Header.Set("Authorization", "Bearer "+ak)
		}
	}
	return nil
}

func buildHTTPClient(a *coreauth.Auth) *http.Client {
	if a == nil || strings.TrimSpace(a.ProxyURL) == "" {
		return http.DefaultClient
	}
	u, err := url.Parse(a.ProxyURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return http.DefaultClient
	}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
}

func upstreamEndpoint(a *coreauth.Auth) string {
	if a != nil && a.Attributes != nil {
		if ep := strings.TrimSpace(a.Attributes["endpoint"]); ep != "" {
			return ep
		}
	}
	// Demo echo endpoint; replace with your upstream.
	return "https://httpbin.org/post"
}

func (MyExecutor) Execute(ctx context.Context, a *coreauth.Auth, req clipexec.Request, opts clipexec.Options) (clipexec.Response, error) {
	client := buildHTTPClient(a)
	endpoint := upstreamEndpoint(a)

	httpReq, errNew := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(req.Payload))
	if errNew != nil {
		return clipexec.Response{}, errNew
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Inject credentials via PrepareRequest hook.
	_ = (MyExecutor{}).PrepareRequest(httpReq, a)

	resp, errDo := client.Do(httpReq)
	if errDo != nil {
		return clipexec.Response{}, errDo
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			// Best-effort close; log if needed in real projects.
		}
	}()
	body, _ := io.ReadAll(resp.Body)
	return clipexec.Response{Payload: body}, nil
}

func (MyExecutor) CountTokens(context.Context, *coreauth.Auth, clipexec.Request, clipexec.Options) (clipexec.Response, error) {
	return clipexec.Response{}, errors.New("count tokens not implemented")
}

func (MyExecutor) ExecuteStream(ctx context.Context, a *coreauth.Auth, req clipexec.Request, opts clipexec.Options) (<-chan clipexec.StreamChunk, error) {
	ch := make(chan clipexec.StreamChunk, 1)
	go func() {
		defer close(ch)
		ch <- clipexec.StreamChunk{Payload: []byte("data: {\"ok\":true}\n\n")}
	}()
	return ch, nil
}

func (MyExecutor) Refresh(ctx context.Context, a *coreauth.Auth) (*coreauth.Auth, error) {
	return a, nil
}

func main() {
	// Try to load local example config first, fall back to "config.yaml" or defaults
	configFile := "example-config.yaml"
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		configFile = "config.yaml"
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		// Fallback to minimal default config if file missing
		log.Printf("Config not found, using defaults: %v", err)
		cfg = &config.Config{
			Host: "127.0.0.1",
			Port: 8081,
		}
	}

	tokenStore := sdkAuth.GetTokenStore()
	if dirSetter, ok := tokenStore.(interface{ SetBaseDir(string) }); ok {
		dirSetter.SetBaseDir(cfg.AuthDir)
	}

	// Register default accessors to ensure the builder succeeds
	// This fixes "provider type not registered" errors
	configaccess.Register()

	core := coreauth.NewManager(tokenStore, nil, nil)
	core.RegisterExecutor(MyExecutor{})

	hooks := switchailocal.Hooks{
		OnAfterStart: func(s *switchailocal.Service) {
			// Register demo models for the custom provider so they appear in /v1/models.
			models := []*switchailocal.ModelInfo{{ID: "myprov-pro-1", Object: "model", Type: providerKey, DisplayName: "MyProv Pro 1"}}
			for _, a := range core.List() {
				if strings.EqualFold(a.Provider, providerKey) {
					switchailocal.GlobalModelRegistry().RegisterClient(a.ID, providerKey, models)
				}
			}
		},
	}

	svc, err := switchailocal.NewBuilder().
		WithConfig(cfg).
		WithConfigPath("config.yaml").
		WithCoreAuthManager(core).
		WithServerOptions(
			// Optional: add a simple middleware + custom request logger
			api.WithMiddleware(func(c *gin.Context) { c.Header("X-Example", "custom-provider"); c.Next() }),
			api.WithRequestLoggerFactory(func(cfg *config.Config, cfgPath string) logging.RequestLogger {
				return logging.NewFileRequestLogger(true, "logs", filepath.Dir(cfgPath))
			}),
		).
		WithHooks(hooks).
		Build()
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		panic(err)
	}
	_ = os.Stderr // keep os import used (demo only)
	_ = time.Second
}
