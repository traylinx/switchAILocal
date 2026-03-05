package hooks

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_Subscribe(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var called bool
	sub := bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
		called = true
	})

	if sub == nil {
		t.Fatal("Subscribe returned nil subscription")
	}

	if sub.ID == "" {
		t.Error("Subscription ID should not be empty")
	}

	if sub.Event != EventRequestReceived {
		t.Errorf("Expected event %s, got %s", EventRequestReceived, sub.Event)
	}

	// Test event publishing
	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	bus.Publish(ctx)

	if !called {
		t.Error("Callback should have been called")
	}
}

func TestEventBus_SubscribeWithFilter(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var calledCount int32

	// Subscribe with filter that only allows provider "test"
	sub := bus.SubscribeWithFilter(EventProviderUnavailable, func(ctx *EventContext) {
		atomic.AddInt32(&calledCount, 1)
	}, func(ctx *EventContext) bool {
		return ctx.Provider == "test"
	})

	if sub == nil {
		t.Fatal("SubscribeWithFilter returned nil subscription")
	}

	// Publish event that should be filtered out
	ctx1 := &EventContext{
		Event:     EventProviderUnavailable,
		Timestamp: time.Now(),
		Provider:  "other",
	}
	bus.Publish(ctx1)

	// Publish event that should pass filter
	ctx2 := &EventContext{
		Event:     EventProviderUnavailable,
		Timestamp: time.Now(),
		Provider:  "test",
	}
	bus.Publish(ctx2)

	if atomic.LoadInt32(&calledCount) != 1 {
		t.Errorf("Expected 1 callback call, got %d", calledCount)
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var called1, called2, called3 bool

	// Subscribe multiple callbacks to the same event
	sub1 := bus.Subscribe(EventRequestFailed, func(ctx *EventContext) {
		called1 = true
	})

	sub2 := bus.Subscribe(EventRequestFailed, func(ctx *EventContext) {
		called2 = true
	})

	sub3 := bus.Subscribe(EventRequestFailed, func(ctx *EventContext) {
		called3 = true
	})

	if sub1 == nil || sub2 == nil || sub3 == nil {
		t.Fatal("One or more subscriptions returned nil")
	}

	// Publish event
	ctx := &EventContext{
		Event:     EventRequestFailed,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"error": "test error"},
	}

	bus.Publish(ctx)

	if !called1 || !called2 || !called3 {
		t.Error("All callbacks should have been called")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var called bool
	sub := bus.Subscribe(EventQuotaWarning, func(ctx *EventContext) {
		called = true
	})

	// Unsubscribe
	sub.Unsubscribe()

	// Publish event
	ctx := &EventContext{
		Event:     EventQuotaWarning,
		Timestamp: time.Now(),
	}

	bus.Publish(ctx)

	if called {
		t.Error("Callback should not have been called after unsubscribe")
	}
}

func TestEventBus_PublishAsync(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var called bool
	var mu sync.Mutex

	sub := bus.Subscribe(EventModelDiscovered, func(ctx *EventContext) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	if sub == nil {
		t.Fatal("Subscribe returned nil subscription")
	}

	// Publish async
	ctx := &EventContext{
		Event:     EventModelDiscovered,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"model": "new-model"},
	}

	bus.PublishAsync(ctx)

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	wasCalled := called
	mu.Unlock()

	if !wasCalled {
		t.Error("Async callback should have been called")
	}
}

func TestEventBus_ErrorHandling(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var called bool

	// Subscribe callback that panics
	sub1 := bus.Subscribe(EventHealthCheckFailed, func(ctx *EventContext) {
		panic("test panic")
	})

	// Subscribe normal callback
	sub2 := bus.Subscribe(EventHealthCheckFailed, func(ctx *EventContext) {
		called = true
	})

	if sub1 == nil || sub2 == nil {
		t.Fatal("One or more subscriptions returned nil")
	}

	// Publish event
	ctx := &EventContext{
		Event:     EventHealthCheckFailed,
		Timestamp: time.Now(),
	}

	// Should not panic and should still call the second callback
	bus.Publish(ctx)

	if !called {
		t.Error("Normal callback should have been called despite panic in first callback")
	}
}

func TestEventBus_QueueOverflow(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	// Fill the queue beyond capacity
	for i := 0; i < 1500; i++ { // Queue capacity is 1000
		ctx := &EventContext{
			Event:     EventRoutingDecision,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"iteration": i},
		}
		bus.PublishAsync(ctx)
	}

	// Should not panic or block
	time.Sleep(10 * time.Millisecond)
}

func TestEventBus_Shutdown(t *testing.T) {
	bus := NewEventBus()

	var called bool
	sub := bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
		called = true
	})

	if sub == nil {
		t.Fatal("Subscribe returned nil subscription")
	}

	// Shutdown the bus
	bus.Shutdown()

	// Try to publish after shutdown
	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
	}

	// Should not panic
	bus.PublishAsync(ctx)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Callback should not have been called
	if called {
		t.Error("Callback should not have been called after shutdown")
	}
}

func TestEventBus_ConcurrentAccess(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var callCount int32
	var wg sync.WaitGroup

	// Subscribe multiple callbacks
	for i := 0; i < 10; i++ {
		bus.Subscribe(EventQuotaExceeded, func(ctx *EventContext) {
			atomic.AddInt32(&callCount, 1)
		})
	}

	// Publish events concurrently
	numGoroutines := 50
	eventsPerGoroutine := 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				ctx := &EventContext{
					Event:     EventQuotaExceeded,
					Timestamp: time.Now(),
				}
				bus.Publish(ctx)
			}
		}()
	}

	wg.Wait()

	expectedCalls := int32(numGoroutines * eventsPerGoroutine * 10) // 10 subscribers
	actualCalls := atomic.LoadInt32(&callCount)

	if actualCalls != expectedCalls {
		t.Errorf("Expected %d callback calls, got %d", expectedCalls, actualCalls)
	}
}

func TestEventBus_FilterFunction(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var filteredCalls, unfilteredCalls int32

	// Subscribe with filter
	sub1 := bus.SubscribeWithFilter(EventProviderUnavailable, func(ctx *EventContext) {
		atomic.AddInt32(&filteredCalls, 1)
	}, func(ctx *EventContext) bool {
		// Only allow events with error message containing "timeout"
		return ctx.ErrorMessage != "" && ctx.ErrorMessage == "timeout"
	})

	// Subscribe without filter
	sub2 := bus.Subscribe(EventProviderUnavailable, func(ctx *EventContext) {
		atomic.AddInt32(&unfilteredCalls, 1)
	})

	if sub1 == nil || sub2 == nil {
		t.Fatal("One or more subscriptions returned nil")
	}

	// Publish events
	events := []*EventContext{
		{Event: EventProviderUnavailable, Timestamp: time.Now(), ErrorMessage: "timeout"},
		{Event: EventProviderUnavailable, Timestamp: time.Now(), ErrorMessage: "connection refused"},
		{Event: EventProviderUnavailable, Timestamp: time.Now(), ErrorMessage: "timeout"},
		{Event: EventProviderUnavailable, Timestamp: time.Now(), ErrorMessage: "network error"},
	}

	for _, event := range events {
		bus.Publish(event)
	}

	if atomic.LoadInt32(&filteredCalls) != 2 {
		t.Errorf("Expected 2 filtered calls, got %d", filteredCalls)
	}

	if atomic.LoadInt32(&unfilteredCalls) != 4 {
		t.Errorf("Expected 4 unfiltered calls, got %d", unfilteredCalls)
	}
}

func TestEventBus_AsyncProcessingOrder(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	var processedEvents []int
	var mu sync.Mutex

	sub := bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
		if data, ok := ctx.Data["order"].(int); ok {
			mu.Lock()
			processedEvents = append(processedEvents, data)
			mu.Unlock()
		}
	})

	if sub == nil {
		t.Fatal("Subscribe returned nil subscription")
	}

	// Publish events async in order
	for i := 0; i < 10; i++ {
		ctx := &EventContext{
			Event:     EventRequestReceived,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"order": i},
		}
		bus.PublishAsync(ctx)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(processedEvents) != 10 {
		t.Errorf("Expected 10 processed events, got %d", len(processedEvents))
	}

	// Events should be processed in order (FIFO queue)
	for i, event := range processedEvents {
		if event != i {
			t.Errorf("Expected event %d at position %d, got %d", i, i, event)
		}
	}
}

// --- Benchmark Tests ---

func BenchmarkEventBus_Publish(b *testing.B) {
	bus := NewEventBus()
	defer bus.Shutdown()

	// Subscribe a simple callback
	bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
		// Do minimal work
		_ = ctx.Event
	})

	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(ctx)
	}
}

func BenchmarkEventBus_PublishAsync(b *testing.B) {
	bus := NewEventBus()
	defer bus.Shutdown()

	// Subscribe a simple callback
	bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
		// Do minimal work
		_ = ctx.Event
	})

	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishAsync(ctx)
	}
}

func BenchmarkEventBus_Subscribe(b *testing.B) {
	bus := NewEventBus()
	defer bus.Shutdown()

	callback := func(ctx *EventContext) {
		_ = ctx.Event
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub := bus.Subscribe(EventRequestReceived, callback)
		sub.Unsubscribe()
	}
}

func BenchmarkEventBus_MultipleSubscribers(b *testing.B) {
	bus := NewEventBus()
	defer bus.Shutdown()

	// Subscribe 100 callbacks
	for i := 0; i < 100; i++ {
		bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
			_ = ctx.Event
		})
	}

	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(ctx)
	}
}

func BenchmarkEventBus_WithFilter(b *testing.B) {
	bus := NewEventBus()
	defer bus.Shutdown()

	// Subscribe with filter
	bus.SubscribeWithFilter(EventProviderUnavailable, func(ctx *EventContext) {
		_ = ctx.Event
	}, func(ctx *EventContext) bool {
		return ctx.Provider == "test"
	})

	ctx := &EventContext{
		Event:     EventProviderUnavailable,
		Timestamp: time.Now(),
		Provider:  "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(ctx)
	}
}