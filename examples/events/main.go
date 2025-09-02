// Package main demonstrates kongtask events usage, corresponding to graphile-worker events.js example
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// This example corresponds to the graphile-worker events.js example
// It shows how to:
// 1. Set up a worker with event listeners
// 2. Listen for job:success events
// 3. Add jobs to be executed

func main() {
	if err := runWorkerWithEvents(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}

	// Also demonstrate direct event emission
	fmt.Println("\n--- Demonstrating direct event emission ---")
	demonstrateEventEmission()
}

func runWorkerWithEvents() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Database connection string (equivalent to JavaScript connectionString)
	connectionString := os.Getenv("DATABASE_URL")
	if connectionString == "" {
		connectionString = "postgres://postgres:postgres@localhost:5432/kongtask_dev?sslmode=disable"
	}

	// Create event bus for worker events
	eventBus := events.NewEventBus(ctx, 1000)

	// Set up event listener for job:success (corresponding to JavaScript runner.events.on("job:success"))
	eventBus.Subscribe(events.JobSuccess, func(ctx context.Context, event events.Event) {
		// Extract worker and job information from event data
		workerID := "unknown"
		jobID := "unknown"

		if wid, ok := event.Data["workerId"].(string); ok {
			workerID = wid
		}
		if jid, ok := event.Data["jobId"]; ok {
			jobID = fmt.Sprintf("%v", jid)
		}

		// This corresponds to: console.log(`Hooray! Worker ${worker.workerId} completed job ${job.id}`);
		fmt.Printf("🎉 Hooray! Worker %s completed job %s\n", workerID, jobID)
	}) // Task list definition (corresponding to JavaScript taskList)
	taskList := map[string]worker.TaskHandler{
		"hello": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			// Parse payload to get name
			var payloadMap map[string]interface{}
			if err := json.Unmarshal(payload, &payloadMap); err != nil {
				return fmt.Errorf("invalid payload format: %w", err)
			}

			// Extract name from payload (corresponding to const { name } = payload;)
			name := "World"
			if n, ok := payloadMap["name"].(string); ok {
				name = n
			}

			// This corresponds to: helpers.logger.info(`Hello, ${name}`);
			fmt.Printf("👋 Hello, %s!\n", name)

			// Emit a job success event manually for demonstration
			eventBus.Emit(events.JobSuccess, map[string]interface{}{
				"workerId":       "demo-worker",
				"jobId":          fmt.Sprintf("job-%d", time.Now().Unix()),
				"taskIdentifier": "hello",
				"name":           name,
			})

			return nil
		},
	}

	// Runner configuration (corresponding to JavaScript run() options)
	// Note: This would be used for actual worker execution
	_ = worker.RunnerOptions{
		ConnectionString: connectionString,
		TaskList:         taskList,
	}

	// For demonstration, we'll run once (corresponding to JavaScript runOnce)
	fmt.Println("🚀 Starting worker to process tasks...")

	// Note: In a real scenario, we would add jobs to the database first
	// For this example, we'll demonstrate the event system working

	// Simulate some job processing events
	go func() {
		time.Sleep(500 * time.Millisecond)

		// Simulate worker create (there is no WorkerStart, use WorkerCreate)
		eventBus.Emit(events.WorkerCreate, map[string]interface{}{
			"workerId": "demo-worker",
		}) // Simulate job start
		eventBus.Emit(events.JobStart, map[string]interface{}{
			"workerId":       "demo-worker",
			"jobId":          "demo-job-001",
			"taskIdentifier": "hello",
		})

		// Process the task (this would trigger our task handler)
		taskHandler := taskList["hello"]
		payload, _ := json.Marshal(map[string]interface{}{"name": "Bobby Tables"})

		if err := taskHandler(ctx, payload, &worker.Helpers{}); err != nil {
			fmt.Printf("Task execution error: %v\n", err)
		}
	}()

	// Let events process
	time.Sleep(2 * time.Second)

	fmt.Println("✅ Event processing completed")
	return nil
}

// demonstrateEventEmission shows direct event emission (for testing/demonstration)
func demonstrateEventEmission() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	bus := events.NewEventBus(ctx, 1000)

	// Set up listeners for different event types
	bus.Subscribe(events.WorkerCreate, func(ctx context.Context, event events.Event) {
		fmt.Printf("🏭 Worker created: %+v\n", event.Data)
	})

	bus.Subscribe(events.JobStart, func(ctx context.Context, event events.Event) {
		fmt.Printf("▶️  Job started: %+v\n", event.Data)
	})

	bus.Subscribe(events.JobSuccess, func(ctx context.Context, event events.Event) {
		fmt.Printf("✅ Job completed successfully: %+v\n", event.Data)
	})

	bus.Subscribe(events.JobFailed, func(ctx context.Context, event events.Event) {
		fmt.Printf("❌ Job failed: %+v\n", event.Data)
	})

	// Emit events manually (this would normally be done by the worker)
	fmt.Println("📡 Emitting demonstration events...")

	bus.Emit(events.WorkerCreate, map[string]interface{}{
		"workerId":    "demo-worker-001",
		"concurrency": 5,
	})

	bus.Emit(events.JobStart, map[string]interface{}{
		"workerId":       "demo-worker-001",
		"jobId":          "demo-job-12345",
		"taskIdentifier": "hello",
	})

	bus.Emit(events.JobSuccess, map[string]interface{}{
		"workerId":       "demo-worker-001",
		"jobId":          "demo-job-12345",
		"taskIdentifier": "hello",
		"duration_ms":    150,
	})

	bus.Emit(events.JobFailed, map[string]interface{}{
		"workerId":       "demo-worker-001",
		"jobId":          "demo-job-67890",
		"taskIdentifier": "problematic_task",
		"error":          "Simulated error for demonstration",
	})

	// Let events process
	time.Sleep(2 * time.Second)

	fmt.Println("🎯 All demonstration events processed!")

	// Context cancellation will clean up the EventBus automatically
	// No need to call Close() explicitly
}
