package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// TestWorkerPanicRecovery tests that worker panics are properly recovered and handled
func TestWorkerPanicRecovery(t *testing.T) {
	// Create a test WorkerPool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventBus := events.NewEventBus(ctx, 10)
	defer func() {
		if err := eventBus.Close(); err != nil {
			t.Logf("Failed to close event bus: %v", err)
		}
	}()

	wp := &WorkerPool{
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger.DefaultLogger,
		eventBus:         eventBus,
		errorChan:        make(chan error, 10),
		shutdownComplete: make(chan struct{}),
	}

	// Start error monitor
	wp.errorHandlerWG.Add(1)
	go wp.startErrorMonitor()

	// Simulate a worker panic
	panicErr := fmt.Errorf("test panic from worker 0: simulated panic")

	// Send panic error to error channel
	select {
	case wp.errorChan <- panicErr:
	case <-time.After(time.Second):
		t.Fatal("Failed to send panic error to error channel")
	}

	// Wait a bit for error processing
	time.Sleep(100 * time.Millisecond)

	// Check that critical error was recorded by attempting to get it safely
	wp.criticalErrorMu.RLock()
	criticalErr := wp.criticalError
	wp.criticalErrorMu.RUnlock()

	if criticalErr == nil {
		t.Error("Expected critical error to be set")
	} else if criticalErr.Error() != panicErr.Error() {
		t.Errorf("Expected critical error %v, got %v", panicErr, criticalErr)
	}

	// Cleanup
	cancel()
	wp.errorHandlerWG.Wait()
}

// TestIsCriticalError tests the error classification logic
func TestIsCriticalError(t *testing.T) {
	wp := &WorkerPool{}

	testCases := []struct {
		name     string
		err      error
		critical bool
	}{
		{
			name:     "nil error",
			err:      nil,
			critical: false,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			critical: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			critical: false,
		},
		{
			name:     "connection refused",
			err:      fmt.Errorf("connection refused"),
			critical: true,
		},
		{
			name:     "connection closed",
			err:      fmt.Errorf("connection closed"),
			critical: true,
		},
		{
			name:     "server closed connection",
			err:      fmt.Errorf("server closed the connection unexpectedly"),
			critical: true,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some generic error"),
			critical: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := wp.isCriticalError(tc.err)
			if result != tc.critical {
				t.Errorf("Expected isCriticalError(%v) = %v, got %v", tc.err, tc.critical, result)
			}
		})
	}
}

// TestErrorMonitorShutdown tests that the error monitor properly handles shutdown
func TestErrorMonitorShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	eventBus := events.NewEventBus(ctx, 10)
	defer func() {
		if err := eventBus.Close(); err != nil {
			t.Logf("Failed to close event bus: %v", err)
		}
	}()

	wp := &WorkerPool{
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger.DefaultLogger,
		eventBus:         eventBus,
		errorChan:        make(chan error, 10),
		shutdownComplete: make(chan struct{}),
	}

	// Start error monitor
	wp.errorHandlerWG.Add(1)
	go wp.startErrorMonitor()

	// Cancel context to trigger shutdown
	cancel()

	// Wait for error monitor to finish
	done := make(chan struct{})
	go func() {
		wp.errorHandlerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("Error monitor did not shut down within timeout")
	}
}
