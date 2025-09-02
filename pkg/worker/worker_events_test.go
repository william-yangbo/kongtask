package worker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/william-yangbo/kongtask/pkg/events"
)

func TestWorker_EventsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create event bus
	eventBus := events.NewEventBus(ctx, 1000)

	// Track emitted events
	emittedEvents := make([]string, 0)
	var eventMutex sync.Mutex

	// Subscribe to all relevant events
	eventBus.Subscribe(events.WorkerCreate, func(ctx context.Context, event events.Event) {
		eventMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.WorkerCreate))
		eventMutex.Unlock()
		t.Logf("Event: %s - %+v", events.WorkerCreate, event.Data)
	})

	eventBus.Subscribe(events.WorkerRelease, func(ctx context.Context, event events.Event) {
		eventMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.WorkerRelease))
		eventMutex.Unlock()
		t.Logf("Event: %s - %+v", events.WorkerRelease, event.Data)
	})

	eventBus.Subscribe(events.WorkerStop, func(ctx context.Context, event events.Event) {
		eventMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.WorkerStop))
		eventMutex.Unlock()
		t.Logf("Event: %s - %+v", events.WorkerStop, event.Data)
	})

	eventBus.Subscribe(events.WorkerGetJobEmpty, func(ctx context.Context, event events.Event) {
		eventMutex.Lock()
		emittedEvents = append(emittedEvents, string(events.WorkerGetJobEmpty))
		eventMutex.Unlock()
		t.Logf("Event: %s - %+v", events.WorkerGetJobEmpty, event.Data)
	})

	// Create a mock worker (we'll just test event emission, not actual DB operations)
	worker := &Worker{
		workerID: "test-worker-001",
		handlers: make(map[string]TaskHandler),
		eventBus: eventBus,
		active:   true,
	}

	// Test worker:create event when first task is registered
	worker.RegisterTask("test_task", func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error {
		t.Logf("Processing test task with payload: %s", string(payload))
		return nil
	})

	// Test worker:release and worker:stop events
	worker.Release()

	// Give events time to process
	time.Sleep(500 * time.Millisecond)

	// Verify events were emitted
	eventMutex.Lock()
	defer eventMutex.Unlock()

	expectedEvents := []string{
		string(events.WorkerCreate),
		string(events.WorkerRelease),
		string(events.WorkerStop),
	}

	if len(emittedEvents) < len(expectedEvents) {
		t.Errorf("Expected at least %d events, got %d", len(expectedEvents), len(emittedEvents))
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

	t.Logf("Successfully emitted %d events: %v", len(emittedEvents), emittedEvents)

	// Let context cancellation clean up the EventBus
}
