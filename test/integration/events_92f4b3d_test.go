package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestWorkerGetJobStartAndJobComplete tests the new events added in commit 92f4b3d
func TestWorkerGetJobStartAndJobComplete(t *testing.T) {
	dbURL, pool := testutil.StartPostgres(t)
	_ = dbURL

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	schema := "graphile_worker"
	testutil.Reset(t, pool, schema)

	eventBus := events.NewEventBus(ctx, 100)
	// Don't defer Close() here to avoid deadlock

	var emittedEvents []events.Event
	var mu sync.Mutex

	eventBus.Subscribe(events.WorkerGetJobStart, func(ctx context.Context, event events.Event) {
		mu.Lock()
		defer mu.Unlock()
		emittedEvents = append(emittedEvents, event)
	})

	eventBus.Subscribe(events.JobComplete, func(ctx context.Context, event events.Event) {
		mu.Lock()
		defer mu.Unlock()
		emittedEvents = append(emittedEvents, event)
	})

	eventCount := func(eventType events.EventType) int {
		mu.Lock()
		defer mu.Unlock()
		count := 0
		for _, event := range emittedEvents {
			if event.Type == eventType {
				count++
			}
		}
		return count
	}

	w := worker.NewWorker(pool, schema, worker.WithEventBus(eventBus))

	taskIdentifier := "test_task_92f4b3d"
	var jobProcessed bool
	var jobProcessedMu sync.Mutex

	w.RegisterTask(taskIdentifier, func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		jobProcessedMu.Lock()
		jobProcessed = true
		jobProcessedMu.Unlock()
		return nil
	})

	addJobErr := w.AddJob(ctx, taskIdentifier, map[string]interface{}{
		"message": "test job for events 92f4b3d",
	})
	require.NoError(t, addJobErr)

	err := w.RunOnce(ctx)
	require.NoError(t, err)

	// Give some time for events to be processed
	time.Sleep(200 * time.Millisecond)

	jobProcessedMu.Lock()
	processed := jobProcessed
	jobProcessedMu.Unlock()
	assert.True(t, processed, "Job should have been processed")

	assert.GreaterOrEqual(t, eventCount(events.WorkerGetJobStart), 1,
		"At least one worker:getJob:start event should be emitted")

	assert.Equal(t, 1, eventCount(events.JobComplete),
		"Exactly one job:complete event should be emitted")

	// Cancel the context to stop the event bus
	cancel()
}
