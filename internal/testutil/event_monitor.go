package testutil

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/pkg/events"
)

// EventCounter tracks the count and last event for a specific event type
type EventCounter struct {
	Count     int
	LastEvent events.Event
	mu        sync.RWMutex
}

// Get returns the current count and last event (thread-safe)
func (ec *EventCounter) Get() (int, events.Event) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.Count, ec.LastEvent
}

// EventMonitor provides utilities for monitoring events during tests
// (corresponds to helpers.ts EventMonitor class)
type EventMonitor struct {
	eventBus *events.EventBus
	ctx      context.Context
	cancel   context.CancelFunc
	counters map[events.EventType]*EventCounter
	waiters  map[events.EventType][]chan events.Event
	mu       sync.RWMutex
}

// NewEventMonitor creates a new event monitor for testing
func NewEventMonitor(t testing.TB) *EventMonitor {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	eventBus := events.NewEventBus(ctx, 1000)

	monitor := &EventMonitor{
		eventBus: eventBus,
		ctx:      ctx,
		cancel:   cancel,
		counters: make(map[events.EventType]*EventCounter),
		waiters:  make(map[events.EventType][]chan events.Event),
	}

	// Cleanup when test ends
	t.Cleanup(func() {
		monitor.Release()
	})

	return monitor
}

// GetEventBus returns the event bus for integration with other components
func (em *EventMonitor) GetEventBus() *events.EventBus {
	return em.eventBus
}

// Count sets up an event counter for the specified event type
// (corresponds to helpers.ts EventMonitor.count method)
func (em *EventMonitor) Count(eventType events.EventType) *EventCounter {
	em.mu.Lock()
	defer em.mu.Unlock()

	if counter, exists := em.counters[eventType]; exists {
		return counter
	}

	counter := &EventCounter{}
	em.counters[eventType] = counter

	// Subscribe to the event
	em.eventBus.Subscribe(eventType, func(ctx context.Context, event events.Event) {
		counter.mu.Lock()
		counter.Count++
		counter.LastEvent = event
		counter.mu.Unlock()
	})

	return counter
}

// AwaitNext waits for the next occurrence of the specified event
// (corresponds to helpers.ts EventMonitor.awaitNext method)
func (em *EventMonitor) AwaitNext(eventType events.EventType) <-chan events.Event {
	em.mu.Lock()
	defer em.mu.Unlock()

	ch := make(chan events.Event, 1)

	// Add to waiters list
	if em.waiters[eventType] == nil {
		em.waiters[eventType] = make([]chan events.Event, 0)

		// Subscribe to the event if this is the first waiter
		em.eventBus.Subscribe(eventType, func(ctx context.Context, event events.Event) {
			em.mu.Lock()
			waiters := em.waiters[eventType]
			// Send to all current waiters and clear the list
			for _, waiter := range waiters {
				select {
				case waiter <- event:
				default:
					// Channel full, skip
				}
			}
			em.waiters[eventType] = nil
			em.mu.Unlock()
		})
	}

	em.waiters[eventType] = append(em.waiters[eventType], ch)
	return ch
}

// AwaitNextTimeout waits for the next occurrence of the specified event with timeout
func (em *EventMonitor) AwaitNextTimeout(t testing.TB, eventType events.EventType, timeout time.Duration) events.Event {
	t.Helper()

	eventCh := em.AwaitNext(eventType)

	select {
	case event := <-eventCh:
		return event
	case <-time.After(timeout):
		t.Fatalf("Timeout waiting for event %s after %v", eventType, timeout)
		return events.Event{} // Never reached
	}
}

// WaitForEventCount waits until the event count reaches the expected value
func (em *EventMonitor) WaitForEventCount(t testing.TB, eventType events.EventType, expectedCount int, timeout time.Duration) {
	t.Helper()

	counter := em.Count(eventType)

	// Log initial state
	initialCount, _ := counter.Get()
	t.Logf("WaitForEventCount: waiting for %s count to reach %d (current: %d, timeout: %v)",
		eventType, expectedCount, initialCount, timeout)

	err := SleepUntil(func() bool {
		count, _ := counter.Get()
		return count >= expectedCount
	}, timeout)

	// Log final state on timeout
	if err != nil {
		finalCount, _ := counter.Get()
		t.Logf("WaitForEventCount: timeout reached - final count for %s: %d (expected: %d)",
			eventType, finalCount, expectedCount)
	}

	require.NoError(t, err, "Timed out waiting for event %s count to reach %d", eventType, expectedCount)
}

// Release releases resources (corresponds to helpers.ts EventMonitor.release method)
func (em *EventMonitor) Release() {
	if em.cancel != nil {
		em.cancel()
	}
}

// WithEventMonitor provides an EventMonitor for the duration of a test function
func WithEventMonitor(t testing.TB, fn func(*EventMonitor)) {
	t.Helper()
	monitor := NewEventMonitor(t)
	fn(monitor)
}

// AssertEventEmitted asserts that a specific event was emitted at least once
func (em *EventMonitor) AssertEventEmitted(t testing.TB, eventType events.EventType) {
	t.Helper()

	counter := em.Count(eventType)
	count, _ := counter.Get()

	require.Greater(t, count, 0, "Expected event %s to be emitted at least once", eventType)
}

// AssertEventCount asserts the exact count of events emitted
func (em *EventMonitor) AssertEventCount(t testing.TB, eventType events.EventType, expectedCount int) {
	t.Helper()

	counter := em.Count(eventType)
	count, _ := counter.Get()

	require.Equal(t, expectedCount, count, "Expected event %s count to be %d, got %d", eventType, expectedCount, count)
}

// GetLastEvent returns the last event of the specified type
func (em *EventMonitor) GetLastEvent(eventType events.EventType) (events.Event, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if counter, exists := em.counters[eventType]; exists {
		_, lastEvent := counter.Get()
		return lastEvent, true
	}

	return events.Event{}, false
}
