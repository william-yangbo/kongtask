// Package events provides a Go-idiomatic event system for kongtask
// following Go best practices with channels, interfaces and context
package events

import (
	"context"
	"sync"
	"time"

	"github.com/william-yangbo/kongtask/pkg/logger"
)

// EventType represents the type of event
type EventType string

const (
	// Pool Events
	PoolCreate           EventType = "pool:create"
	PoolListenConnecting EventType = "pool:listen:connecting"
	PoolListenSuccess    EventType = "pool:listen:success"
	PoolListenError      EventType = "pool:listen:error"
	PoolRelease          EventType = "pool:release"
	PoolGracefulShutdown EventType = "pool:gracefulShutdown"
	PoolShutdownError    EventType = "pool:gracefulShutdown:error"

	// Worker Events
	WorkerCreate      EventType = "worker:create"
	WorkerRelease     EventType = "worker:release"
	WorkerStop        EventType = "worker:stop"
	WorkerGetJobStart EventType = "worker:getJob:start"
	WorkerGetJobError EventType = "worker:getJob:error"
	WorkerGetJobEmpty EventType = "worker:getJob:empty"
	WorkerFatalError  EventType = "worker:fatalError"

	// Job Events
	JobStart    EventType = "job:start"
	JobSuccess  EventType = "job:success"
	JobError    EventType = "job:error"
	JobFailed   EventType = "job:failed"
	JobComplete EventType = "job:complete"

	// Lifecycle Events
	GracefulShutdown EventType = "gracefulShutdown"
	Stop             EventType = "stop"
)

// Event represents a single event with metadata
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     error                  `json:"error,omitempty"`
}

// EventHandler is a function that handles events
// Following Go convention for handler functions
type EventHandler func(ctx context.Context, event Event)

// EventBus provides a Go-idiomatic event system using channels
type EventBus struct {
	mu        sync.RWMutex
	handlers  map[EventType][]EventHandler
	eventChan chan Event
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	logger    *logger.Logger
}

// NewEventBus creates a new event bus with buffered channel
func NewEventBus(ctx context.Context, bufferSize int) *EventBus {
	busCtx, cancel := context.WithCancel(ctx)

	bus := &EventBus{
		handlers:  make(map[EventType][]EventHandler),
		eventChan: make(chan Event, bufferSize),
		ctx:       busCtx,
		cancel:    cancel,
		logger:    logger.DefaultLogger.Scope(logger.LogScope{Label: "events"}),
	}

	// Start event processing goroutine
	bus.wg.Add(1)
	go bus.processEvents()

	return bus
}

// Subscribe adds an event handler for the specified event type
// Returns an unsubscribe function following Go conventions
func (bus *EventBus) Subscribe(eventType EventType, handler EventHandler) func() {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	bus.handlers[eventType] = append(bus.handlers[eventType], handler)

	// Return unsubscribe function
	return func() {
		bus.mu.Lock()
		defer bus.mu.Unlock()

		handlers := bus.handlers[eventType]
		for i, h := range handlers {
			// Compare function pointers (works for most cases)
			if &h == &handler {
				bus.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}
}

// Emit sends an event asynchronously
// Non-blocking to prevent deadlocks
func (bus *EventBus) Emit(eventType EventType, data map[string]interface{}) {
	event := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}

	select {
	case bus.eventChan <- event:
		// Event sent successfully
	case <-bus.ctx.Done():
		// Bus is shutting down
		return
	default:
		// Channel is full, log warning but don't block
		bus.logger.Warn("Event channel full, dropping event", logger.LogMeta{
			"eventType": string(eventType),
			"data":      data,
		})
	}
}

// EmitError sends an event with an error
func (bus *EventBus) EmitError(eventType EventType, err error, data map[string]interface{}) {
	event := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
		Error:     err,
	}

	select {
	case bus.eventChan <- event:
		// Event sent successfully
	case <-bus.ctx.Done():
		// Bus is shutting down
		return
	default:
		// Channel is full, log warning but don't block
		bus.logger.Warn("Event channel full, dropping error event", logger.LogMeta{
			"eventType": string(eventType),
			"error":     err.Error(),
			"data":      data,
		})
	}
}

// processEvents processes events from the channel
func (bus *EventBus) processEvents() {
	defer bus.wg.Done()

	for {
		select {
		case event := <-bus.eventChan:
			bus.handleEvent(event)
		case <-bus.ctx.Done():
			// Process remaining events before shutdown
			for {
				select {
				case event := <-bus.eventChan:
					bus.handleEvent(event)
				default:
					return
				}
			}
		}
	}
}

// handleEvent dispatches event to registered handlers
func (bus *EventBus) handleEvent(event Event) {
	bus.mu.RLock()
	handlers := make([]EventHandler, len(bus.handlers[event.Type]))
	copy(handlers, bus.handlers[event.Type])
	bus.mu.RUnlock()

	// Call handlers concurrently but with timeout
	for _, handler := range handlers {
		go func(h EventHandler) {
			// Create timeout context for handler execution
			handlerCtx, cancel := context.WithTimeout(bus.ctx, 5*time.Second)
			defer cancel()

			// Recover from panics in handlers
			defer func() {
				if r := recover(); r != nil {
					bus.logger.Error("Event handler panicked", logger.LogMeta{
						"eventType": string(event.Type),
						"panic":     r,
					})
				}
			}()

			h(handlerCtx, event)
		}(handler)
	}
}

// Close gracefully shuts down the event bus
func (bus *EventBus) Close() error {
	bus.cancel()

	// Close the event channel to signal no more events
	close(bus.eventChan)

	// Wait for all goroutines to finish
	bus.wg.Wait()

	return nil
}

// GetStats returns statistics about the event bus
func (bus *EventBus) GetStats() map[string]interface{} {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_event_types"] = len(bus.handlers)
	stats["channel_length"] = len(bus.eventChan)
	stats["channel_capacity"] = cap(bus.eventChan)

	handlerCounts := make(map[string]int)
	for eventType, handlers := range bus.handlers {
		handlerCounts[string(eventType)] = len(handlers)
	}
	stats["handler_counts"] = handlerCounts

	return stats
}
