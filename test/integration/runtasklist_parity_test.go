package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/logger"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestRunTaskListParity implements parity testing for RunTaskList function
// based on main.runTaskList.test.ts scenarios

// Deferred structure simulates TypeScript deferred functionality for job control
type Deferred struct {
	ch       chan struct{}
	once     sync.Once
	resolved bool
	mu       sync.Mutex
}

// NewDeferred creates a new deferred promise-like structure
func NewDeferred() *Deferred {
	return &Deferred{
		ch: make(chan struct{}),
	}
}

// Resolve resolves the deferred (like resolving a promise)
func (d *Deferred) Resolve() {
	d.once.Do(func() {
		d.mu.Lock()
		d.resolved = true
		d.mu.Unlock()
		close(d.ch)
	})
}

// Wait waits for the deferred to be resolved
func (d *Deferred) Wait() {
	<-d.ch
}

// IsResolved checks if the deferred has been resolved
func (d *Deferred) IsResolved() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.resolved
}

// setupRunTaskListTest creates a PostgreSQL test container and database pool for testing
func setupRunTaskListTest(t *testing.T) (*worker.WorkerUtils, *pgxpool.Pool) {
	dbURL, pool := testutil.StartPostgres(t)
	t.Logf("Database URL: %s", dbURL)

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	ctx := context.Background()
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create WorkerUtils for job management
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	return workerUtils, pool
}

// addJob helper function (equivalent to the TypeScript addJob function)
func addJob(t *testing.T, utils *worker.WorkerUtils, id string) {
	ctx := context.Background()
	payload := map[string]string{"id": id}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	_, err = utils.QuickAddJob(ctx, "job1", json.RawMessage(payloadBytes), worker.TaskSpec{
		QueueName: stringPtr("serial"), // Use serial queue like in TypeScript test
	})
	require.NoError(t, err)
}

// helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// TestRunTaskListJobExecutionAndCleanExit tests main job execution and clean exit
// Equivalent to: "main will execute jobs as they come up, and exits cleanly"
func TestRunTaskListJobExecutionAndCleanExit(t *testing.T) {
	workerUtils, pool := setupRunTaskListTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track job execution
	jobPromises := make(map[string]*Deferred)
	var jobPromisesMu sync.Mutex
	var callCount int
	var callCountMu sync.Mutex

	// Define task handlers (equivalent to graphile-worker TaskList)
	tasks := map[string]worker.TaskHandler{
		"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			// Parse the job ID from payload
			var payloadData struct {
				ID string `json:"id"`
			}
			err := json.Unmarshal(payload, &payloadData)
			if err != nil {
				return fmt.Errorf("failed to parse payload: %w", err)
			}

			// Track call count (equivalent to jest.fn() call tracking)
			callCountMu.Lock()
			callCount++
			callCountMu.Unlock()

			// Create deferred promise for this job
			jobPromisesMu.Lock()
			if _, exists := jobPromises[payloadData.ID]; exists {
				jobPromisesMu.Unlock()
				return fmt.Errorf("Job with this id already registered")
			}
			deferred := NewDeferred()
			jobPromises[payloadData.ID] = deferred
			jobPromisesMu.Unlock()

			// Wait for the promise to be resolved (simulating async work)
			deferred.Wait()
			return nil
		},
	}

	// Create worker pool options
	options := worker.WorkerPoolOptions{
		Concurrency:  3, // Same as TypeScript test
		Schema:       "graphile_worker",
		PollInterval: 100 * time.Millisecond, // Fast polling for test
		Logger:       nil,                    // Use default logger
	}

	// Start the worker pool
	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Track if worker pool has finished
	var finished bool
	var finishedMu sync.Mutex
	go func() {
		workerPool.Wait()
		finishedMu.Lock()
		finished = true
		finishedMu.Unlock()
	}()

	// Add jobs dynamically and resolve them (equivalent to TypeScript test logic)
	for i := 0; i < 5; i++ {
		// Verify expected number of jobs are registered
		jobPromisesMu.Lock()
		currentJobCount := len(jobPromises)
		jobPromisesMu.Unlock()
		assert.Equal(t, i, currentJobCount, "Should have %d jobs registered at iteration %d", i, i)

		// Add a new job
		addJob(t, workerUtils, fmt.Sprintf("%d", i))

		// Wait for the job to be picked up by a worker (sleepUntil equivalent)
		require.Eventually(t, func() bool {
			jobPromisesMu.Lock()
			defer jobPromisesMu.Unlock()
			_, exists := jobPromises[fmt.Sprintf("%d", i)]
			return exists
		}, 2*time.Second, 10*time.Millisecond, "Job %d should be picked up by worker", i)

		// Verify job count increased
		jobPromisesMu.Lock()
		currentJobCount = len(jobPromises)
		jobPromisesMu.Unlock()
		assert.Equal(t, i+1, currentJobCount, "Should have %d jobs registered after adding job %d", i+1, i)

		// Resolve this job so the next can start
		jobPromisesMu.Lock()
		if deferred, exists := jobPromises[fmt.Sprintf("%d", i)]; exists {
			jobPromisesMu.Unlock()
			deferred.Resolve()
		} else {
			jobPromisesMu.Unlock()
			t.Errorf("Job %d not found in jobPromises", i)
		}
	}

	// Wait a bit and ensure worker pool is still running
	time.Sleep(100 * time.Millisecond)
	finishedMu.Lock()
	currentFinished := finished
	finishedMu.Unlock()
	assert.False(t, currentFinished, "Worker pool should still be running")

	// Release the worker pool (equivalent to workerPool.release())
	err = workerPool.Release()
	require.NoError(t, err)

	// Verify the task was called the expected number of times
	callCountMu.Lock()
	finalCallCount := callCount
	callCountMu.Unlock()
	assert.Equal(t, 5, finalCallCount, "job1 should have been called 5 times")

	// Wait a bit and ensure worker pool has finished
	time.Sleep(100 * time.Millisecond)
	finishedMu.Lock()
	finalFinished := finished
	finishedMu.Unlock()
	assert.True(t, finalFinished, "Worker pool should have finished after shutdown")

	// Verify all jobs were processed and removed from queue
	finalJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalJobCount, "All jobs should be processed and removed from queue")
}

// TestRunTaskListDebugCompatibility tests debug function compatibility
// Equivalent to: "doesn't bail on deprecated `debug` function"
func TestRunTaskListDebugCompatibility(t *testing.T) {
	workerUtils, pool := setupRunTaskListTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track job execution
	var jobStarted bool
	var jobStartedMu sync.Mutex
	var jobDeferred *Deferred

	// Define task handlers with debug usage
	tasks := map[string]worker.TaskHandler{
		"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			// Test helpers.Logger functionality (equivalent to deprecated helpers.debug)
			helpers.Logger.Debug(fmt.Sprintf("Hey %s", string(payload)))

			// Signal that job has started
			jobStartedMu.Lock()
			jobStarted = true
			jobDeferred = NewDeferred()
			jobStartedMu.Unlock()

			// Wait for test to finish
			jobDeferred.Wait()
			return nil
		},
	}

	// Create worker pool options
	options := worker.WorkerPoolOptions{
		Concurrency:  3,
		Schema:       "graphile_worker",
		PollInterval: 100 * time.Millisecond,
		Logger:       nil,
	}

	// Start the worker pool
	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Add a job
	addJob(t, workerUtils, "test")

	// Wait for job to start (sleepUntil equivalent)
	require.Eventually(t, func() bool {
		jobStartedMu.Lock()
		defer jobStartedMu.Unlock()
		return jobStarted
	}, 2*time.Second, 10*time.Millisecond, "Job should start processing")

	// Resolve the job
	jobStartedMu.Lock()
	if jobDeferred != nil {
		jobDeferred.Resolve()
	}
	jobStartedMu.Unlock()

	// Shutdown the worker pool
	err = workerPool.Release()
	require.NoError(t, err)
}

// TestRunTaskListWithSignalHandling tests the signal handling version
// Note: This test focuses on basic functionality rather than actual signal testing
func TestRunTaskListWithSignalHandling(t *testing.T) {
	// Enable test mode to prevent actual signal handling
	worker.SetTestMode(true)
	defer worker.SetTestMode(false)

	workerUtils, pool := setupRunTaskListTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Track job execution
	var jobProcessed bool
	var jobProcessedMu sync.Mutex

	// Define task handlers
	tasks := map[string]worker.TaskHandler{
		"signal_test": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			jobProcessedMu.Lock()
			jobProcessed = true
			jobProcessedMu.Unlock()
			return nil
		},
	}

	// Create worker pool options with a logger
	options := worker.WorkerPoolOptions{
		Concurrency:  2,
		Schema:       "graphile_worker",
		PollInterval: 100 * time.Millisecond,
		Logger:       logger.DefaultLogger,
	}

	// Start worker pool with signal handling
	managedPool, err := worker.RunTaskListWithSignalHandling(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Add a job
	payload := map[string]string{"test": "signal_handling"}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "signal_test", json.RawMessage(payloadBytes), worker.TaskSpec{})
	require.NoError(t, err)

	// Wait for job to be processed
	require.Eventually(t, func() bool {
		jobProcessedMu.Lock()
		defer jobProcessedMu.Unlock()
		return jobProcessed
	}, 2*time.Second, 50*time.Millisecond, "Job should be processed")

	// Test graceful shutdown
	err = managedPool.Release()
	require.NoError(t, err)

	// Wait for shutdown to complete
	managedPool.Wait()

	// Verify job queue is empty
	finalJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalJobCount, "All jobs should be processed")
}
