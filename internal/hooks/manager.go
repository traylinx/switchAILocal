package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// HookManager manages the lifecycle and execution of automation hooks.
type HookManager struct {
	hooksDir       string
	hooks          map[HookEvent][]*Hook
	eventBus       *EventBus
	programs       map[string]*vm.Program
	actionHandlers map[HookAction]ActionHandler
	mu             sync.RWMutex

	watcher     *fsnotify.Watcher
	stopWatcher chan struct{}
}

// NewHookManager creates a new hook manager.
func NewHookManager(hooksDir string, eventBus *EventBus) (*HookManager, error) {
	if hooksDir == "" {
		// Default to user home directory + .switchailocal/hooks
		home, err := os.UserHomeDir()
		if err != nil {
			// Fallback to current directory if home directory is not accessible
			wd, _ := os.Getwd()
			hooksDir = filepath.Join(wd, ".switchailocal", "hooks")
		} else {
			hooksDir = filepath.Join(home, ".switchailocal", "hooks")
		}
	}

	manager := &HookManager{
		hooksDir:       hooksDir,
		hooks:          make(map[HookEvent][]*Hook),
		eventBus:       eventBus,
		programs:       make(map[string]*vm.Program),
		actionHandlers: make(map[HookAction]ActionHandler),
		stopWatcher:    make(chan struct{}),
	}

	// Register default action handlers
	RegisterBuiltInActions(manager)

	return manager, nil
}

// LoadHooks loads all hooks from the hooks directory.
func (m *HookManager) LoadHooks() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.hooksDir); os.IsNotExist(err) {
		if err := os.MkdirAll(m.hooksDir, 0755); err != nil {
			return fmt.Errorf("failed to create hooks directory: %w", err)
		}
	}

	newHooks := make(map[HookEvent][]*Hook)
	err := filepath.Walk(m.hooksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			data, err := os.ReadFile(path)
			if err != nil {
				log.Errorf("Failed to read hook file %s: %v", path, err)
				return nil
			}

			var hook Hook
			if err := yaml.Unmarshal(data, &hook); err != nil {
				log.Errorf("Failed to parse hook %s: %v", path, err)
				return nil
			}

			hook.FilePath = path
			if hook.Enabled {
				newHooks[hook.Event] = append(newHooks[hook.Event], &hook)
				log.Debugf("Loaded hook: %s for event %s", hook.Name, hook.Event)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	m.hooks = newHooks
	m.programs = make(map[string]*vm.Program) // Clear cache

	// Subscribe to all handled events
	// We do this dynamically or just subscribe to all known types in SubscribeToAllEvents

	log.Infof("Successfully loaded hooks for %d event types", len(m.hooks))
	return nil
}

// subscribeToEvents ensures the manager is subscribed to all relevant events.
// This is a bit tricky with reloading.
// Alternative: Subscribe to ALL known events unconditionally at startup.
func (m *HookManager) SubscribeToAllEvents() {
	events := []HookEvent{
		EventRequestReceived, EventRequestFailed, EventProviderUnavailable,
		EventQuotaWarning, EventQuotaExceeded, EventModelDiscovered,
		EventHealthCheckFailed, EventRoutingDecision,
	}

	for _, evt := range events {
		m.eventBus.Subscribe(evt, m.handleEvent)
	}
}

func (m *HookManager) handleEvent(ctx *EventContext) {
	m.mu.RLock()
	hooks, exists := m.hooks[ctx.Event]
	m.mu.RUnlock()

	if !exists || len(hooks) == 0 {
		return
	}

	for _, hook := range hooks {
		matches, err := m.evaluateCondition(hook.Condition, ctx)
		if err != nil {
			log.Warnf("Failed to evaluate hook condition '%s': %v", hook.Condition, err)
			continue
		}

		if matches {
			log.Infof("Executing hook: %s (Action: %s)", hook.Name, hook.Action)
			go m.executeAction(hook, ctx)
		}
	}
}

func (m *HookManager) evaluateCondition(condition string, ctx *EventContext) (bool, error) {
	if condition == "" || condition == "true" {
		return true, nil
	}

	m.mu.Lock()
	program, exists := m.programs[condition]
	if !exists {
		var err error
		// Compile with generic map environment to avoid context-specific compilation
		program, err = expr.Compile(condition)
		if err != nil {
			m.mu.Unlock()
			return false, err
		}
		m.programs[condition] = program
	}
	m.mu.Unlock()

	// Convert EventContext to map for safe evaluation
	ctxMap := map[string]interface{}{
		"Event":     string(ctx.Event),
		"Timestamp": ctx.Timestamp,
		"Data":      ctx.Data,
		"Provider":  ctx.Provider,
	}

	// Add Request fields if present
	if ctx.Request != nil {
		ctxMap["Request"] = ctx.Request
	}

	// Add Error if present
	if ctx.Error != nil {
		ctxMap["Error"] = ctx.Error.Error()
	}

	output, err := expr.Run(program, ctxMap)
	if err != nil {
		return false, err
	}

	result, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("condition did not return boolean")
	}

	return result, nil
}

func (m *HookManager) executeAction(hook *Hook, ctx *EventContext) {
	m.mu.RLock()
	handler, exists := m.actionHandlers[hook.Action]
	m.mu.RUnlock()

	if !exists {
		log.Warnf("No handler registered for action: %s", hook.Action)
		return
	}

	if err := handler(hook, ctx); err != nil {
		log.Errorf("Action %s failed for hook %s: %v", hook.Action, hook.Name, err)
	}
}

// RegisterAction registers a handler for a specific action type.
func (m *HookManager) RegisterAction(action HookAction, handler ActionHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionHandlers[action] = handler
}

// StartWatcher starts a background fsnotify watcher for hot-reloading hooks.
func (m *HookManager) StartWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	m.watcher = watcher

	err = m.watcher.Add(m.hooksDir)
	if err != nil {
		watcher.Close()
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
					log.Infof("Hooks directory changed (%s), reloading...", event.Name)
					time.Sleep(100 * time.Millisecond)
					if err := m.LoadHooks(); err != nil {
						log.Errorf("Failed to reload hooks: %v", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Errorf("Hooks watcher error: %v", err)
			case <-m.stopWatcher:
				return
			}
		}
	}()

	return nil
}

// StopWatcher stops the file watcher.
func (m *HookManager) StopWatcher() {
	if m.watcher != nil {
		select {
		case <-m.stopWatcher:
		default:
			close(m.stopWatcher)
		}
		m.watcher.Close()
	}
}

// GetHooksDir returns the hooks directory path.
func (m *HookManager) GetHooksDir() string {
	return m.hooksDir
}

// GetHooks returns all loaded hooks flattened.
func (m *HookManager) GetHooks() []*Hook {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Hook, 0)
	for _, hooks := range m.hooks {
		result = append(result, hooks...)
	}
	return result
}

// GetHook returns a hook by ID.
func (m *HookManager) GetHook(id string) *Hook {
	all := m.GetHooks()
	for _, h := range all {
		if h.ID == id {
			return h
		}
	}
	return nil
}

// EvaluateCondition exposes condition evaluation for testing.
func (m *HookManager) EvaluateCondition(h *Hook, ctx *EventContext) (bool, error) {
	return m.evaluateCondition(h.Condition, ctx)
}
