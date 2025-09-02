// Package events provides examples of how to use the Go-idiomatic event system
package events

import (
	"context"
	"fmt"
	"time"
)

// Example demonstrates how to use the event system
func Example() {
	ctx := context.Background()

	// Create an event bus
	bus := NewEventBus(ctx, 100)
	defer bus.Close()

	// Set up default handlers for logging and metrics
	SetupDefaultHandlers(bus)

	// Create a custom handler for job success events
	jobSuccessHandler := func(ctx context.Context, event Event) {
		if jobID, ok := event.Data["jobId"].(int64); ok {
			if workerID, ok := event.Data["workerId"].(string); ok {
				fmt.Printf("🎉 Job %d completed by worker %s\n", jobID, workerID)
			}
		}
	}

	// Subscribe to job success events
	unsubscribe := bus.Subscribe(JobSuccess, jobSuccessHandler)
	defer unsubscribe()

	// Emit some events
	bus.Emit(WorkerCreate, WorkerEventData("worker-123", map[string]interface{}{
		"concurrency": 4,
	}))

	bus.Emit(JobStart, JobEventData("worker-123", 1001, "send_email", map[string]interface{}{
		"priority": 5,
	}))

	// Simulate job completion
	time.Sleep(100 * time.Millisecond)
	bus.Emit(JobSuccess, JobEventData("worker-123", 1001, "send_email", map[string]interface{}{
		"duration_ms": 50,
	}))

	// Simulate job error
	bus.EmitError(JobError, fmt.Errorf("network timeout"), JobEventData("worker-123", 1002, "process_payment", map[string]interface{}{
		"attempt": 1,
	}))

	// Give time for handlers to process
	time.Sleep(200 * time.Millisecond)

	// Print stats
	stats := bus.GetStats()
	fmt.Printf("Event bus stats: %+v\n", stats)
}

// IntegrationExample shows how to integrate with existing Worker and WorkerPool
func IntegrationExample() {
	ctx := context.Background()

	// Create event bus
	bus := NewEventBus(ctx, 1000)
	defer bus.Close()

	// Create emitter for worker components
	emitter := NewBusEmitter(bus)

	// Set up monitoring handlers
	setupMonitoring(bus)

	// Simulate worker lifecycle events
	simulateWorkerLifecycle(emitter)
}

// setupMonitoring configures event handlers for monitoring and observability
func setupMonitoring(bus *EventBus) {
	// Performance monitoring handler
	performanceHandler := func(ctx context.Context, event Event) {
		switch event.Type {
		case JobSuccess:
			if duration, ok := event.Data["duration_ms"].(int); ok {
				// Record job duration metrics
				fmt.Printf("📊 Job duration: %dms\n", duration)
			}
		case JobError:
			// Count job errors
			fmt.Printf("❌ Job error recorded\n")
		case WorkerCreate:
			// Track worker creation
			fmt.Printf("🔧 New worker created\n")
		}
	}

	// Health monitoring handler
	healthHandler := func(ctx context.Context, event Event) {
		switch event.Type {
		case WorkerFatalError:
			// Alert on fatal errors
			fmt.Printf("🚨 ALERT: Worker fatal error - %v\n", event.Error)
		case PoolShutdownError:
			// Alert on shutdown errors
			fmt.Printf("🚨 ALERT: Pool shutdown error - %v\n", event.Error)
		}
	}

	// Subscribe handlers
	bus.Subscribe(JobSuccess, performanceHandler)
	bus.Subscribe(JobError, performanceHandler)
	bus.Subscribe(WorkerCreate, performanceHandler)
	bus.Subscribe(WorkerFatalError, healthHandler)
	bus.Subscribe(PoolShutdownError, healthHandler)
}

// simulateWorkerLifecycle demonstrates event emission during worker operations
func simulateWorkerLifecycle(emitter EventEmitter) {
	workerID := "worker-demo-001"

	// Worker creation
	emitter.EmitEvent(WorkerCreate, WorkerEventData(workerID, map[string]interface{}{
		"version": "1.0.0",
	}))

	// Job processing
	jobID := int64(12345)
	emitter.EmitEvent(JobStart, JobEventData(workerID, jobID, "demo_task", map[string]interface{}{
		"payload_size": 1024,
	}))

	// Simulate processing time
	time.Sleep(50 * time.Millisecond)

	// Job completion
	emitter.EmitEvent(JobSuccess, JobEventData(workerID, jobID, "demo_task", map[string]interface{}{
		"duration_ms": 45,
		"result_size": 512,
	}))

	// Worker shutdown
	emitter.EmitEvent(WorkerRelease, WorkerEventData(workerID, map[string]interface{}{
		"jobs_processed": 1,
	}))

	emitter.EmitEvent(WorkerStop, WorkerEventData(workerID, map[string]interface{}{
		"graceful": true,
	}))
}

// CustomMetricsExample shows how to implement custom metrics collection
func CustomMetricsExample() {
	ctx := context.Background()
	bus := NewEventBus(ctx, 100)
	defer bus.Close()

	// Custom metrics collector
	metrics := &MetricsCollector{
		jobsCompleted: make(map[string]int),
		jobErrors:     make(map[string]int),
	}

	// Subscribe to events
	bus.Subscribe(JobSuccess, metrics.HandleJobSuccess)
	bus.Subscribe(JobError, metrics.HandleJobError)
	bus.Subscribe(JobFailed, metrics.HandleJobFailed)

	// Emit some events
	for i := 0; i < 10; i++ {
		taskType := fmt.Sprintf("task_%d", i%3)
		if i%4 == 0 {
			// Simulate error
			bus.EmitError(JobError, fmt.Errorf("simulated error"), JobEventData("worker-1", int64(i), taskType, nil))
		} else {
			// Simulate success
			bus.Emit(JobSuccess, JobEventData("worker-1", int64(i), taskType, nil))
		}
	}

	time.Sleep(100 * time.Millisecond)
	metrics.PrintStats()
}

// MetricsCollector example implementation
type MetricsCollector struct {
	jobsCompleted map[string]int
	jobErrors     map[string]int
}

func (m *MetricsCollector) HandleJobSuccess(ctx context.Context, event Event) {
	if taskType, ok := event.Data["taskIdentifier"].(string); ok {
		m.jobsCompleted[taskType]++
	}
}

func (m *MetricsCollector) HandleJobError(ctx context.Context, event Event) {
	if taskType, ok := event.Data["taskIdentifier"].(string); ok {
		m.jobErrors[taskType]++
	}
}

func (m *MetricsCollector) HandleJobFailed(ctx context.Context, event Event) {
	if taskType, ok := event.Data["taskIdentifier"].(string); ok {
		m.jobErrors[taskType]++
	}
}

func (m *MetricsCollector) PrintStats() {
	fmt.Println("=== Job Metrics ===")
	fmt.Println("Completed:")
	for task, count := range m.jobsCompleted {
		fmt.Printf("  %s: %d\n", task, count)
	}
	fmt.Println("Errors:")
	for task, count := range m.jobErrors {
		fmt.Printf("  %s: %d\n", task, count)
	}
}
