// Enhanced Error Handling Example
// This example demonstrates the new error handling and panic recovery features
// synchronized from graphile-worker commit 79f2160

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

func main() {
	// Connect to PostgreSQL
	pool, err := pgxpool.New(context.Background(),
		"postgres://user:password@localhost/dbname")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Create context for the worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up event listener to monitor errors
	eventBus := events.NewEventBus(ctx, 100)
	defer func() {
		if err := eventBus.Close(); err != nil {
			fmt.Printf("Failed to close event bus: %v\n", err)
		}
	}()

	// Subscribe to pool error events
	eventBus.Subscribe(events.PoolError, func(eventCtx context.Context, event events.Event) {
		fmt.Printf("Pool error detected: %v\n", event.Data)
	})

	// Subscribe to pool graceful shutdown events
	eventBus.Subscribe(events.PoolGracefulShutdown, func(eventCtx context.Context, event events.Event) {
		fmt.Printf("Pool shutting down: %v\n", event.Data)
	})

	// Define task handlers with potential for panic
	tasks := map[string]worker.TaskHandler{
		"safe_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			log.Printf("Processing safe task: %s", payload)
			return nil
		},
		"risky_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var data map[string]interface{}
			if err := json.Unmarshal(payload, &data); err != nil {
				return err
			}

			// Simulate a scenario that might panic
			if shouldPanic, ok := data["panic"].(bool); ok && shouldPanic {
				panic("Simulated panic in risky_task")
			}

			log.Printf("Processing risky task safely: %s", payload)
			return nil
		},
		"critical_error_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			// Simulate a critical error that should trigger shutdown
			return fmt.Errorf("connection refused")
		},
	}

	// Create worker pool with enhanced error handling
	workerPool, err := worker.RunTaskList(ctx, worker.WorkerPoolOptions{
		Concurrency: 2,
		Schema:      "graphile_worker",
		Events:      eventBus,
	}, tasks, pool)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := workerPool.Release(); err != nil {
			fmt.Printf("Failed to release worker pool: %v\n", err)
		}
	}()

	// Add some test jobs
	w := worker.NewWorker(pool, "graphile_worker")

	// Safe job
	err = w.AddJob(context.Background(), "safe_task", map[string]string{
		"message": "This is a safe task",
	})
	if err != nil {
		log.Printf("Failed to add safe job: %v", err)
	}

	// Risky job that will panic
	err = w.AddJob(context.Background(), "risky_task", map[string]interface{}{
		"message": "This task will panic",
		"panic":   true,
	})
	if err != nil {
		log.Printf("Failed to add risky job: %v", err)
	}

	// Risky job that won't panic
	err = w.AddJob(context.Background(), "risky_task", map[string]interface{}{
		"message": "This task will not panic",
		"panic":   false,
	})
	if err != nil {
		log.Printf("Failed to add safe risky job: %v", err)
	}

	// Wait for jobs to be processed (with timeout)
	fmt.Println("Processing jobs...")
	time.Sleep(5 * time.Second)

	// Add a critical error job that should trigger shutdown
	err = w.AddJob(context.Background(), "critical_error_task", map[string]string{
		"message": "This will trigger a critical error",
	})
	if err != nil {
		log.Printf("Failed to add critical error job: %v", err)
	}

	// Wait for the worker pool to complete and check for critical errors
	fmt.Println("Waiting for worker pool to complete...")
	if criticalErr := workerPool.Wait(); criticalErr != nil {
		fmt.Printf("Worker pool stopped due to critical error: %v\n", criticalErr)
	} else {
		fmt.Println("Worker pool completed successfully")
	}

	fmt.Println("Example completed")
}
