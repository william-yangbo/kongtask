package worker

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// WorkerPoolOptions represents options for worker pool
type WorkerPoolOptions struct {
	Concurrency          int              // Number of concurrent workers
	Schema               string           // Database schema (default: "graphile_worker")
	PollInterval         time.Duration    // Polling interval (default: 1s)
	Logger               *logger.Logger   // Logger instance
	NoHandleSignals      bool             // If set true, we won't install signal handlers (v0.5.0 feature)
	MaxPoolSize          int              // Maximum database connection pool size
	MaxContiguousErrors  int              // Maximum contiguous errors before worker stops
	DatabaseURL          string           // Database connection URL
	PgPool               *pgxpool.Pool    // Existing database pool (alternative to DatabaseURL)
	NoPreparedStatements bool             // If set true, disable prepared statements for pgBouncer compatibility
	Events               *events.EventBus // EventBus for worker events (lib.ts alignment)
	UseNodeTime          bool             // Use Node's time source rather than PostgreSQL's (commit 5a09a37 alignment)
}

// generatePoolID generates a cryptographically secure random pool identifier
// This improves upon timestamp-based approaches by preventing collisions
// when multiple pools start simultaneously
func generatePoolID() string {
	// Generate 9 random bytes (same as graphile-worker v0.5.0+)
	bytes := make([]byte, 9)
	_, err := cryptoRand.Read(bytes)
	if err != nil {
		// Fallback to time-based if crypto fails (should be extremely rare)
		return fmt.Sprintf("pool_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// Constants for LISTEN connection exponential backoff (aligned with graphile-worker commit 50e237e)
const (
	// Wait at most 60 seconds between connection attempts for LISTEN.
	maxListenDelay = 60 * time.Second
)

// WorkerPool represents a pool of workers with graceful shutdown
type WorkerPool struct {
	workers  []*Worker
	pool     *pgxpool.Pool
	schema   string
	tasks    map[string]TaskHandler
	options  WorkerPoolOptions
	logger   *logger.Logger
	eventBus *events.EventBus // Event bus for emitting pool events (main.ts alignment)

	// Lifecycle management
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	shutdownOnce sync.Once

	// Database notification
	notifyConn     *pgxpool.Conn
	notifyCtx      context.Context
	notifyCancel   context.CancelFunc
	listenAttempts int // Counter for exponential backoff (commit 50e237e alignment)

	// Channels for coordination
	nudgeChannel     chan struct{}
	shutdownComplete chan struct{}

	// Error handling and recovery (sync from graphile-worker commit 79f2160)
	errorChan       chan error     // Channel for critical errors that should trigger shutdown
	criticalError   error          // Stores the first critical error that caused shutdown
	criticalErrorMu sync.RWMutex   // Protects criticalError field from data races
	errorHandlerWG  sync.WaitGroup // Separate WaitGroup for error handler goroutine
}

// RunTaskList creates and starts a worker pool (equivalent to graphile-worker runTaskList)
// API signature updated to match graphile-worker commit 5e455c0: options-first parameter order
func RunTaskList(ctx context.Context, options WorkerPoolOptions, tasks map[string]TaskHandler, pool *pgxpool.Pool) (*WorkerPool, error) {
	// Validate required parameters
	if pool == nil {
		return nil, fmt.Errorf("database pool cannot be nil")
	}
	if tasks == nil {
		return nil, fmt.Errorf("tasks map cannot be nil")
	}

	// Apply default options with environment variable support
	applyDefaultOptions(&options)

	if options.Logger == nil {
		options.Logger = logger.DefaultLogger
	}

	// Get EventBus from shared options processing (main.ts alignment)
	compiled := ProcessSharedOptions(&options, nil)
	eventBus := compiled.Events

	// Create pool context
	poolCtx, poolCancel := context.WithCancel(ctx)

	wp := &WorkerPool{
		pool:             pool,
		schema:           options.Schema,
		tasks:            tasks,
		options:          options,
		logger:           options.Logger,
		eventBus:         eventBus,
		ctx:              poolCtx,
		cancel:           poolCancel,
		nudgeChannel:     make(chan struct{}, options.Concurrency*2),
		shutdownComplete: make(chan struct{}),
		errorChan:        make(chan error, 10), // Buffered channel for critical errors
	}

	// Emit pool:create event (main.ts alignment)
	eventBus.Emit(events.PoolCreate, map[string]interface{}{
		"workerPool": wp,
	})

	// Create workers
	poolID := generatePoolID() // Generate unique pool identifier
	for i := 0; i < options.Concurrency; i++ {
		worker := NewWorker(pool, options.Schema, WithNoPreparedStatements(options.NoPreparedStatements))
		// Use pool ID + worker index for unique worker identification
		// This maintains worker uniqueness while using secure random generation
		worker.workerID = fmt.Sprintf("worker-%s-%d", poolID, i)

		// Pass eventBus to worker for event emission
		worker.eventBus = eventBus

		// Register all tasks
		for taskName, handler := range tasks {
			worker.RegisterTask(taskName, handler)
		}

		wp.workers = append(wp.workers, worker)
	}

	// Start database notification listener
	if err := wp.startNotificationListener(); err != nil {
		return nil, fmt.Errorf("failed to start notification listener: %w", err)
	}

	// Start critical error monitor (sync from graphile-worker commit 79f2160)
	wp.errorHandlerWG.Add(1)
	go wp.startErrorMonitor()

	// Start workers
	for i, worker := range wp.workers {
		wp.wg.Add(1)
		go wp.runWorker(i, worker)
	}

	// Start nudge coordinator
	wp.wg.Add(1)
	go wp.runNudgeCoordinator()

	wp.logger.Info(fmt.Sprintf("Worker pool started with %d workers (tasks: %v)",
		options.Concurrency, getTaskNames(tasks)))

	return wp, nil
}

// startNotificationListener starts listening for database notifications
func (wp *WorkerPool) startNotificationListener() error {
	wp.notifyCtx, wp.notifyCancel = context.WithCancel(wp.ctx)

	// Emit pool:listen:connecting event (main.ts alignment)
	wp.eventBus.Emit(events.PoolListenConnecting, map[string]interface{}{
		"workerPool": wp,
		"attempts":   wp.listenAttempts, // Add attempts parameter (commit 50e237e alignment)
	})

	conn, err := wp.pool.Acquire(wp.notifyCtx)
	if err != nil {
		// Apply exponential backoff for initial connection failures (commit 50e237e alignment)
		wp.reconnectWithExponentialBackoff(err)
		return nil // Return nil since reconnection is handled asynchronously
	}
	wp.notifyConn = conn

	// Start listening for job insertions
	_, err = conn.Exec(wp.notifyCtx, `LISTEN "jobs:insert"`)
	if err != nil {
		// Apply exponential backoff for LISTEN failures (commit 50e237e alignment)
		conn.Release()
		wp.reconnectWithExponentialBackoff(err)
		return nil // Return nil since reconnection is handled asynchronously
	}

	// Successful listen; reset attempts counter (commit 50e237e alignment)
	wp.listenAttempts = 0

	// Emit pool:listen:success event (main.ts alignment)
	wp.eventBus.Emit(events.PoolListenSuccess, map[string]interface{}{
		"workerPool": wp,
	})

	// Start notification handler
	wp.wg.Add(1)
	go wp.handleNotifications()

	return nil
}

// startErrorMonitor starts the critical error monitoring goroutine
// This implements the error aggregation pattern from graphile-worker commit 79f2160
func (wp *WorkerPool) startErrorMonitor() {
	defer wp.errorHandlerWG.Done()

	wp.logger.Debug("Error monitor started")

	for {
		select {
		case <-wp.ctx.Done():
			wp.logger.Debug("Error monitor stopping due to context cancellation")
			return
		case err := <-wp.errorChan:
			wp.criticalErrorMu.Lock()
			if wp.criticalError == nil {
				wp.criticalError = err
			}
			wp.criticalErrorMu.Unlock()

			wp.logger.Error(fmt.Sprintf("Critical error detected, initiating graceful shutdown: %v", err))

			// Emit error event before shutdown (main.ts alignment)
			wp.eventBus.Emit(events.PoolError, map[string]interface{}{
				"workerPool": wp,
				"error":      err,
			})

			// Trigger graceful shutdown due to critical error
			// This mirrors the error handling from graphile-worker commit 79f2160
			go func() {
				if shutdownErr := wp.GracefulShutdown(fmt.Sprintf("Critical error: %v", err)); shutdownErr != nil {
					wp.logger.Error(fmt.Sprintf("Error during critical error shutdown: %v", shutdownErr))
				}
			}()
			return
		}
	}
}

// handleNotifications processes database notifications
// Enhanced error handling aligned with graphile-worker commit e714bd0
func (wp *WorkerPool) handleNotifications() {
	defer wp.wg.Done()

	// Panic recovery for notification handler
	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("notification handler panic: %v", r)
			wp.logger.Error(fmt.Sprintf("Notification handler panic recovered: %v", panicErr))

			select {
			case wp.errorChan <- panicErr:
			default:
				wp.logger.Error("Error channel full, could not report notification handler panic")
			}
		}
	}()

	var errorHandled bool

	// Enhanced cleanup function
	cleanup := func() {
		if wp.notifyConn != nil {
			// Attempt to unlisten before releasing (ignore errors during cleanup)
			if !errorHandled {
				_, _ = wp.notifyConn.Exec(context.Background(), `UNLISTEN "jobs:insert"`)
			}
			wp.notifyConn.Release()
			wp.notifyConn = nil
		}
	}

	defer cleanup()

	for {
		select {
		case <-wp.notifyCtx.Done():
			return
		default:
			// Wait for notification with timeout
			notification, err := wp.notifyConn.Conn().WaitForNotification(wp.notifyCtx)
			if err != nil {
				if wp.notifyCtx.Err() != nil {
					return // Context cancelled
				}

				// Mark error as handled to prevent duplicate cleanup
				errorHandled = true

				// Release current connection before retry
				cleanup()

				// Apply exponential backoff with jitter (commit 50e237e alignment)
				wp.reconnectWithExponentialBackoff(err)
				return
			}

			if notification.Channel == "jobs:insert" {
				wp.logger.Debug("Received job insert notification, nudging workers")
				// Nudge workers when new jobs arrive
				select {
				case wp.nudgeChannel <- struct{}{}:
				default:
					// Channel full, workers are already being nudged
				}
			}
		}
	}
}

// reconnectWithExponentialBackoff implements exponential backoff for LISTEN connection retries
// Aligned with graphile-worker commit 50e237e
func (wp *WorkerPool) reconnectWithExponentialBackoff(err error) {
	// Emit pool:listen:error event (main.ts alignment)
	wp.eventBus.Emit(events.PoolListenError, map[string]interface{}{
		"workerPool": wp,
		"error":      err,
	})

	wp.listenAttempts++

	// When figuring the next delay we want exponential back-off, but we also
	// want to avoid the thundering herd problem. For now, we'll add some
	// randomness to it via the `jitter` variable, this variable is
	// deliberately weighted towards the higher end of the duration.
	jitter := wp.generateSecureJitter()

	// Backoff (ms): 136, 370, 1005, 2730, 7421, 20172, 54832
	delayFloat := jitter * math.Min(float64(maxListenDelay/time.Millisecond), 50*math.Exp(float64(wp.listenAttempts)))
	delay := time.Duration(delayFloat) * time.Millisecond

	wp.logger.Error(fmt.Sprintf("Error with notify listener (trying again in %v): %s", delay, err.Error()))

	// Schedule reconnection attempt
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			if wp.notifyCtx.Err() == nil {
				// Emit pool:listen:connecting event (main.ts alignment)
				wp.eventBus.Emit(events.PoolListenConnecting, map[string]interface{}{
					"workerPool": wp,
					"attempts":   wp.listenAttempts,
				})

				if err := wp.startNotificationListener(); err != nil {
					// If reconnection fails, apply backoff again
					wp.reconnectWithExponentialBackoff(err)
				}
			}
		case <-wp.notifyCtx.Done():
			// Context cancelled, stop reconnection attempts
			return
		}
	}()
}

// runNudgeCoordinator coordinates worker nudging with panic recovery
func (wp *WorkerPool) runNudgeCoordinator() {
	defer wp.wg.Done()

	// Panic recovery for nudge coordinator
	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("nudge coordinator panic: %v", r)
			wp.logger.Error(fmt.Sprintf("Nudge coordinator panic recovered: %v", panicErr))

			select {
			case wp.errorChan <- panicErr:
			default:
				wp.logger.Error("Error channel full, could not report nudge coordinator panic")
			}
		}
	}()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case <-wp.nudgeChannel:
			// Try to nudge an idle worker
			wp.nudgeIdleWorker()
		}
	}
}

// nudgeIdleWorker attempts to wake up an idle worker
func (wp *WorkerPool) nudgeIdleWorker() {
	for _, worker := range wp.workers {
		if worker.Nudge() {
			wp.logger.Debug(fmt.Sprintf("Nudged worker %s", worker.workerID))
			return
		}
	}
	wp.logger.Debug("No idle workers to nudge")
}

// runWorker runs a single worker in the pool with panic recovery
// Enhanced with panic recovery aligned with graphile-worker commit 79f2160
func (wp *WorkerPool) runWorker(workerIndex int, worker *Worker) {
	defer wp.wg.Done()

	// Panic recovery mechanism to prevent worker crashes from bringing down the pool
	defer func() {
		if r := recover(); r != nil {
			// Convert panic to error and send to error channel
			panicErr := fmt.Errorf("worker panic (worker %d): %v", workerIndex, r)

			// Log the panic with stack trace for debugging
			wp.logger.Error(fmt.Sprintf("Worker panic recovered: %v", panicErr))

			// Send to error channel for centralized handling
			select {
			case wp.errorChan <- panicErr:
			default:
				// Error channel is full, log this as well
				wp.logger.Error("Error channel full, could not report worker panic")
			}
		}
	}()

	workerLogger := wp.logger.Scope(logger.LogScope{
		WorkerID: worker.workerID,
	})

	workerLogger.Info("Worker starting")

	// Run worker with enhanced polling that respects nudges
	err := wp.runWorkerWithNudging(worker)
	if err != nil && err != context.Canceled {
		workerLogger.Error(fmt.Sprintf("Worker stopped with error: %v", err))

		// Check if this is a critical error that should trigger shutdown
		if wp.isCriticalError(err) {
			select {
			case wp.errorChan <- fmt.Errorf("critical worker error (worker %d): %w", workerIndex, err):
			default:
				// Error channel full
				wp.logger.Error("Error channel full, could not report critical worker error")
			}
		}
	} else {
		workerLogger.Info("Worker stopped gracefully")
	}
}

// runWorkerWithNudging runs worker with nudge-aware polling
func (wp *WorkerPool) runWorkerWithNudging(worker *Worker) error {
	ticker := time.NewTicker(wp.options.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wp.ctx.Done():
			return wp.ctx.Err()
		case <-ticker.C:
			// Regular polling
			if err := wp.processAvailableJobs(worker); err != nil {
				return err
			}
		}
	}
}

// processAvailableJobs processes all available jobs for a worker
func (wp *WorkerPool) processAvailableJobs(worker *Worker) error {
	for {
		select {
		case <-wp.ctx.Done():
			return wp.ctx.Err()
		default:
			job, err := worker.GetJob(wp.ctx)
			if err != nil {
				return fmt.Errorf("failed to get job: %w", err)
			}

			if job == nil {
				// No more jobs available
				return nil
			}

			// Process the job
			if err := worker.ProcessJob(wp.ctx, job); err != nil {
				wp.logger.Error(fmt.Sprintf("Job processing failed: %v", err))
				// Continue with next job even if one fails
			}
		}
	}
}

// isCriticalError determines if an error should trigger pool shutdown
// This implements error classification from graphile-worker commit 79f2160
func (wp *WorkerPool) isCriticalError(err error) bool {
	if err == nil {
		return false
	}

	// Database connection errors are critical
	if err.Error() != "" &&
		(fmt.Sprintf("%v", err) == "connection refused" ||
			fmt.Sprintf("%v", err) == "connection closed" ||
			fmt.Sprintf("%v", err) == "server closed the connection unexpectedly") {
		return true
	}

	// Context cancellation is not critical (it's expected during shutdown)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// For now, be conservative and don't treat other errors as critical
	// This can be expanded based on operational experience
	return false
}

// Release gracefully shuts down the worker pool
func (wp *WorkerPool) Release() error {
	// Emit pool:release event (main.ts alignment)
	wp.eventBus.Emit(events.PoolRelease, map[string]interface{}{
		"pool": wp,
	})
	return wp.GracefulShutdown("Worker pool release requested")
}

// GracefulShutdown performs graceful shutdown of the worker pool
func (wp *WorkerPool) GracefulShutdown(message string) error {
	var shutdownErr error

	wp.shutdownOnce.Do(func() {
		// Emit pool:gracefulShutdown event (main.ts alignment)
		wp.eventBus.Emit(events.PoolGracefulShutdown, map[string]interface{}{
			"pool":    wp,
			"message": message,
		})

		wp.logger.Info(fmt.Sprintf("Starting graceful shutdown: %s", message))

		// Cancel notification listener first
		if wp.notifyCancel != nil {
			wp.notifyCancel()
		}

		// Cancel main context to stop workers
		wp.cancel()

		// Wait for all workers to finish with timeout
		done := make(chan struct{})
		go func() {
			wp.wg.Wait()
			close(done)
		}()

		// Wait for error handler to finish with timeout
		errorHandlerDone := make(chan struct{})
		go func() {
			wp.errorHandlerWG.Wait()
			close(errorHandlerDone)
		}()

		select {
		case <-done:
			wp.logger.Info("All workers stopped gracefully")
		case <-time.After(30 * time.Second):
			shutdownErr = fmt.Errorf("graceful shutdown timeout")
			wp.logger.Error("Graceful shutdown timeout reached")
			// Emit pool:gracefulShutdown:error event (main.ts alignment)
			wp.eventBus.Emit(events.PoolShutdownError, map[string]interface{}{
				"pool":  wp,
				"error": shutdownErr,
			})
		}

		// Wait for error handler to finish (with a shorter timeout)
		select {
		case <-errorHandlerDone:
			wp.logger.Debug("Error handler stopped gracefully")
		case <-time.After(5 * time.Second):
			wp.logger.Warn("Error handler stop timeout")
		}

		close(wp.shutdownComplete)
	})

	// If there was an error during shutdown, emit error event
	if shutdownErr != nil {
		wp.eventBus.Emit(events.PoolShutdownError, map[string]interface{}{
			"pool":  wp,
			"error": shutdownErr,
		})
	}

	return shutdownErr
}

// Wait waits for the worker pool to complete and returns any critical error
// Enhanced to return critical errors aligned with graphile-worker commit 79f2160
func (wp *WorkerPool) Wait() error {
	<-wp.shutdownComplete
	wp.criticalErrorMu.RLock()
	err := wp.criticalError
	wp.criticalErrorMu.RUnlock()
	return err
}

// WaitWithoutError waits for the worker pool to complete without returning error
// This maintains backward compatibility with existing code
func (wp *WorkerPool) WaitWithoutError() {
	<-wp.shutdownComplete
}

// GetActiveJobs returns currently active jobs across all workers
func (wp *WorkerPool) GetActiveJobs() []*Job {
	var activeJobs []*Job
	for _, worker := range wp.workers {
		if job := worker.GetActiveJob(); job != nil {
			activeJobs = append(activeJobs, job)
		}
	}
	return activeJobs
}

// generateSecureJitter creates secure random jitter for exponential backoff
// Uses crypto/rand instead of math/rand for security compliance
func (wp *WorkerPool) generateSecureJitter() float64 {
	// Generate 8 random bytes
	bytes := make([]byte, 8)
	_, err := cryptoRand.Read(bytes)
	if err != nil {
		// Fallback to deterministic jitter if crypto/rand fails
		wp.logger.Warn("Failed to generate secure random jitter, using fallback")
		return 0.75 // Fixed jitter value as fallback
	}

	// Convert bytes to float64 in range [0,1)
	// Take first 8 bytes and treat as uint64
	var randUint64 uint64
	for i := 0; i < 8; i++ {
		randUint64 = (randUint64 << 8) | uint64(bytes[i])
	}

	// Convert to float64 in range [0,1)
	randFloat := float64(randUint64) / float64(^uint64(0))

	// Apply the same formula as before: 0.5 + sqrt(rand)/2
	return 0.5 + math.Sqrt(randFloat)/2
}

// getTaskNames extracts task names from task map
func getTaskNames(tasks map[string]TaskHandler) []string {
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	return names
}
