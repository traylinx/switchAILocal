// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package usage

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Record contains the usage statistics captured for a single provider request.
type Record struct {
	Provider    string
	Model       string
	APIKey      string
	AuthID      string
	AuthIndex   uint64
	Source      string
	RequestedAt time.Time
	Failed      bool
	Detail      Detail
}

// Detail holds the token usage breakdown.
type Detail struct {
	InputTokens     int64
	OutputTokens    int64
	ReasoningTokens int64
	CachedTokens    int64
	TotalTokens     int64
}

// Plugin consumes usage records emitted by the proxy runtime.
type Plugin interface {
	HandleUsage(ctx context.Context, record Record)
}

// SynchronousObserver receives usage on the request goroutine before the record
// enters the asynchronous plugin queue. It is intended for request-scoped quota
// accounting; implementations must return quickly and must not panic.
type SynchronousObserver interface {
	ObserveUsage(ctx context.Context, record Record)
}

// ModelAuthorizer rechecks the concrete model selected after request hooks and
// routing, immediately before executor dispatch.
type ModelAuthorizer interface {
	AuthorizeModel(model string) error
}

type synchronousObserverContextKey struct{}

// WithSynchronousObserver associates a request-scoped usage observer with ctx.
func WithSynchronousObserver(ctx context.Context, observer SynchronousObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, synchronousObserverContextKey{}, observer)
}

// PropagateSynchronousObserver copies the request-scoped observer from src to
// dst. API handlers use it when they derive executor contexts from a background
// parent while retaining the request's cancellation separately.
func PropagateSynchronousObserver(dst, src context.Context) context.Context {
	if dst == nil {
		dst = context.Background()
	}
	if src == nil {
		return dst
	}
	observer, _ := src.Value(synchronousObserverContextKey{}).(SynchronousObserver)
	return WithSynchronousObserver(dst, observer)
}

// AuthorizeModel asks the request-scoped policy observer to approve the final
// concrete model. Requests without a policy observer retain legacy behavior.
func AuthorizeModel(ctx context.Context, model string) error {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(synchronousObserverContextKey{}).(ModelAuthorizer)
	if observer == nil {
		return nil
	}
	return observer.AuthorizeModel(model)
}

type queueItem struct {
	ctx    context.Context
	record Record
}

// Manager maintains a queue of usage records and delivers them to registered plugins.
type Manager struct {
	once     sync.Once
	stopOnce sync.Once
	cancel   context.CancelFunc

	mu     sync.Mutex
	cond   *sync.Cond
	queue  []queueItem
	closed bool

	pluginsMu sync.RWMutex
	plugins   []Plugin
}

// NewManager constructs a manager with a buffered queue.
func NewManager(buffer int) *Manager {
	m := &Manager{}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Start launches the background dispatcher. Calling Start multiple times is safe.
func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	m.once.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		var workerCtx context.Context
		workerCtx, m.cancel = context.WithCancel(ctx)
		go m.run(workerCtx)
	})
}

// Stop stops the dispatcher and drains the queue.
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
		m.cond.Broadcast()
	})
}

// Register appends a plugin to the delivery list.
func (m *Manager) Register(plugin Plugin) {
	if m == nil || plugin == nil {
		return
	}
	m.pluginsMu.Lock()
	m.plugins = append(m.plugins, plugin)
	m.pluginsMu.Unlock()
}

// Publish enqueues a usage record for processing. If no plugin is registered
// the record will be discarded downstream.
func (m *Manager) Publish(ctx context.Context, record Record) {
	if m == nil {
		return
	}
	// ensure worker is running even if Start was not called explicitly
	m.Start(context.Background())
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.queue = append(m.queue, queueItem{ctx: ctx, record: record})
	m.mu.Unlock()
	m.cond.Signal()
}

func (m *Manager) run(ctx context.Context) {
	for {
		m.mu.Lock()
		for !m.closed && len(m.queue) == 0 {
			m.cond.Wait()
		}
		if len(m.queue) == 0 && m.closed {
			m.mu.Unlock()
			return
		}
		item := m.queue[0]
		m.queue = m.queue[1:]
		m.mu.Unlock()
		m.dispatch(item)
	}
}

func (m *Manager) dispatch(item queueItem) {
	m.pluginsMu.RLock()
	plugins := make([]Plugin, len(m.plugins))
	copy(plugins, m.plugins)
	m.pluginsMu.RUnlock()
	if len(plugins) == 0 {
		return
	}
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		safeInvoke(plugin, item.ctx, item.record)
	}
}

func safeInvoke(plugin Plugin, ctx context.Context, record Record) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("usage: plugin panic recovered: %v", r)
		}
	}()
	plugin.HandleUsage(ctx, record)
}

var defaultManager = NewManager(512)

// DefaultManager returns the global usage manager instance.
func DefaultManager() *Manager { return defaultManager }

// RegisterPlugin registers a plugin on the default manager.
func RegisterPlugin(plugin Plugin) { DefaultManager().Register(plugin) }

// PublishRecord publishes a record using the default manager. A request-scoped
// synchronous observer, when present, is invoked first so enforcement does not
// race the asynchronous plugin dispatcher.
func PublishRecord(ctx context.Context, record Record) {
	if ctx != nil {
		if observer, ok := ctx.Value(synchronousObserverContextKey{}).(SynchronousObserver); ok && observer != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("usage: synchronous observer panic recovered: %v", r)
					}
				}()
				observer.ObserveUsage(ctx, record)
			}()
		}
	}
	DefaultManager().Publish(ctx, record)
}

// StartDefault starts the default manager's dispatcher.
func StartDefault(ctx context.Context) { DefaultManager().Start(ctx) }

// StopDefault stops the default manager's dispatcher.
func StopDefault() { DefaultManager().Stop() }
