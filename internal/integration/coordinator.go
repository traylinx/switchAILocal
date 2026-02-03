// Package integration provides coordination and lifecycle management for intelligent systems.
// It integrates Memory, Heartbeat, Steering, and Hooks into the main server lifecycle.
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/heartbeat"
	"github.com/traylinx/switchAILocal/internal/hooks"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/steering"

	log "github.com/sirupsen/logrus"
)

// ServiceCoordinator manages the lifecycle of all four intelligent systems.
// It handles initialization, startup, shutdown, and provides access to system instances.
type ServiceCoordinator struct {
	// Configuration
	config *IntegrationConfig

	// System instances
	memoryManager memory.MemoryManager
	heartbeat     heartbeat.HeartbeatMonitor
	steering      *steering.SteeringEngine
	hooks         *hooks.HookManager
	eventBus      *hooks.EventBus

	// Lifecycle management
	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// IntegrationConfig holds configuration for all intelligent systems.
type IntegrationConfig struct {
	Memory    *memory.MemoryConfig
	Heartbeat *heartbeat.HeartbeatConfig
	Steering  *config.SteeringConfig
	Hooks     *config.HooksConfig
	
	// MainConfig is the full application configuration, used for provider registration
	MainConfig *config.Config
}

// NewServiceCoordinator creates a new service coordinator with the given configuration.
// It initializes all systems but does not start them. Call Start() to begin operations.
func NewServiceCoordinator(cfg *IntegrationConfig) (*ServiceCoordinator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	// Apply defaults if needed
	if cfg.Memory == nil {
		cfg.Memory = memory.DefaultMemoryConfig()
	}
	if cfg.Heartbeat == nil {
		cfg.Heartbeat = heartbeat.DefaultHeartbeatConfig()
	}
	if cfg.Steering == nil {
		cfg.Steering = &config.SteeringConfig{
			Enabled:   false,
			RulesDir:  ".switchailocal/steering",
			HotReload: true,
		}
	}
	if cfg.Hooks == nil {
		cfg.Hooks = &config.HooksConfig{
			Enabled:   false,
			HooksDir:  ".switchailocal/hooks",
			HotReload: true,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	coordinator := &ServiceCoordinator{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize systems in order
	if err := coordinator.initializeSystems(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize systems: %w", err)
	}

	return coordinator, nil
}

// initializeSystems initializes all four intelligent systems.
// Systems are initialized in dependency order:
// 1. Event Bus (no dependencies)
// 2. Memory System (no dependencies)
// 3. Steering Engine (no dependencies)
// 4. Hooks Manager (depends on Event Bus)
// 5. Heartbeat Monitor (no dependencies, but will emit events to Event Bus)
func (sc *ServiceCoordinator) initializeSystems() error {
	var errors []error

	// 1. Initialize Event Bus (always initialized, even if hooks are disabled)
	log.Debug("Initializing Event Bus...")
	sc.eventBus = hooks.NewEventBus()
	log.Info("Event Bus initialized successfully")

	// 2. Initialize Memory System (if enabled)
	if sc.config.Memory.Enabled {
		log.Debug("Initializing Memory System...")
		memMgr, err := memory.NewMemoryManager(sc.config.Memory)
		if err != nil {
			log.Errorf("Failed to initialize Memory System: %v", err)
			errors = append(errors, fmt.Errorf("memory system initialization failed: %w", err))
		} else {
			sc.memoryManager = memMgr
			log.Info("Memory System initialized successfully")
		}
	} else {
		log.Debug("Memory System is disabled, skipping initialization")
	}

	// 3. Initialize Steering Engine (always initialized to load rules)
	log.Debug("Initializing Steering Engine...")
	steeringDir := sc.config.Steering.RulesDir
	if steeringDir == "" {
		steeringDir = sc.config.Steering.SteeringDir
	}
	if steeringDir == "" {
		// Use default directory
		home, err := os.UserHomeDir()
		if err != nil {
			wd, _ := os.Getwd()
			steeringDir = filepath.Join(wd, ".switchailocal", "steering")
		} else {
			steeringDir = filepath.Join(home, ".switchailocal", "steering")
		}
	}

	steeringEngine, err := steering.NewSteeringEngine(steeringDir)
	if err != nil {
		log.Errorf("Failed to initialize Steering Engine: %v", err)
		errors = append(errors, fmt.Errorf("steering engine initialization failed: %w", err))
	} else {
		sc.steering = steeringEngine
		// Load rules immediately
		if err := sc.steering.LoadRules(); err != nil {
			log.Warnf("Failed to load steering rules: %v", err)
			// Don't fail initialization if rules can't be loaded
		}
		log.Info("Steering Engine initialized successfully")
	}

	// 4. Initialize Hooks Manager (always initialized to subscribe to events)
	log.Debug("Initializing Hooks Manager...")
	hooksDir := sc.config.Hooks.HooksDir
	if hooksDir == "" {
		// Use default directory
		home, err := os.UserHomeDir()
		if err != nil {
			wd, _ := os.Getwd()
			hooksDir = filepath.Join(wd, ".switchailocal", "hooks")
		} else {
			hooksDir = filepath.Join(home, ".switchailocal", "hooks")
		}
	}

	hooksMgr, err := hooks.NewHookManager(hooksDir, sc.eventBus)
	if err != nil {
		log.Errorf("Failed to initialize Hooks Manager: %v", err)
		errors = append(errors, fmt.Errorf("hooks manager initialization failed: %w", err))
	} else {
		sc.hooks = hooksMgr
		// Load hooks immediately
		if err := sc.hooks.LoadHooks(); err != nil {
			log.Warnf("Failed to load hooks: %v", err)
			// Don't fail initialization if hooks can't be loaded
		}
		// Subscribe to all events
		sc.hooks.SubscribeToAllEvents()
		log.Info("Hooks Manager initialized successfully")
	}

	// 5. Initialize Heartbeat Monitor (if enabled)
	if sc.config.Heartbeat.Enabled {
		log.Debug("Initializing Heartbeat Monitor...")
		hbMonitor := heartbeat.NewHeartbeatMonitor(sc.config.Heartbeat)
		sc.heartbeat = hbMonitor
		
		// Register provider checkers from configuration
		if sc.config.MainConfig != nil {
			log.Debug("Registering provider health checkers...")
			if err := RegisterProviderCheckers(hbMonitor, sc.config.MainConfig); err != nil {
				log.Warnf("Failed to register provider checkers: %v", err)
				// Don't fail initialization if checker registration fails
			}
		}
		
		log.Info("Heartbeat Monitor initialized successfully")
	} else {
		log.Debug("Heartbeat Monitor is disabled, skipping initialization")
	}

	// Return error if any critical system failed to initialize
	// We consider all systems non-critical for now (fail gracefully)
	if len(errors) > 0 {
		log.Warnf("Some systems failed to initialize: %v", errors)
		// Don't return error - allow server to start with partial functionality
	}

	return nil
}

// Start begins operations for all enabled systems.
// It starts background services and file watchers.
// This method is idempotent - calling it multiple times has no effect.
func (sc *ServiceCoordinator) Start(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.started {
		return fmt.Errorf("service coordinator already started")
	}

	log.Info("Starting Service Coordinator...")

	var errors []error

	// Start Heartbeat Monitor (if enabled and initialized)
	if sc.heartbeat != nil && sc.config.Heartbeat.Enabled {
		log.Debug("Starting Heartbeat Monitor...")
		if err := sc.heartbeat.Start(ctx); err != nil {
			log.Errorf("Failed to start Heartbeat Monitor: %v", err)
			errors = append(errors, fmt.Errorf("heartbeat monitor start failed: %w", err))
		} else {
			log.Info("Heartbeat Monitor started successfully")
		}
	}

	// Start Steering Engine file watcher (if hot-reload is enabled)
	if sc.steering != nil && sc.config.Steering.HotReload {
		log.Debug("Starting Steering Engine file watcher...")
		if err := sc.steering.StartWatcher(); err != nil {
			log.Warnf("Failed to start Steering Engine file watcher: %v", err)
			// Don't fail startup if watcher can't start
		} else {
			log.Info("Steering Engine file watcher started successfully")
		}
	}

	// Start Hooks Manager file watcher (if hot-reload is enabled)
	if sc.hooks != nil && sc.config.Hooks.HotReload {
		log.Debug("Starting Hooks Manager file watcher...")
		if err := sc.hooks.StartWatcher(); err != nil {
			log.Warnf("Failed to start Hooks Manager file watcher: %v", err)
			// Don't fail startup if watcher can't start
		} else {
			log.Info("Hooks Manager file watcher started successfully")
		}
	}

	// Connect event sources to event bus
	log.Debug("Connecting event sources to event bus...")
	eventIntegrator := NewEventBusIntegrator(sc.eventBus, sc.hooks, sc.heartbeat)
	
	// Connect heartbeat monitor events
	if err := eventIntegrator.ConnectHeartbeatEvents(); err != nil {
		log.Warnf("Failed to connect heartbeat events: %v", err)
		// Don't fail startup if event connection fails
	}
	
	// Connect routing decision events (no-op, but call for completeness)
	if err := eventIntegrator.ConnectRoutingEvents(); err != nil {
		log.Warnf("Failed to connect routing events: %v", err)
	}
	
	// Connect provider failure events (no-op, but call for completeness)
	if err := eventIntegrator.ConnectProviderEvents(); err != nil {
		log.Warnf("Failed to connect provider events: %v", err)
	}
	
	log.Info("Event sources connected to event bus successfully")

	sc.started = true
	log.Info("Service Coordinator started successfully")

	// Return error only if critical systems failed
	// For now, we treat all systems as non-critical
	if len(errors) > 0 {
		log.Warnf("Some systems failed to start: %v", errors)
		// Don't return error - allow server to continue with partial functionality
	}

	return nil
}

// Stop gracefully shuts down all systems.
// It stops background services, closes file watchers, and releases resources.
// Systems are stopped in reverse initialization order.
// This method is idempotent - calling it multiple times has no effect.
func (sc *ServiceCoordinator) Stop(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.started {
		return nil // Already stopped or never started
	}

	log.Info("Stopping Service Coordinator...")

	// Cancel context to signal shutdown
	if sc.cancel != nil {
		sc.cancel()
	}

	var errors []error

	// Create a timeout context for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	// Stop systems in reverse order
	// 1. Stop Heartbeat Monitor
	if sc.heartbeat != nil {
		log.Debug("Stopping Heartbeat Monitor...")
		if err := sc.heartbeat.Stop(); err != nil {
			log.Errorf("Failed to stop Heartbeat Monitor: %v", err)
			errors = append(errors, fmt.Errorf("heartbeat monitor stop failed: %w", err))
		} else {
			log.Info("Heartbeat Monitor stopped successfully")
		}
	}

	// 2. Stop Hooks Manager file watcher
	if sc.hooks != nil {
		log.Debug("Stopping Hooks Manager file watcher...")
		sc.hooks.StopWatcher()
		log.Info("Hooks Manager file watcher stopped successfully")
	}

	// 3. Stop Steering Engine file watcher
	if sc.steering != nil {
		log.Debug("Stopping Steering Engine file watcher...")
		sc.steering.StopWatcher()
		log.Info("Steering Engine file watcher stopped successfully")
	}

	// 4. Wait for in-flight operations to complete (with timeout)
	// This is a simple wait - in a production system, you'd track active operations
	select {
	case <-shutdownCtx.Done():
		log.Warn("Shutdown timeout reached, forcing shutdown")
	case <-time.After(100 * time.Millisecond):
		// Brief pause to allow in-flight operations to complete
	}

	// 5. Close Memory System
	if sc.memoryManager != nil {
		log.Debug("Closing Memory System...")
		if err := sc.memoryManager.Close(); err != nil {
			log.Errorf("Failed to close Memory System: %v", err)
			errors = append(errors, fmt.Errorf("memory system close failed: %w", err))
		} else {
			log.Info("Memory System closed successfully")
		}
	}

	// 6. Shutdown Event Bus
	if sc.eventBus != nil {
		log.Debug("Shutting down Event Bus...")
		sc.eventBus.Shutdown()
		log.Info("Event Bus shut down successfully")
	}

	sc.started = false
	log.Info("Service Coordinator stopped successfully")

	// Return combined errors if any
	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// GetMemory returns the Memory Manager instance.
// Returns nil if the memory system is disabled or failed to initialize.
func (sc *ServiceCoordinator) GetMemory() memory.MemoryManager {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.memoryManager
}

// GetHeartbeat returns the Heartbeat Monitor instance.
// Returns nil if the heartbeat monitor is disabled or failed to initialize.
func (sc *ServiceCoordinator) GetHeartbeat() heartbeat.HeartbeatMonitor {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.heartbeat
}

// GetSteering returns the Steering Engine instance.
// Returns nil if the steering engine failed to initialize.
func (sc *ServiceCoordinator) GetSteering() *steering.SteeringEngine {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.steering
}

// GetHooks returns the Hooks Manager instance.
// Returns nil if the hooks manager failed to initialize.
func (sc *ServiceCoordinator) GetHooks() *hooks.HookManager {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.hooks
}

// GetEventBus returns the Event Bus instance.
// The event bus is always initialized, even if hooks are disabled.
func (sc *ServiceCoordinator) GetEventBus() *hooks.EventBus {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.eventBus
}

// IsStarted returns true if the coordinator has been started.
func (sc *ServiceCoordinator) IsStarted() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.started
}
