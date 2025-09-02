package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Simple test that doesn't use logger to avoid hanging
func TestEventConstants_Simple(t *testing.T) {
	// Test event string values
	if string(PoolCreate) != "pool:create" {
		t.Errorf("PoolCreate should be 'pool:create', got %s", string(PoolCreate))
	}

	if string(WorkerCreate) != "worker:create" {
		t.Errorf("WorkerCreate should be 'worker:create', got %s", string(WorkerCreate))
	}

	if string(JobStart) != "job:start" {
		t.Errorf("JobStart should be 'job:start', got %s", string(JobStart))
	}
}

// Test basic EventBus functionality without logger
func TestEventBus_Basic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use simplified EventBus that doesn't need logger
	bus := &EventBus{
		handlers:  make(map[EventType][]EventHandler),
		eventChan: make(chan Event, 10),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start processing manually without logger
	go func() {
		for {
			select {
			case event := <-bus.eventChan:
				bus.handleEventSimple(event)
			case <-ctx.Done():
				return
			}
		}
	}()

	var receivedEvents []Event
	var mu sync.Mutex

	handler := func(ctx context.Context, event Event) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
	}

	// Test subscription
	bus.Subscribe(JobStart, handler)

	// Test emission
	testData := map[string]interface{}{"jobId": "test-123"}
	select {
	case bus.eventChan <- Event{
		Type:      JobStart,
		Timestamp: time.Now(),
		Data:      testData,
	}:
		// Event sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Failed to send event")
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(receivedEvents))
	}

	if len(receivedEvents) > 0 {
		event := receivedEvents[0]
		if event.Type != JobStart {
			t.Errorf("Expected event type %s, got %s", JobStart, event.Type)
		}

		if event.Data["jobId"] != "test-123" {
			t.Errorf("Expected jobId test-123, got %v", event.Data["jobId"])
		}
	}
}

// handleEventSimple is a simplified version without logger
func (bus *EventBus) handleEventSimple(event Event) {
	bus.mu.RLock()
	handlers := make([]EventHandler, len(bus.handlers[event.Type]))
	copy(handlers, bus.handlers[event.Type])
	bus.mu.RUnlock()

	// Call handlers
	for _, handler := range handlers {
		go func(h EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					// Ignore panics in simple test
				}
			}()

			h(bus.ctx, event)
		}(handler)
	}
}
