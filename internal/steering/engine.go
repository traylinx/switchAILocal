package steering

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// SteeringEngine manages the lifecycle and matching of steering rules.
type SteeringEngine struct {
	steeringDir string
	rules       []*SteeringRule
	evaluator   *ConditionEvaluator
	injector    *ContextInjector
	mu          sync.RWMutex

	// watcher for hot-reloading
	watcher     *fsnotify.Watcher
	stopWatcher chan struct{}
}

// NewSteeringEngine creates a new steering engine.
func NewSteeringEngine(steeringDir string) (*SteeringEngine, error) {
	if steeringDir == "" {
		// Default to user home directory + .switchailocal/steering
		home, err := os.UserHomeDir()
		if err != nil {
			// Fallback to current directory if home directory is not accessible
			wd, _ := os.Getwd()
			steeringDir = filepath.Join(wd, ".switchailocal", "steering")
		} else {
			steeringDir = filepath.Join(home, ".switchailocal", "steering")
		}
	}

	engine := &SteeringEngine{
		steeringDir: steeringDir,
		rules:       make([]*SteeringRule, 0),
		evaluator:   NewConditionEvaluator(),
		injector:    NewContextInjector(),
		stopWatcher: make(chan struct{}),
	}

	return engine, nil
}

// LoadRules loads all steering rules from the steering directory.
func (e *SteeringEngine) LoadRules() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, err := os.Stat(e.steeringDir); os.IsNotExist(err) {
		if err := os.MkdirAll(e.steeringDir, 0755); err != nil {
			return fmt.Errorf("failed to create steering directory: %w", err)
		}
		// Create subdirectories for organization
		_ = os.MkdirAll(filepath.Join(e.steeringDir, "intents"), 0755)
		_ = os.MkdirAll(filepath.Join(e.steeringDir, "providers"), 0755)
		_ = os.MkdirAll(filepath.Join(e.steeringDir, "users"), 0755)
	}

	newRules := make([]*SteeringRule, 0)

	// Get absolute path of steering directory for security checks
	absSteeringDir, err := filepath.Abs(e.steeringDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of steering directory: %w", err)
	}

	err = filepath.Walk(e.steeringDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Security: Skip symlinks to prevent directory traversal attacks
		if info.Mode()&os.ModeSymlink != 0 {
			log.Warnf("Skipping symlink in steering directory: %s", path)
			return nil
		}

		// Security: Ensure path is within steering directory
		absPath, err := filepath.Abs(path)
		if err != nil {
			log.Warnf("Failed to get absolute path for %s: %v", path, err)
			return nil
		}
		if !strings.HasPrefix(absPath, absSteeringDir) {
			log.Warnf("Skipping file outside steering directory: %s", path)
			return nil
		}

		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			// Security: Check file size to prevent YAML bombs
			if info.Size() > 1*1024*1024 { // 1MB limit
				log.Warnf("Skipping large steering file: %s (%d bytes)", path, info.Size())
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				log.Errorf("Failed to read steering file %s: %v", path, err)
				return nil
			}

			var rule SteeringRule
			if err := yaml.Unmarshal(data, &rule); err != nil {
				log.Errorf("Failed to parse steering rule %s: %v", path, err)
				return nil
			}

			rule.FilePath = path
			newRules = append(newRules, &rule)
			log.Debugf("Loaded steering rule: %s from %s", rule.Name, path)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Sort rules by priority (highest first)
	sort.Slice(newRules, func(i, j int) bool {
		return newRules[i].Activation.Priority > newRules[j].Activation.Priority
	})

	e.rules = newRules
	log.Infof("Successfully loaded %d steering rules", len(e.rules))
	return nil
}

// FindMatchingRules finds all rules that activate for the given context.
func (e *SteeringEngine) FindMatchingRules(ctx *RoutingContext) ([]*SteeringRule, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	matches := make([]*SteeringRule, 0)
	for _, rule := range e.rules {
		active, err := e.evaluator.Evaluate(rule.Activation.Condition, ctx)
		if err != nil {
			log.Warnf("Failed to evaluate condition for rule %s: %v", rule.Name, err)
			continue
		}

		if active {
			// Check time-based rules within preferences
			if len(rule.Preferences.TimeBasedRules) > 0 {
				for _, tr := range rule.Preferences.TimeBasedRules {
					if e.evaluator.CheckTimeRule(tr, ctx.Timestamp) {
						// Apply time-based override to primary model if specified
						if tr.PreferModel != "" {
							log.Debugf("Time-based rule matches: preference for model %s", tr.PreferModel)
						}
						break
					}
				}
			}
			// Return a COPY of the rule to prevent race conditions
			// The caller can safely use this copy even after the lock is released
			ruleCopy := *rule
			matches = append(matches, &ruleCopy)
		}
	}

	return matches, nil
}

// StartWatcher starts a background fsnotify watcher for hot-reloading rules.
func (e *SteeringEngine) StartWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	e.watcher = watcher

	// Add steering directory and subdirectories
	err = filepath.Walk(e.steeringDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
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
					log.Infof("Steering directory changed (%s), reloading rules...", event.Name)
					// Simple debounce can be added here
					time.Sleep(100 * time.Millisecond)
					if err := e.LoadRules(); err != nil {
						log.Errorf("Failed to reload steering rules: %v", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Errorf("Steering watcher error: %v", err)
			case <-e.stopWatcher:
				return
			}
		}
	}()

	return nil
}

// StopWatcher stops the file watcher.
func (e *SteeringEngine) StopWatcher() {
	if e.watcher != nil {
		select {
		case <-e.stopWatcher:
			// Channel already closed
		default:
			close(e.stopWatcher)
		}
		e.watcher.Close()
		e.watcher = nil
	}
}

// GetRules returns the currently loaded rules.
func (e *SteeringEngine) GetRules() []*SteeringRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy
	res := make([]*SteeringRule, len(e.rules))
	copy(res, e.rules)
	return res
}

// ApplySteering applies the matched rules to a request context.
// It modifies the primary model, fallbacks, and injects context as specified by the rules.
func (e *SteeringEngine) ApplySteering(ctx *RoutingContext, messages []map[string]string, metadata map[string]interface{}, rules []*SteeringRule) (string, []map[string]string, map[string]interface{}) {
	if len(rules) == 0 {
		return "", messages, metadata
	}

	selectedModel := ""
	newMessages := messages
	newMetadata := metadata

	// Apply rules in priority order (they are already sorted)
	for _, rule := range rules {
		prefs := rule.Preferences

		// 1. Time-based overrides (highest priority within a rule)
		if len(prefs.TimeBasedRules) > 0 {
			for _, tr := range prefs.TimeBasedRules {
				if e.evaluator.CheckTimeRule(tr, ctx.Timestamp) {
					if tr.PreferModel != "" {
						selectedModel = tr.PreferModel
						break
					}
				}
			}
		}

		// 2. Primary model override (if not already set by time-based rule)
		if selectedModel == "" && prefs.PrimaryModel != "" {
			selectedModel = prefs.PrimaryModel
		}

		// 3. Context Injection
		if prefs.ContextInjection != "" {
			formattedPrompt := e.injector.FormatContextInjection(prefs.ContextInjection, ctx)
			newMessages = e.injector.InjectSystemPrompt(newMessages, formattedPrompt)
		}

		// 4. Provider Settings
		if len(prefs.ProviderSettings) > 0 {
			newMetadata = e.injector.ApplyProviderSettings(newMetadata, prefs.ProviderSettings)
		}

		// Stop after first high-priority rule if OverrideRouter is true
		if prefs.OverrideRouter {
			break
		}
	}

	return selectedModel, newMessages, newMetadata
}
