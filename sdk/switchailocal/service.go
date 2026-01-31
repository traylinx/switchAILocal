// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package switchailocal

import (
	"sync"

	"context"

	"github.com/traylinx/switchAILocal/internal/api"
	"github.com/traylinx/switchAILocal/internal/discovery"
	"github.com/traylinx/switchAILocal/internal/intelligence"
	"github.com/traylinx/switchAILocal/internal/plugin"
	"github.com/traylinx/switchAILocal/internal/watcher"
	"github.com/traylinx/switchAILocal/internal/wsrelay"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
	"github.com/traylinx/switchAILocal/sdk/config"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

// Service wraps the proxy server lifecycle so external programs can embed the CLI proxy.
// It manages the complete lifecycle including authentication, file watching, HTTP server,
// and integration with various AI service providers.
type Service struct {
	// cfg holds the current application configuration.
	cfg *config.Config

	// cfgMu protects concurrent access to the configuration.
	cfgMu sync.RWMutex

	// configPath is the path to the configuration file.
	configPath string

	// tokenProvider handles loading token-based clients.
	tokenProvider TokenClientProvider

	// apiKeyProvider handles loading API key-based clients.
	apiKeyProvider APIKeyClientProvider

	// watcherFactory creates file watcher instances.
	watcherFactory WatcherFactory

	// hooks provides lifecycle callbacks.
	hooks Hooks

	// serverOptions contains additional server configuration options.
	serverOptions []api.ServerOption

	// server is the HTTP API server instance.
	server *api.Server

	// serverErr channel for server startup/shutdown errors.
	serverErr chan error

	// watcher handles file system monitoring.
	watcher *WatcherWrapper

	// watcherCancel cancels the watcher context.
	watcherCancel context.CancelFunc

	// authUpdates channel for authentication updates.
	authUpdates chan watcher.AuthUpdate

	// authQueueStop cancels the auth update queue processing.
	authQueueStop context.CancelFunc

	// authManager handles legacy authentication operations.
	authManager *sdkAuth.Manager

	// accessManager handles request authentication providers.
	accessManager *sdkaccess.Manager

	// coreManager handles core authentication and execution.
	coreManager *coreauth.Manager

	// shutdownOnce ensures shutdown is called only once.
	shutdownOnce sync.Once

	// wsGateway manages websocket Gemini providers.
	wsGateway *wsrelay.Manager

	// luaEngine manages LUA plugins for request/response modification.
	luaEngine *plugin.LuaEngine

	// discoverer handles dynamic model discovery from GitHub and API sources.
	discoverer *discovery.Discoverer

	// intelligenceService manages Phase 2 intelligent routing features.
	intelligenceService *intelligence.Service
}

// CoreManager returns the underlying core authentication manager.
func (s *Service) CoreManager() *coreauth.Manager {
	return s.coreManager
}

// RegisterUsagePlugin registers a usage plugin on the global usage manager.
// This allows external code to monitor API usage and token consumption.
//
// Parameters:
//   - plugin: The usage plugin to register
func (s *Service) RegisterUsagePlugin(plugin usage.Plugin) {
	usage.RegisterPlugin(plugin)
}

// GetIntelligenceService returns the intelligence service instance.
// Returns nil if intelligence services are not enabled or not initialized.
//
// Returns:
//   - *intelligence.Service: The intelligence service instance, or nil
func (s *Service) GetIntelligenceService() *intelligence.Service {
	return s.intelligenceService
}

// newDefaultAuthManager creates a default authentication manager with all supported providers.
func newDefaultAuthManager() *sdkAuth.Manager {
	return sdkAuth.NewManager(
		sdkAuth.GetTokenStore(),
		sdkAuth.NewGeminiAuthenticator(),
		sdkAuth.NewCodexAuthenticator(),
		sdkAuth.NewClaudeAuthenticator(),
		sdkAuth.NewQwenAuthenticator(),
		sdkAuth.NewVibeAuthenticator(),
		sdkAuth.NewOllamaAuthenticator(),
	)
}
