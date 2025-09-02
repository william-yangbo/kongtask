package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// Stop stops the runner gracefully (v0.4.0 alignment)
func (r *Runner) Stop() error {
	r.stopMutex.Lock()
	defer r.stopMutex.Unlock()

	if r.stopped {
		return fmt.Errorf("runner is already stopped")
	}

	r.stopped = true

	// Emit stop event (v0.4.0 alignment)
	r.events.Emit("stop", map[string]interface{}{})

	// Signal stop
	close(r.stopCh)

	// Wait for completion
	select {
	case err := <-r.completeCh:
		return err
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for runner to stop")
	}
}

// AddJob adds a job to the queue (v0.4.0 alignment)
func (r *Runner) AddJob(taskName string, payload interface{}) error {
	r.stopMutex.RLock()
	defer r.stopMutex.RUnlock()

	if r.stopped {
		return fmt.Errorf("runner is stopped")
	}

	return r.worker.AddJob(context.Background(), taskName, payload)
}

// Promise returns a channel that signals when the runner completes (v0.4.0 alignment)
func (r *Runner) Promise() <-chan error {
	return r.completeCh
}

// Events returns the EventBus for listening to runner events (v0.4.0 alignment)
func (r *Runner) Events() *events.EventBus {
	return r.events
}

// IsRunning returns whether the runner is currently running
func (r *Runner) IsRunning() bool {
	r.stopMutex.RLock()
	defer r.stopMutex.RUnlock()
	return !r.stopped
}

// processOptions processes and validates runner options (v0.4.0 alignment)
func processOptions(options RunnerOptions) (*processedOptions, error) {
	// Validate options first
	if err := validateOptions(options); err != nil {
		return nil, err
	}

	// Set defaults
	setDefaults(&options)

	var result *processedOptions

	err := WithReleasers(func(releasers *Releasers) error {
		// Assert and setup database pool
		pool, ownedPool, err := assertPool(options, releasers)
		if err != nil {
			return err
		}

		// Assert and setup task list
		taskList, err := assertTaskList(options, releasers)
		if err != nil {
			return err
		}

		// Run migrations
		migrator := migrate.NewMigrator(pool, options.Schema)
		if err := migrator.Migrate(context.Background()); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		result = &processedOptions{
			taskList:  taskList,
			pool:      pool,
			ownedPool: ownedPool,
			logger:    options.Logger,
			releasers: releasers,
			options:   options,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// processedOptions holds the processed runner configuration
type processedOptions struct {
	taskList  map[string]worker.TaskHandler
	pool      *pgxpool.Pool
	ownedPool bool
	logger    *logger.Logger
	releasers *Releasers
	options   RunnerOptions
}

// Run starts a runner that runs until stopped (v0.4.0 alignment)
func Run(options RunnerOptions) (*Runner, error) {
	processed, err := processOptions(options)
	if err != nil {
		return nil, err
	}

	// Create or use provided EventBus for runner events
	var eventBus *events.EventBus
	if options.Events != nil {
		eventBus = options.Events
	} else {
		eventBus = events.NewEventBus(context.Background(), 100)
	}

	// Create worker
	workerOpts := []worker.WorkerOption{
		worker.WithLogger(processed.logger),
		worker.WithPollInterval(processed.options.PollInterval),
		worker.WithEventBus(eventBus), // Pass EventBus to worker
	}
	if processed.options.WorkerID != "" {
		workerOpts = append(workerOpts, worker.WithWorkerID(processed.options.WorkerID))
	}

	w := worker.NewWorker(processed.pool, processed.options.Schema, workerOpts...)

	// Register all tasks
	for taskName, handler := range processed.taskList {
		w.RegisterTask(taskName, handler)
	}

	// Create runner
	runner := &Runner{
		worker:     w,
		pool:       processed.pool,
		ownedPool:  processed.ownedPool,
		stopped:    false,
		stopCh:     make(chan struct{}),
		completeCh: make(chan error, 1),
		releasers:  make([]ReleaseFunc, 0),
		events:     eventBus, // Set EventBus
	}

	// Add releasers
	if processed.ownedPool {
		runner.releasers = append(runner.releasers, func() error {
			processed.pool.Close()
			return nil
		})
	}

	// Start worker in background
	go func() {
		defer func() {
			// Cleanup resources
			for _, releaser := range runner.releasers {
				if err := releaser(); err != nil {
					processed.logger.Error(fmt.Sprintf("Failed to release resource: %v", err))
				}
			}
		}()

		// Create context with cancellation
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Listen for stop signal
		go func() {
			<-runner.stopCh
			cancel()
		}()

		// Run worker
		err := w.Run(ctx)
		runner.completeCh <- err
	}()

	return runner, nil
}

// RunOnce runs jobs until no more jobs are available, then returns (v0.4.0 alignment)
func RunOnce(options RunnerOptions) error {
	processed, err := processOptions(options)
	if err != nil {
		return err
	}

	defer func() {
		if err := processed.releasers.ReleaseAll(); err != nil {
			processed.logger.Error(fmt.Sprintf("Failed to release resources: %v", err))
		}
	}()

	// Create worker
	workerOpts := []worker.WorkerOption{
		worker.WithLogger(processed.logger),
		worker.WithPollInterval(processed.options.PollInterval),
		worker.WithContinuous(false), // Run once mode
	}
	if processed.options.WorkerID != "" {
		workerOpts = append(workerOpts, worker.WithWorkerID(processed.options.WorkerID))
	}

	w := worker.NewWorker(processed.pool, processed.options.Schema, workerOpts...)

	// Register all tasks
	for taskName, handler := range processed.taskList {
		w.RegisterTask(taskName, handler)
	}

	// Run multiple workers for concurrency
	if processed.options.Concurrency <= 1 {
		return w.RunOnce(context.Background())
	}

	// Run multiple workers concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, processed.options.Concurrency)

	for i := 0; i < processed.options.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.RunOnce(context.Background()); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	// Return first error if any
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// RunMigrations runs database migrations and returns (v0.4.0 alignment)
func RunMigrations(options RunnerOptions) error {
	// We don't need task list for migrations
	migrationOptions := options
	migrationOptions.TaskList = make(map[string]worker.TaskHandler)
	migrationOptions.TaskDirectory = ""

	if err := validateOptions(migrationOptions); err != nil {
		return err
	}

	setDefaults(&migrationOptions)

	return WithReleasers(func(releasers *Releasers) error {
		pool, _, err := assertPool(migrationOptions, releasers)
		if err != nil {
			return err
		}

		migrator := migrate.NewMigrator(pool, migrationOptions.Schema)
		return migrator.Migrate(context.Background())
	})
}
