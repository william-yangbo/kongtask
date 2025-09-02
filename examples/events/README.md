# kongtask Events Example

This example demonstrates kongtask's event system, which corresponds to the graphile-worker `events.js` example.

## Overview

This example shows how to:

1. Set up a worker with event listeners
2. Listen for various job lifecycle events (job:start, job:success, job:failed)
3. Emit events during task processing
4. Handle events with proper Go context management

## Key Features

- **Event-driven Architecture**: Uses Go channels and goroutines for efficient event processing
- **Context Support**: Proper context handling for cancellation and timeouts
- **Type Safety**: Strongly typed event constants and structured event data
- **JavaScript Parity**: Mirrors the functionality of graphile-worker's events system

## Event Types

The system supports all the same event types as graphile-worker:

### Worker Events

- `worker:create` - When a worker is created
- `worker:start` - When a worker starts processing
- `worker:stop` - When a worker stops

### Pool Events

- `pool:create` - When a worker pool is created
- `pool:start` - When a pool starts listening for jobs
- `pool:stop` - When a pool stops

### Job Events

- `job:start` - When a job begins processing
- `job:success` - When a job completes successfully
- `job:failed` - When a job fails
- `job:error` - When a job encounters an error

## Usage

```bash
# Build the example
go build -o events-example main.go

# Run the example
./events-example
```

## Code Structure

The example consists of two main parts:

### 1. Worker with Events (`runWorkerWithEvents`)

This function demonstrates:

- Creating an EventBus
- Subscribing to job events (particularly `job:success`)
- Simulating job processing with event emission
- Proper cleanup with context cancellation

### 2. Direct Event Emission (`demonstrateEventEmission`)

This function shows:

- Manual event emission for testing
- Multiple event listeners
- Event data handling

## Correspondence with JavaScript

This Go example corresponds to the graphile-worker `events.js` example:

**JavaScript:**

```javascript
runner.events.on('job:success', ({ worker, job }) => {
  console.log(`Hooray! Worker ${worker.workerId} completed job ${job.id}`);
});
```

**Go:**

```go
eventBus.Subscribe(events.JobSuccess, func(ctx context.Context, event events.Event) {
  workerID := event.Data["workerId"].(string)
  jobID := fmt.Sprintf("%v", event.Data["jobId"])
  fmt.Printf("🎉 Hooray! Worker %s completed job %s\n", workerID, jobID)
})
```

## Integration with kongtask Worker

While this example demonstrates the event system in isolation, in a real application you would:

1. Integrate the EventBus with the actual kongtask worker
2. Use real database connections and job processing
3. Subscribe to events from actual worker lifecycle events

The event system is designed to be thread-safe and can handle high-throughput scenarios with proper buffering and context management.
