package hooks

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Subscription is a handle for a registered subscriber.
type Subscription struct {
	ID          string
	Event       HookEvent
	Callback    func(*EventContext)
	Filter      func(*EventContext) bool
	Unsubscribe func()
}

// EventBus manages event distribution to subscribers.
type EventBus struct {
	subscribers  map[HookEvent][]*Subscription
	mu           sync.RWMutex
	eventQueue   chan *EventContext
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownOnce sync.Once
	shutdown     bool
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	bus := &EventBus{
		subscribers: make(map[HookEvent][]*Subscription),
		eventQueue:  make(chan *EventContext, 1000), // Buffer size 1000
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start async processor
	go bus.processQueue()

	return bus
}

// Subscribe registers a callback for a specific event type.
func (b *EventBus) Subscribe(event HookEvent, callback func(*EventContext)) *Subscription {
	return b.SubscribeWithFilter(event, callback, nil)
}

// SubscribeWithFilter registers a callback with an optional filter function.
func (b *EventBus) SubscribeWithFilter(event HookEvent, callback func(*EventContext), filter func(*EventContext) bool) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	sub := &Subscription{
		ID:       id,
		Event:    event,
		Callback: callback,
		Filter:   filter,
	}

	sub.Unsubscribe = func() {
		b.unsubscribe(sub)
	}

	b.subscribers[event] = append(b.subscribers[event], sub)
	return sub
}

func (b *EventBus) unsubscribe(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[sub.Event]
	for i, s := range subs {
		if s.ID == sub.ID {
			b.subscribers[sub.Event] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// Publish distributes an event to all subscribers synchronously.
func (b *EventBus) Publish(ctx *EventContext) {
	b.mu.RLock()
	subs := b.subscribers[ctx.Event]
	// Copy slice to avoid holding lock during execution
	activeSubs := make([]*Subscription, len(subs))
	copy(activeSubs, subs)
	b.mu.RUnlock()

	for _, sub := range activeSubs {
		if sub.Filter == nil || sub.Filter(ctx) {
			// Execute safely
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("Panic in event subscriber for %s: %v", ctx.Event, r)
					}
				}()
				sub.Callback(ctx)
			}()
		}
	}
}

// PublishAsync distributes an event asynchronously via the queue.
func (b *EventBus) PublishAsync(ctx *EventContext) {
	b.mu.RLock()
	isShutdown := b.shutdown
	b.mu.RUnlock()
	
	if isShutdown {
		return
	}
	
	select {
	case <-b.ctx.Done():
		// Bus is shutting down, ignore event
		return
	case b.eventQueue <- ctx:
		// Queued
	default:
		log.Warnf("Event queue full, dropping event: %s", ctx.Event)
	}
}

func (b *EventBus) processQueue() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case event, ok := <-b.eventQueue:
			if !ok {
				// Channel closed
				return
			}
			if event != nil {
				b.Publish(event)
			}
		}
	}
}

// Shutdown stops the event bus processing.
func (b *EventBus) Shutdown() {
	b.shutdownOnce.Do(func() {
		b.mu.Lock()
		b.shutdown = true
		b.mu.Unlock()
		
		b.cancel()
		close(b.eventQueue)
	})
}
