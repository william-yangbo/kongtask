package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// TestWorkerPool_EventsIntegration tests pool-level events emission
func TestWorkerPool_EventsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a test EventBus
	eventBus := events.NewEventBus(ctx, 100)
	// Don't defer eventBus.Close() to avoid hanging

	// Track emitted events
	var emittedEvents []string
	var eventsMutex sync.Mutex

	// Wait group to synchronize event handling
	var wg sync.WaitGroup

	// Create event handlers
	poolCreateHandler := func(handlerCtx context.Context, event events.Event) {
		defer wg.Done()
		eventsMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.PoolCreate))
		eventsMutex.Unlock()
		t.Logf("Event: %s - %v", events.PoolCreate, event.Data)
	}

	poolListenConnectingHandler := func(handlerCtx context.Context, event events.Event) {
		defer wg.Done()
		eventsMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.PoolListenConnecting))
		eventsMutex.Unlock()
		t.Logf("Event: %s - %v", events.PoolListenConnecting, event.Data)
	}

	poolReleaseHandler := func(handlerCtx context.Context, event events.Event) {
		defer wg.Done()
		eventsMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.PoolRelease))
		eventsMutex.Unlock()
		t.Logf("Event: %s - %v", events.PoolRelease, event.Data)
	}

	poolGracefulShutdownHandler := func(handlerCtx context.Context, event events.Event) {
		defer wg.Done()
		eventsMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.PoolGracefulShutdown))
		eventsMutex.Unlock()
		t.Logf("Event: %s - %v", events.PoolGracefulShutdown, event.Data)
	}

	// Subscribe to pool events
	eventBus.Subscribe(events.PoolCreate, poolCreateHandler)
	eventBus.Subscribe(events.PoolListenConnecting, poolListenConnectingHandler)
	eventBus.Subscribe(events.PoolRelease, poolReleaseHandler)
	eventBus.Subscribe(events.PoolGracefulShutdown, poolGracefulShutdownHandler)

	// Test options
	options := WorkerPoolOptions{
		Concurrency:          1,
		Schema:               "test_schema",
		Logger:               logger.DefaultLogger,
		Events:               eventBus,
		MaxPoolSize:          2,
		MaxContiguousErrors:  10,
		PollInterval:         time.Second,
		NoHandleSignals:      true, // Disable signal handling for test
		NoPreparedStatements: false,
	}

	// Test EventBus integration
	compiled := ProcessSharedOptions(&options, nil)
	if compiled.Events != eventBus {
		t.Errorf("Expected EventBus to be passed through ProcessSharedOptions")
	}

	// Manually emit events to test the infrastructure
	wg.Add(4) // Expecting 4 events

	eventBus.Emit(events.PoolCreate, map[string]interface{}{
		"workerPool": "test-pool",
	})

	eventBus.Emit(events.PoolListenConnecting, map[string]interface{}{
		"workerPool": "test-pool",
	})

	eventBus.Emit(events.PoolRelease, map[string]interface{}{
		"pool": "test-pool",
	})

	eventBus.Emit(events.PoolGracefulShutdown, map[string]interface{}{
		"pool":    "test-pool",
		"message": "Test shutdown",
	})

	// Wait for all events to be processed with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All events processed successfully")
	case <-ctx.Done():
		t.Error("Test timeout: events were not processed in time")
		return
	}

	// Verify events were emitted
	eventsMutex.Lock()
	defer eventsMutex.Unlock()

	expectedEvents := []string{
		string(events.PoolCreate),
		string(events.PoolListenConnecting),
		string(events.PoolRelease),
		string(events.PoolGracefulShutdown),
	}

	if len(emittedEvents) != len(expectedEvents) {
		t.Errorf("Expected exactly %d events, got %d", len(expectedEvents), len(emittedEvents))
	}

	// Check that all expected events were emitted
	for _, expectedEvent := range expectedEvents {
		found := false
		for _, emittedEvent := range emittedEvents {
			if emittedEvent == expectedEvent {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected event %s was not emitted", expectedEvent)
		}
	}

	t.Logf("Successfully emitted %d pool events: %v", len(emittedEvents), emittedEvents)
}
