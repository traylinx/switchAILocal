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
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/integration"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/runtime/executor"
	"github.com/traylinx/switchAILocal/internal/steering"
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
	// Convert config types for integration
	memoryConfig := convertMemoryConfig(&cfg.Memory)
	heartbeatConfig := convertHeartbeatConfig(&cfg.Heartbeat)

	// Load integration configuration from main config
	integrationCfg := &integration.IntegrationConfig{
		Memory:     memoryConfig,
		Heartbeat:  heartbeatConfig,
		Steering:   &cfg.Steering,
		Hooks:      &cfg.Hooks,
		MainConfig: cfg,
	}

	// Create ServiceCoordinator with integration config
	coordinator, err := integration.NewServiceCoordinator(integrationCfg)
	if err != nil {
		log.Errorf("failed to create service coordinator: %v", err)
		// Continue without intelligent systems - fail gracefully
		coordinator = nil
	}

	// Start ServiceCoordinator
	if coordinator != nil {
		startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := coordinator.Start(startCtx); err != nil {
			log.Errorf("failed to start service coordinator: %v", err)
			// Continue without intelligent systems - fail gracefully
			coordinator = nil
		}
		startCancel()
	}

	// Create pipeline integrator if coordinator is available
	var pipelineIntegrator *integration.RequestPipelineIntegrator
	if coordinator != nil {
		pipelineIntegrator = integration.NewRequestPipelineIntegrator(
			coordinator.GetSteering().(*steering.SteeringEngine),
			coordinator.GetMemory().(memory.MemoryManager),
			coordinator.GetEventBus().(*hooks.EventBus),
		)
	}

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

	// Pass coordinator to API server via ServerOption
	if coordinator != nil {
		builder = builder.WithServerOptions(
			api.WithServiceCoordinator(coordinator),
			api.WithPipelineIntegrator(pipelineIntegrator),
		)
	}

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
		// Stop coordinator on build failure
		if coordinator != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if stopErr := coordinator.Stop(shutdownCtx); stopErr != nil {
				log.Errorf("failed to stop service coordinator: %v", stopErr)
			}
			shutdownCancel()
		}
		return
	}

	fmt.Println("INTELLIGENCE_DIAGNOSTIC: About to call service.Run() in StartService")
	err = service.Run(runCtx)
	fmt.Println("INTELLIGENCE_DIAGNOSTIC: service.Run() returned")
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("proxy service exited with error: %v", err)
	}

	// Stop coordinator on shutdown
	if coordinator != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if stopErr := coordinator.Stop(shutdownCtx); stopErr != nil {
			log.Errorf("failed to stop service coordinator: %v", stopErr)
		}
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

// convertMemoryConfig converts config.MemoryConfig to memory.MemoryConfig
func convertMemoryConfig(cfg *config.MemoryConfig) *memory.MemoryConfig {
	return &memory.MemoryConfig{
		Enabled:       cfg.Enabled,
		BaseDir:       cfg.BaseDir,
		RetentionDays: cfg.RetentionDays,
		MaxLogSizeMB:  cfg.MaxLogSizeMB,
		Compression:   cfg.Compression,
	}
}

// convertHeartbeatConfig converts config.HeartbeatConfig to heartbeat.HeartbeatConfig
func convertHeartbeatConfig(cfg *config.HeartbeatConfig) *heartbeat.HeartbeatConfig {
	// Parse duration strings
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		interval = 5 * time.Minute // Default
	}

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = 5 * time.Second // Default
	}

	retryDelay, err := time.ParseDuration(cfg.RetryDelay)
	if err != nil {
		retryDelay = time.Second // Default
	}

	return &heartbeat.HeartbeatConfig{
		Enabled:                cfg.Enabled,
		Interval:               interval,
		Timeout:                timeout,
		AutoDiscovery:          cfg.AutoDiscovery,
		QuotaWarningThreshold:  cfg.QuotaWarningThreshold,
		QuotaCriticalThreshold: cfg.QuotaCriticalThreshold,
		MaxConcurrentChecks:    cfg.MaxConcurrentChecks,
		RetryAttempts:          cfg.RetryAttempts,
		RetryDelay:             retryDelay,
	}
}
