package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// WorkerPoolOptions represents options for worker pool
type WorkerPoolOptions struct {
	Concurrency     int            // Number of concurrent workers
	Schema          string         // Database schema (default: "graphile_worker")
	PollInterval    time.Duration  // Polling interval (default: 1s)
	Logger          *logger.Logger // Logger instance
	NoHandleSignals bool           // If set true, we won't install signal handlers (v0.5.0 feature)
}

// generatePoolID generates a cryptographically secure random pool identifier
// This improves upon timestamp-based approaches by preventing collisions
// when multiple pools start simultaneously
func generatePoolID() string {
	// Generate 9 random bytes (same as graphile-worker v0.5.0+)
	bytes := make([]byte, 9)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to time-based if crypto fails (should be extremely rare)
		return fmt.Sprintf("pool_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// WorkerPool represents a pool of workers with graceful shutdown
type WorkerPool struct {
	workers []*Worker
	pool    *pgxpool.Pool
	schema  string
	tasks   map[string]TaskHandler
	options WorkerPoolOptions
	logger  *logger.Logger

	// Lifecycle management
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	shutdownOnce sync.Once

	// Database notification
	notifyConn   *pgxpool.Conn
	notifyCtx    context.Context
	notifyCancel context.CancelFunc

	// Channels for coordination
	nudgeChannel     chan struct{}
	shutdownComplete chan struct{}
}

// RunTaskList creates and starts a worker pool (equivalent to graphile-worker runTaskList)
func RunTaskList(ctx context.Context, tasks map[string]TaskHandler, pool *pgxpool.Pool, options WorkerPoolOptions) (*WorkerPool, error) {
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

	// Create pool context
	poolCtx, poolCancel := context.WithCancel(ctx)

	wp := &WorkerPool{
		pool:             pool,
		schema:           options.Schema,
		tasks:            tasks,
		options:          options,
		logger:           options.Logger,
		ctx:              poolCtx,
		cancel:           poolCancel,
		nudgeChannel:     make(chan struct{}, options.Concurrency*2),
		shutdownComplete: make(chan struct{}),
	}

	// Create workers
	poolID := generatePoolID() // Generate unique pool identifier
	for i := 0; i < options.Concurrency; i++ {
		worker := NewWorker(pool, options.Schema)
		// Use pool ID + worker index for unique worker identification
		// This maintains worker uniqueness while using secure random generation
		worker.workerID = fmt.Sprintf("worker-%s-%d", poolID, i)

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

	conn, err := wp.pool.Acquire(wp.notifyCtx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection for notifications: %w", err)
	}
	wp.notifyConn = conn

	// Start listening for job insertions
	_, err = conn.Exec(wp.notifyCtx, `LISTEN "jobs:insert"`)
	if err != nil {
		conn.Release()
		return fmt.Errorf("failed to listen for notifications: %w", err)
	}

	// Start notification handler
	wp.wg.Add(1)
	go wp.handleNotifications()

	return nil
}

// handleNotifications processes database notifications
func (wp *WorkerPool) handleNotifications() {
	defer wp.wg.Done()
	defer func() {
		if wp.notifyConn != nil {
			wp.notifyConn.Release()
		}
	}()

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
				wp.logger.Error(fmt.Sprintf("Notification error: %v", err))
				time.Sleep(5 * time.Second) // Retry after error
				continue
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

// runNudgeCoordinator coordinates worker nudging
func (wp *WorkerPool) runNudgeCoordinator() {
	defer wp.wg.Done()

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

// runWorker runs a single worker in the pool
func (wp *WorkerPool) runWorker(workerIndex int, worker *Worker) {
	defer wp.wg.Done()

	workerLogger := wp.logger.Scope(logger.LogScope{
		WorkerID: worker.workerID,
	})

	workerLogger.Info("Worker starting")

	// Run worker with enhanced polling that respects nudges
	err := wp.runWorkerWithNudging(worker)
	if err != nil && err != context.Canceled {
		workerLogger.Error(fmt.Sprintf("Worker stopped with error: %v", err))
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

// Release gracefully shuts down the worker pool
func (wp *WorkerPool) Release() error {
	return wp.GracefulShutdown("Worker pool release requested")
}

// GracefulShutdown performs graceful shutdown of the worker pool
func (wp *WorkerPool) GracefulShutdown(message string) error {
	var shutdownErr error

	wp.shutdownOnce.Do(func() {
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

		select {
		case <-done:
			wp.logger.Info("All workers stopped gracefully")
		case <-time.After(30 * time.Second):
			shutdownErr = fmt.Errorf("graceful shutdown timeout")
			wp.logger.Error("Graceful shutdown timeout reached")
		}

		close(wp.shutdownComplete)
	})

	return shutdownErr
}

// Wait waits for the worker pool to complete
func (wp *WorkerPool) Wait() {
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

// getTaskNames extracts task names from task map
func getTaskNames(tasks map[string]TaskHandler) []string {
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	return names
}
