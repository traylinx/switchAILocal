// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package cmd provides command-line interface functionality for the switchAILocal server.
// It includes authentication flows for various AI service providers, service startup,
// and other command-line operations.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/api"
	"github.com/traylinx/switchAILocal/internal/cli"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/runtime/executor"
	"github.com/traylinx/switchAILocal/sdk/switchailocal"
	sdkauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// StartService builds and runs the proxy service using the exported SDK.
// It creates a new proxy service instance, sets up signal handling for graceful shutdown,
// and starts the service with the provided configuration.
//
// Parameters:
//   - cfg: The application configuration
//   - configPath: The path to the configuration file
//   - localPassword: Optional password accepted for local management requests
func StartService(cfg *config.Config, configPath string, localPassword string) {
	builder := switchailocal.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(configPath).
		WithLocalManagementPassword(localPassword).
		WithHooks(switchailocal.Hooks{
			OnAfterStart: func(s *switchailocal.Service) {
				// CLI PROXY: Discover and register local tools
				tools := cli.DiscoverInstalledTools()
				for _, tool := range tools {
					// 1. Register Executor
					exec := executor.NewLocalCLIExecutor(tool)
					s.CoreManager().RegisterExecutor(exec)

					// 2. Register System Auth Record
					auth := &sdkauth.Auth{
						ID:       "cli-" + tool.Definition.ProviderKey,
						Provider: tool.Definition.ProviderKey,
						Status:   sdkauth.StatusActive,
						Label:    fmt.Sprintf("Local Code Proxy: %s", tool.Definition.Name),
						Metadata: map[string]any{
							"source":      "local_cli_discovery",
							"binary_path": tool.Path,
						},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}
					// Use background context for registration
					if _, err := s.CoreManager().Register(context.Background(), auth); err != nil {
						log.Errorf("Failed to register system auth for %s: %v", tool.Definition.Name, err)
					} else {
						log.Infof("Registered local CLI proxy and auth for %s (%s)", tool.Definition.Name, tool.Definition.ProviderKey)
					}
				}

			},
		})

	ctxSignal, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	runCtx := ctxSignal
	if localPassword != "" {
		var keepAliveCancel context.CancelFunc
		runCtx, keepAliveCancel = context.WithCancel(ctxSignal)
		builder = builder.WithServerOptions(api.WithKeepAliveEndpoint(10*time.Second, func() {
			log.Warn("keep-alive endpoint idle for 10s, shutting down")
			keepAliveCancel()
		}))
	}

	service, err := builder.Build()
	if err != nil {
		log.Errorf("failed to build proxy service: %v", err)
		return
	}

	err = service.Run(runCtx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("proxy service exited with error: %v", err)
	}
}

// WaitForCloudDeploy waits indefinitely for shutdown signals in cloud deploy mode
// when no configuration file is available.
func WaitForCloudDeploy() {
	// Clarify that we are intentionally idle for configuration and not running the API server.
	log.Info("Cloud deploy mode: No config found; standing by for configuration. API server is not started. Press Ctrl+C to exit.")

	ctxSignal, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Block until shutdown signal is received
	<-ctxSignal.Done()
	log.Info("Cloud deploy mode: Shutdown signal received; exiting")
}
