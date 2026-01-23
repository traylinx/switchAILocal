// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package switchailocal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/api"
	"github.com/traylinx/switchAILocal/internal/discovery"
	"github.com/traylinx/switchAILocal/internal/discovery/parsers"
	"github.com/traylinx/switchAILocal/internal/plugin"
	"github.com/traylinx/switchAILocal/internal/registry"
	"github.com/traylinx/switchAILocal/sdk/config"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

// Run starts the service and blocks until the context is cancelled or the server stops.
// It initializes all components including authentication, file watching, HTTP server,
// and starts processing requests. The method blocks until the context is cancelled.
//
// Parameters:
//   - ctx: The context for controlling the service lifecycle
//
// Returns:
//   - error: An error if the service fails to start or run
func (s *Service) Run(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("switchailocal: service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	usage.StartDefault(ctx)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	defer func() {
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Errorf("service shutdown returned error: %v", err)
		}
	}()

	s.registerBuiltinExecutors()

	if err := s.ensureAuthDir(); err != nil {
		return err
	}

	s.applyRetryConfig(s.cfg)

	if s.coreManager != nil {
		if errLoad := s.coreManager.Load(ctx); errLoad != nil {
			log.Warnf("failed to load auth store: %v", errLoad)
		}
	}

	tokenResult, err := s.tokenProvider.Load(ctx, s.cfg)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	_ = tokenResult

	apiKeyResult, err := s.apiKeyProvider.Load(ctx, s.cfg)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	_ = apiKeyResult

	// Initialize LUA plugin engine
	pluginCfg := plugin.Config{
		Enabled:   s.cfg.Plugin.Enabled,
		PluginDir: s.cfg.Plugin.PluginDir,
	}
	s.luaEngine = plugin.NewLuaEngine(pluginCfg)

	// Initialize model discovery
	cacheDir := filepath.Join(s.cfg.AuthDir, "cache", "discovery")
	disc, err := discovery.NewDiscoverer(cacheDir)
	if err != nil {
		log.WithError(err).Warn("Failed to initialize model discoverer, discovery disabled")
	} else {
		s.discoverer = disc

		// Register dynamic sources from config
		for _, entry := range s.cfg.SwitchAIKey {
			if entry.ModelsURL != "" {
				authHeader := ""
				if entry.APIKey != "" {
					authHeader = "Bearer " + entry.APIKey
				}
				disc.AddSource(discovery.SourceConfig{
					ProviderID: "switchai",
					URL:        entry.ModelsURL,
					SourceType: "api",
					TTLSeconds: discovery.DefaultAPITTL,
					Parser:     parsers.NewOpenAIParser("switchai"),
					AuthHeader: authHeader,
				})
			}
		}

		for _, compat := range s.cfg.OpenAICompatibility {
			if compat.ModelsURL != "" {
				authHeader := ""
				if len(compat.APIKeyEntries) > 0 {
					authHeader = "Bearer " + compat.APIKeyEntries[0].APIKey
				}
				disc.AddSource(discovery.SourceConfig{
					ProviderID: strings.ToLower(compat.Name),
					URL:        compat.ModelsURL,
					SourceType: "api",
					TTLSeconds: discovery.DefaultAPITTL,
					Parser:     parsers.NewOpenAIParser(strings.ToLower(compat.Name)),
					AuthHeader: authHeader,
				})
			}
		}

		if s.cfg.Ollama.Enabled && s.cfg.Ollama.AutoDiscover {
			url := s.cfg.Ollama.BaseURL
			if url == "" {
				url = "http://localhost:11434"
			}
			url = strings.TrimSuffix(url, "/") + "/api/tags"
			disc.AddSource(discovery.SourceConfig{
				ProviderID: "ollama",
				URL:        url,
				SourceType: "api",
				TTLSeconds: discovery.DefaultAPITTL,
				Parser:     parsers.NewOllamaParser(),
			})
		}

		if s.cfg.OpenCode.Enabled {
			url := s.cfg.OpenCode.BaseURL
			if url == "" {
				url = "http://localhost:4096"
			}
			disc.AddSource(discovery.SourceConfig{
				ProviderID: "opencode",
				URL:        strings.TrimSuffix(url, "/") + "/agent",
				SourceType: "api",
				TTLSeconds: discovery.DefaultAPITTL,
				Parser:     parsers.NewOpenCodeParser(url),
			})
		}

		// Run discovery in background to not block startup
		go func() {
			discCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			results, discErr := disc.DiscoverAll(discCtx)
			if discErr != nil {
				log.WithError(discErr).Warn("Model discovery failed")
				return
			}
			totalModels := 0
			modelReg := registry.GetGlobalRegistry()
			for provider, models := range results {
				log.WithField("provider", provider).WithField("count", len(models)).Info("Discovered models")

				// Use the original discovery provider IDs to keep CLI and API separate
				regProvider := provider

				modelReg.RegisterClient("discovery-"+provider, regProvider, models)
				totalModels += len(models)
			}
			log.WithField("total", totalModels).Info("Model discovery complete")
		}()
	}

	// handlers no longer depend on legacy clients; pass nil slice initially
	s.server = api.NewServer(s.cfg, s.coreManager, s.accessManager, s.configPath, s.luaEngine, s.serverOptions...)

	// Wire discoverer for /v1/models/refresh endpoint
	if s.discoverer != nil && s.server != nil {
		s.server.SetDiscoverer(s.discoverer)
	}

	if s.authManager == nil {
		s.authManager = newDefaultAuthManager()
	}

	s.ensureWebsocketGateway()
	if s.server != nil && s.wsGateway != nil {
		s.server.AttachWebsocketRoute(s.wsGateway.Path(), s.wsGateway.Handler())
		s.server.SetWebsocketAuthChangeHandler(func(oldEnabled, newEnabled bool) {
			if oldEnabled == newEnabled {
				return
			}
			if !oldEnabled && newEnabled {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if errStop := s.wsGateway.Stop(ctx); errStop != nil {
					log.Warnf("failed to reset websocket connections after ws-auth change %t -> %t: %v", oldEnabled, newEnabled, errStop)
					return
				}
				log.Debugf("ws-auth enabled; existing websocket sessions terminated to enforce authentication")
				return
			}
			log.Debugf("ws-auth disabled; existing websocket sessions remain connected")
		})
	}

	if s.hooks.OnBeforeStart != nil {
		s.hooks.OnBeforeStart(s.cfg)
	}

	s.serverErr = make(chan error, 1)
	go func() {
		if errStart := s.server.Start(); errStart != nil {
			s.serverErr <- errStart
		} else {
			s.serverErr <- nil
		}
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Printf("API server started successfully on: %s:%d\n", s.cfg.Host, s.cfg.Port)

	if s.hooks.OnAfterStart != nil {
		s.hooks.OnAfterStart(s)
	}

	var watcherWrapper *WatcherWrapper
	reloadCallback := func(newCfg *config.Config) {
		previousStrategy := ""
		s.cfgMu.RLock()
		if s.cfg != nil {
			previousStrategy = strings.ToLower(strings.TrimSpace(s.cfg.Routing.Strategy))
		}
		s.cfgMu.RUnlock()

		if newCfg == nil {
			s.cfgMu.RLock()
			newCfg = s.cfg
			s.cfgMu.RUnlock()
		}
		if newCfg == nil {
			return
		}

		nextStrategy := strings.ToLower(strings.TrimSpace(newCfg.Routing.Strategy))
		normalizeStrategy := func(strategy string) string {
			switch strategy {
			case "fill-first", "fillfirst", "ff":
				return "fill-first"
			default:
				return "round-robin"
			}
		}
		previousStrategy = normalizeStrategy(previousStrategy)
		nextStrategy = normalizeStrategy(nextStrategy)
		if s.coreManager != nil && previousStrategy != nextStrategy {
			var selector auth.Selector
			switch nextStrategy {
			case "fill-first":
				selector = &auth.FillFirstSelector{}
			default:
				selector = &auth.RoundRobinSelector{}
			}
			s.coreManager.SetSelector(selector)
			log.Infof("routing strategy updated to %s", nextStrategy)
		}

		s.applyRetryConfig(newCfg)
		if s.server != nil {
			s.server.UpdateClients(newCfg)
		}
		s.cfgMu.Lock()
		s.cfg = newCfg
		s.cfgMu.Unlock()
		s.rebindExecutors()
	}

	watcherWrapper, err = s.watcherFactory(s.configPath, s.cfg.AuthDir, reloadCallback)
	if err != nil {
		return fmt.Errorf("switchailocal: failed to create watcher: %w", err)
	}
	s.watcher = watcherWrapper
	s.ensureAuthUpdateQueue(ctx)
	if s.authUpdates != nil {
		watcherWrapper.SetAuthUpdateQueue(s.authUpdates)
	}
	watcherWrapper.SetConfig(s.cfg)

	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	s.watcherCancel = watcherCancel
	if err = watcherWrapper.Start(watcherCtx); err != nil {
		return fmt.Errorf("switchailocal: failed to start watcher: %w", err)
	}
	log.Info("file watcher started for config and auth directory changes")

	// Prefer core auth manager auto refresh if available.
	if s.coreManager != nil {
		interval := 15 * time.Minute
		s.coreManager.StartAutoRefresh(context.Background(), interval)
		log.Infof("core auth auto-refresh started (interval=%s)", interval)
	}

	select {
	case <-ctx.Done():
		log.Debug("service context cancelled, shutting down...")
		return ctx.Err()
	case err = <-s.serverErr:
		return err
	}
}

// Shutdown gracefully stops background workers and the HTTP server.
// It ensures all resources are properly cleaned up and connections are closed.
// The shutdown is idempotent and can be called multiple times safely.
//
// Parameters:
//   - ctx: The context for controlling the shutdown timeout
//
// Returns:
//   - error: An error if shutdown fails
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var shutdownErr error
	s.shutdownOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}

		if s.watcherCancel != nil {
			s.watcherCancel()
		}
		if s.coreManager != nil {
			s.coreManager.StopAutoRefresh()
		}
		if s.watcher != nil {
			if err := s.watcher.Stop(); err != nil {
				log.Errorf("failed to stop file watcher: %v", err)
				shutdownErr = err
			}
		}
		if s.wsGateway != nil {
			if err := s.wsGateway.Stop(ctx); err != nil {
				log.Errorf("failed to stop websocket gateway: %v", err)
				if shutdownErr == nil {
					shutdownErr = err
				}
			}
		}
		if s.authQueueStop != nil {
			s.authQueueStop()
			s.authQueueStop = nil
		}

		if s.server != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := s.server.Stop(shutdownCtx); err != nil {
				log.Errorf("error stopping API server: %v", err)
				if shutdownErr == nil {
					shutdownErr = err
				}
			}
		}

		usage.StopDefault()
	})
	return shutdownErr
}

func (s *Service) ensureAuthDir() error {
	info, err := os.Stat(s.cfg.AuthDir)
	if err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(s.cfg.AuthDir, 0o755); mkErr != nil {
				return fmt.Errorf("switchailocal: failed to create auth directory %s: %w", s.cfg.AuthDir, mkErr)
			}
			log.Infof("created missing auth directory: %s", s.cfg.AuthDir)
			return nil
		}
		return fmt.Errorf("switchailocal: error checking auth directory %s: %w", s.cfg.AuthDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("switchailocal: auth path exists but is not a directory: %s", s.cfg.AuthDir)
	}
	return nil
}
