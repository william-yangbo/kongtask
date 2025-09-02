package events

import (
	"context"

	"github.com/william-yangbo/kongtask/pkg/logger"
)

// EventEmitter is an interface for components that can emit events
// Following Go interface best practices
type EventEmitter interface {
	EmitEvent(eventType EventType, data map[string]interface{})
	EmitError(eventType EventType, err error, data map[string]interface{})
}

// NullEmitter is a no-op implementation of EventEmitter
// Follows null object pattern for optional event handling
type NullEmitter struct{}

func (n *NullEmitter) EmitEvent(eventType EventType, data map[string]interface{}) {
	// No-op
}

func (n *NullEmitter) EmitError(eventType EventType, err error, data map[string]interface{}) {
	// No-op
}

// BusEmitter wraps EventBus to implement EventEmitter interface
type BusEmitter struct {
	bus *EventBus
}

func NewBusEmitter(bus *EventBus) *BusEmitter {
	return &BusEmitter{bus: bus}
}

func (e *BusEmitter) EmitEvent(eventType EventType, data map[string]interface{}) {
	if e.bus != nil {
		e.bus.Emit(eventType, data)
	}
}

func (e *BusEmitter) EmitError(eventType EventType, err error, data map[string]interface{}) {
	if e.bus != nil {
		e.bus.EmitError(eventType, err, data)
	}
}

// Utility functions for creating common event data structures

// WorkerEventData creates event data for worker-related events
func WorkerEventData(workerID string, additional map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"workerId": workerID,
	}
	for k, v := range additional {
		data[k] = v
	}
	return data
}

// JobEventData creates event data for job-related events
func JobEventData(workerID string, jobID int64, taskIdentifier string, additional map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"workerId":       workerID,
		"jobId":          jobID,
		"taskIdentifier": taskIdentifier,
	}
	for k, v := range additional {
		data[k] = v
	}
	return data
}

// PoolEventData creates event data for pool-related events
func PoolEventData(poolID string, additional map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"poolId": poolID,
	}
	for k, v := range additional {
		data[k] = v
	}
	return data
}

// Common event handlers for typical use cases

// LoggingHandler creates an event handler that logs all events
func LoggingHandler() EventHandler {
	return func(ctx context.Context, event Event) {
		// This will use the default logger to log events
		// Users can customize this behavior
		switch event.Type {
		case JobSuccess:
			// Extract job information for success logging
			if jobID, ok := event.Data["jobId"].(int64); ok {
				if workerID, ok := event.Data["workerId"].(string); ok {
					// Format similar to existing kongtask success logs
					logger.DefaultLogger.Info("Job completed successfully",
						logger.LogMeta{
							"jobId":    jobID,
							"workerId": workerID,
						})
				}
			}
		case JobError, JobFailed:
			// Log job errors
			if jobID, ok := event.Data["jobId"].(int64); ok {
				if workerID, ok := event.Data["workerId"].(string); ok {
					logger.DefaultLogger.Error("Job failed",
						logger.LogMeta{
							"jobId":    jobID,
							"workerId": workerID,
							"error":    event.Error,
						})
				}
			}
		case WorkerFatalError:
			// Log worker fatal errors
			if workerID, ok := event.Data["workerId"].(string); ok {
				logger.DefaultLogger.Error("Worker fatal error",
					logger.LogMeta{
						"workerId": workerID,
						"error":    event.Error,
					})
			}
		default:
			// Debug log for other events
			logger.DefaultLogger.Debug("Event emitted",
				logger.LogMeta{
					"eventType": string(event.Type),
					"data":      event.Data,
				})
		}
	}
}

// MetricsHandler creates an event handler that tracks metrics
// This can be extended to integrate with Prometheus, StatsD, etc.
func MetricsHandler() EventHandler {
	return func(ctx context.Context, event Event) {
		// Example implementation - can be extended with actual metrics library
		switch event.Type {
		case JobSuccess:
			// Increment job success counter
			// metrics.IncrementCounter("jobs.success")
		case JobError:
			// Increment job error counter
			// metrics.IncrementCounter("jobs.error")
		case JobFailed:
			// Increment job failed counter
			// metrics.IncrementCounter("jobs.failed")
		case WorkerCreate:
			// Track worker creation
			// metrics.SetGauge("workers.active", currentWorkerCount)
		case WorkerStop:
			// Track worker shutdown
			// metrics.SetGauge("workers.active", currentWorkerCount)
		}
	}
}

// Example usage and integration helpers

// DefaultEventBus creates a default event bus with reasonable settings
func DefaultEventBus(ctx context.Context) *EventBus {
	return NewEventBus(ctx, 1000) // Buffer size of 1000 events
}

// SetupDefaultHandlers configures common event handlers
func SetupDefaultHandlers(bus *EventBus) {
	// Subscribe to all event types for logging
	allEventTypes := []EventType{
		PoolCreate, PoolListenConnecting, PoolListenSuccess, PoolListenError,
		PoolRelease, PoolGracefulShutdown, PoolShutdownError,
		WorkerCreate, WorkerRelease, WorkerStop, WorkerGetJobError,
		WorkerGetJobEmpty, WorkerFatalError,
		JobStart, JobSuccess, JobError, JobFailed,
		GracefulShutdown, Stop,
	}

	loggingHandler := LoggingHandler()
	metricsHandler := MetricsHandler()

	for _, eventType := range allEventTypes {
		bus.Subscribe(eventType, loggingHandler)
		bus.Subscribe(eventType, metricsHandler)
	}
}
