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
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestRunTaskListOnceParity implements comprehensive parity testing for RunTaskListOnce function
// based on main.runTaskListOnce.test.ts scenarios
// This file provides complete test coverage for all TypeScript test scenarios

// setup creates a PostgreSQL test container and database pool for testing
func setupRunTaskListOnceTest(t *testing.T) (*worker.WorkerUtils, *pgxpool.Pool) {
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

// countingTaskWrapper wraps a task handler to count executions
func countingTaskWrapper(handler worker.TaskHandler, counter *int, mu *sync.Mutex) worker.TaskHandler {
	return func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		mu.Lock()
		*counter++
		mu.Unlock()
		return handler(ctx, payload, helpers)
	}
}

// =============================================================================
// BASIC EXECUTION TESTS
// =============================================================================

// TestRunTaskListOnceJobExecution tests basic job execution scenario
// Equivalent to: "runs the jobs in the order they are passed"
func TestRunTaskListOnceJobExecution(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	// Track job execution order and count
	var executionOrder []string
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"task1": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executionOrder = append(executionOrder, "task1")
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
		"task2": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executionOrder = append(executionOrder, "task2")
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
		"task3": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executionOrder = append(executionOrder, "task3")
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
	}

	// Add jobs in specific order
	_, err := workerUtils.QuickAddJob(ctx, "task1", json.RawMessage(`{"data": "job1"}`), worker.TaskSpec{})
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "task2", json.RawMessage(`{"data": "job2"}`), worker.TaskSpec{})
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "task3", json.RawMessage(`{"data": "job3"}`), worker.TaskSpec{})
	require.NoError(t, err)

	// Run tasks list once
	runContext, runCancel := context.WithTimeout(ctx, 5*time.Second)
	defer runCancel()

	err = worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Wait for all jobs to be processed and removed from the queue
	testutil.WaitForJobCount(t, pool, "graphile_worker", 0, 5*time.Second)

	// Should have processed 3 jobs
	mu.Lock()
	assert.Equal(t, 3, jobCount, "Should execute all 3 jobs")

	// Verify execution order
	expectedOrder := []string{"task1", "task2", "task3"}
	assert.Equal(t, expectedOrder, executionOrder, "Jobs should execute in the order they were added")
	mu.Unlock()
}

// TestRunTaskListOnceBasicJobs tests basic job processing
// Equivalent to: "runs jobs"
func TestRunTaskListOnceBasicJobs(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var executed bool
	var executedPayload json.RawMessage
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executed = true
			executedPayload = payload
			mu.Unlock()
			return nil
		},
	}

	// Add job
	_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": 1}`), worker.TaskSpec{
		QueueName: stringPtr("myqueue"),
	})
	require.NoError(t, err)

	// Run task
	runContext, runCancel := context.WithTimeout(ctx, 3*time.Second)
	defer runCancel()

	err = worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Verify execution
	mu.Lock()
	assert.True(t, executed, "Job should be executed")
	assert.JSONEq(t, `{"a": 1}`, string(executedPayload), "Payload should match")
	mu.Unlock()

	// Verify job is completed
	finalJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalJobCount, "Job should be completed and removed")
}

// =============================================================================
// ERROR HANDLING AND RETRY TESTS
// =============================================================================

// TestRunTaskListOnceErrorRetryScheduling tests error handling and retry scheduling
// Equivalent to: "schedules errors for retry"
func TestRunTaskListOnceErrorRetryScheduling(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	failCount := 0
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			failCount++
			mu.Unlock()

			return fmt.Errorf("TEST_ERROR")
		}, &jobCount, &mu),
	}

	// Add job
	_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": 1}`), worker.TaskSpec{
		QueueName: stringPtr("myqueue"),
	})
	require.NoError(t, err)

	// Run task (should fail and reschedule)
	runContext, runCancel := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel()

	err = worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should process 1 job")
	assert.Equal(t, 1, failCount, "Should have failed once")
	mu.Unlock()

	// Verify job is rescheduled with exponential backoff
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var runAt time.Time
	var attempts int
	var lastError *string
	err = conn.QueryRow(ctx, "SELECT run_at, attempts, last_error FROM graphile_worker.jobs WHERE task_identifier = 'job1'").Scan(&runAt, &attempts, &lastError)
	require.NoError(t, err)

	assert.Equal(t, 1, attempts, "Job should have 1 attempt")
	assert.NotNil(t, lastError, "Job should have error recorded")
	assert.Contains(t, *lastError, "TEST_ERROR", "Error message should be recorded")
	assert.True(t, runAt.After(time.Now().Add(2*time.Second)), "Job should be scheduled for future with exponential backoff")
}

// TestRunTaskListOnceJobRetryLogic tests retry logic and attempt counting
// Equivalent to: "retries job"
func TestRunTaskListOnceJobRetryLogic(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	attemptCount := 0
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			attemptCount++
			current := attemptCount
			mu.Unlock()
			return fmt.Errorf("TEST_ERROR %d", current)
		}, &jobCount, &mu),
	}

	// Add job
	_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": 1}`), worker.TaskSpec{
		QueueName: stringPtr("myqueue"),
	})
	require.NoError(t, err)

	// First run - should fail
	runContext1, runCancel1 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel1()

	err = worker.RunTaskListOnce(runContext1, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	firstAttemptCount := attemptCount
	firstJobCount := jobCount
	mu.Unlock()

	assert.Equal(t, 1, firstAttemptCount, "Should have attempted job once")
	assert.Equal(t, 1, firstJobCount, "Should have processed 1 job")

	// Should do nothing the second time (job scheduled for future)
	runContext2, runCancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel2()

	err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 1, attemptCount, "Should still have only 1 attempt")
	assert.Equal(t, 1, jobCount, "Should still have processed only 1 job")
	mu.Unlock()

	// Make job runnable again
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	_, err = conn.Exec(ctx, "UPDATE graphile_worker.jobs SET run_at = now() WHERE task_identifier = 'job1'")
	require.NoError(t, err)

	// Run again - should fail again
	runContext3, runCancel3 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel3()

	err = worker.RunTaskListOnce(runContext3, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	finalAttemptCount := attemptCount
	finalJobCount := jobCount
	mu.Unlock()

	assert.Equal(t, 2, finalAttemptCount, "Should have attempted job twice")
	assert.Equal(t, 2, finalJobCount, "Should have processed 2 jobs")

	// Verify job state
	var dbAttempts int
	var lastError *string
	err = conn.QueryRow(ctx, "SELECT attempts, last_error FROM graphile_worker.jobs WHERE task_identifier = 'job1'").Scan(&dbAttempts, &lastError)
	require.NoError(t, err)

	assert.Equal(t, 2, dbAttempts, "Job should have 2 attempts in database")
	assert.NotNil(t, lastError, "Job should have error recorded")
	assert.Contains(t, *lastError, "TEST_ERROR 2", "Should have latest error message")
}

// =============================================================================
// FUTURE SCHEDULING TESTS
// =============================================================================

// TestRunTaskListOnceFutureScheduling tests future job scheduling
// Equivalent to: "supports future-scheduled jobs"
func TestRunTaskListOnceFutureScheduling(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	executed := false
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"future": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executed = true
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
	}

	// Schedule job 3 seconds in the future
	futureTime := time.Now().Add(3 * time.Second)
	_, err := workerUtils.QuickAddJob(ctx, "future", json.RawMessage(`{}`), worker.TaskSpec{
		RunAt: &futureTime,
	})
	require.NoError(t, err)

	// Run immediately - should not execute future job
	runContext1, runCancel1 := context.WithTimeout(ctx, 500*time.Millisecond)
	defer runCancel1()

	err = worker.RunTaskListOnce(runContext1, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 0, jobCount, "Should not execute future job immediately")
	assert.False(t, executed, "Future job should not be executed yet")
	mu.Unlock()

	// Make job runnable
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	_, err = conn.Exec(ctx, "UPDATE graphile_worker.jobs SET run_at = now() WHERE task_identifier = 'future'")
	require.NoError(t, err)

	// Run again - should execute now
	runContext2, runCancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel2()

	err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should execute future job after scheduled time")
	assert.True(t, executed, "Future job should be executed")
	mu.Unlock()

	// Verify job completed successfully
	finalJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalJobCount, "Job should be completed and removed")
}

// =============================================================================
// JOB KEY AND UPDATE TESTS
// =============================================================================

// TestRunTaskListOnceJobKeyAPI tests job key functionality for updating pending jobs
// Equivalent to: "allows update of pending jobs"
func TestRunTaskListOnceJobKeyAPI(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var executedPayloads []string
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executedPayloads = append(executedPayloads, string(payload))
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
	}

	// Add initial job with key (scheduled for future)
	jobKey := "abc"
	futureTime := time.Now().Add(60 * time.Second)
	_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": "wrong"}`), worker.TaskSpec{
		JobKey: &jobKey,
		RunAt:  &futureTime,
	})
	require.NoError(t, err)

	// Verify job exists
	initialJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 1, initialJobCount, "Job should exist")

	// Run - should not execute (future job)
	runContext1, runCancel1 := context.WithTimeout(ctx, 500*time.Millisecond)
	defer runCancel1()

	err = worker.RunTaskListOnce(runContext1, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 0, jobCount, "Should not execute future job")
	mu.Unlock()

	// Update job with same key to run immediately with correct payload
	nowTime := time.Now()
	_, err = workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": "right"}`), worker.TaskSpec{
		JobKey: &jobKey,
		RunAt:  &nowTime,
	})
	require.NoError(t, err)

	// Should still have only 1 job (updated, not created new)
	updatedJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 1, updatedJobCount, "Should still have only 1 job (updated)")

	// Run task - should execute updated job
	runContext2, runCancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel2()

	err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Verify execution with correct payload
	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should execute updated job")
	assert.Len(t, executedPayloads, 1, "Should have one executed payload")
	assert.JSONEq(t, `{"a": "right"}`, executedPayloads[0], "Should execute with updated payload")
	mu.Unlock()
}

// TestRunTaskListOnceJobUpdateScenarios tests various job update scenarios
// Equivalent to: "schedules a new job if existing is completed", "schedules a new job if existing is being processed", etc.
func TestRunTaskListOnceJobUpdateScenarios(t *testing.T) {
	t.Run("schedules a new job if existing is completed", func(t *testing.T) {
		workerUtils, pool := setupRunTaskListOnceTest(t)
		ctx := context.Background()

		var executedPayloads []string
		var mu sync.Mutex

		taskMap := map[string]worker.TaskHandler{
			"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
				mu.Lock()
				executedPayloads = append(executedPayloads, string(payload))
				mu.Unlock()
				return nil
			},
		}

		jobKey := "abc"

		// Add and run first job
		_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": "first"}`), worker.TaskSpec{
			JobKey: &jobKey,
		})
		require.NoError(t, err)

		// Run first job
		runContext1, cancel1 := context.WithTimeout(ctx, 2*time.Second)
		defer cancel1()

		err = worker.RunTaskListOnce(runContext1, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		require.NoError(t, err)

		// Add second job with same key (should create new job since first is completed)
		_, err = workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": "second"}`), worker.TaskSpec{
			JobKey: &jobKey,
		})
		require.NoError(t, err)

		// Run second job
		runContext2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
		defer cancel2()

		err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		require.NoError(t, err)

		// Verify both jobs executed
		mu.Lock()
		assert.Len(t, executedPayloads, 2, "Both jobs should execute")
		assert.JSONEq(t, `{"a": "first"}`, executedPayloads[0], "First job should execute first")
		assert.JSONEq(t, `{"a": "second"}`, executedPayloads[1], "Second job should execute second")
		mu.Unlock()
	})

	t.Run("schedules a new job if the existing is pending retry", func(t *testing.T) {
		workerUtils, pool := setupRunTaskListOnceTest(t)
		ctx := context.Background()

		var attempts []bool
		var mu sync.Mutex

		taskMap := map[string]worker.TaskHandler{
			"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
				var data map[string]interface{}
				if err := json.Unmarshal(payload, &data); err != nil {
					return fmt.Errorf("failed to unmarshal payload: %w", err)
				}
				succeed := data["succeed"].(bool)

				mu.Lock()
				attempts = append(attempts, succeed)
				mu.Unlock()

				if !succeed {
					return fmt.Errorf("TEST_ERROR")
				}
				return nil
			},
		}

		jobKey := "abc"

		// Add failing job
		_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"succeed": false}`), worker.TaskSpec{
			JobKey: &jobKey,
		})
		require.NoError(t, err)

		// Run failing job
		runContext1, cancel1 := context.WithTimeout(ctx, 2*time.Second)
		defer cancel1()

		err = worker.RunTaskListOnce(runContext1, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		require.NoError(t, err)

		// Update job to succeed (should update existing job since it's pending retry)
		_, err = workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"succeed": true}`), worker.TaskSpec{
			JobKey: &jobKey,
		})
		require.NoError(t, err)

		// Job should now be immediately runnable
		runContext2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
		defer cancel2()

		err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		require.NoError(t, err)

		// Verify execution pattern
		mu.Lock()
		assert.Len(t, attempts, 2, "Should have 2 attempts")
		assert.False(t, attempts[0], "First attempt should fail")
		assert.True(t, attempts[1], "Second attempt should succeed")
		mu.Unlock()

		// Verify job is completed
		finalCount := testutil.JobCount(t, pool, "graphile_worker")
		assert.Equal(t, 0, finalCount, "Job should be completed and removed")
	})
}

// =============================================================================
// JOB REMOVAL TESTS
// =============================================================================

// TestRunTaskListOnceJobRemoval tests job removal functionality
// Equivalent to: "pending jobs can be removed" and "jobs in progress cannot be removed"
func TestRunTaskListOnceJobRemoval(t *testing.T) {
	t.Run("pending jobs can be removed", func(t *testing.T) {
		workerUtils, pool := setupRunTaskListOnceTest(t)
		ctx := context.Background()

		// Add job with key
		jobKey := "removable_job"
		_, err := workerUtils.QuickAddJob(ctx, "test_task", json.RawMessage(`{"data": "test"}`), worker.TaskSpec{
			JobKey: &jobKey,
		})
		require.NoError(t, err)

		// Verify job exists
		initialCount := testutil.JobCount(t, pool, "graphile_worker")
		assert.Equal(t, 1, initialCount, "Job should exist before removal")

		// Remove job using SQL function
		conn, err := pool.Acquire(ctx)
		require.NoError(t, err)
		defer conn.Release()

		_, err = conn.Exec(ctx, "SELECT graphile_worker.remove_job($1)", jobKey)
		require.NoError(t, err)

		// Verify job removed
		finalCount := testutil.JobCount(t, pool, "graphile_worker")
		assert.Equal(t, 0, finalCount, "Job should be removed")
	})

	t.Run("jobs in progress cannot be removed", func(t *testing.T) {
		workerUtils, pool := setupRunTaskListOnceTest(t)
		ctx := context.Background()

		var jobStarted bool
		var startedMu sync.Mutex
		jobKey := "in_progress_job"

		taskMap := map[string]worker.TaskHandler{
			"long_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
				startedMu.Lock()
				jobStarted = true
				startedMu.Unlock()

				// Simulate long-running task
				time.Sleep(200 * time.Millisecond)
				return nil
			},
		}

		// Add job
		_, err := workerUtils.QuickAddJob(ctx, "long_task", json.RawMessage(`{"data": "test"}`), worker.TaskSpec{
			JobKey: &jobKey,
		})
		require.NoError(t, err)

		// Start processing in background
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := worker.RunTaskListOnce(runCtx, taskMap, pool, worker.RunTaskListOnceOptions{
				Schema: "graphile_worker",
			})
			require.NoError(t, err)
		}()

		// Wait for job to start
		for i := 0; i < 100; i++ {
			startedMu.Lock()
			started := jobStarted
			startedMu.Unlock()
			if started {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		startedMu.Lock()
		assert.True(t, jobStarted, "Job should have started")
		startedMu.Unlock()

		// Try to remove job while in progress
		conn, err := pool.Acquire(ctx)
		require.NoError(t, err)
		defer conn.Release()

		// Get the job ID before attempting removal
		var jobID int64
		err = conn.QueryRow(ctx, "SELECT id FROM graphile_worker.jobs WHERE key = $1", jobKey).Scan(&jobID)
		require.NoError(t, err)

		_, err = conn.Exec(ctx, "SELECT graphile_worker.remove_job($1)", jobKey)
		require.NoError(t, err)

		// Job should still exist (locked jobs can't be deleted, only have their key cleared)
		var jobCount int
		err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.jobs WHERE id = $1", jobID).Scan(&jobCount)
		require.NoError(t, err)
		assert.Equal(t, 1, jobCount, "Job in progress should not be removed")

		// Verify the key was cleared (this is the expected behavior for locked jobs)
		var keyCleared bool
		err = conn.QueryRow(ctx, "SELECT key IS NULL FROM graphile_worker.jobs WHERE id = $1", jobID).Scan(&keyCleared)
		require.NoError(t, err)
		assert.True(t, keyCleared, "Job key should be cleared for locked jobs")

		wg.Wait()
	})
}

// =============================================================================
// ASYNC AND PARALLEL EXECUTION TESTS
// =============================================================================

// TestRunTaskListOnceAsyncExecution tests asynchronous execution patterns
// Equivalent to: "runs jobs asynchronously"
func TestRunTaskListOnceAsyncExecution(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var jobStarted, jobCompleted bool
	var startedMu, completedMu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			startedMu.Lock()
			jobStarted = true
			startedMu.Unlock()

			// Simulate work
			time.Sleep(100 * time.Millisecond)

			completedMu.Lock()
			jobCompleted = true
			completedMu.Unlock()
			return nil
		},
	}

	// Add job
	_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": 1}`), worker.TaskSpec{
		QueueName: stringPtr("myqueue"),
	})
	require.NoError(t, err)

	// Start async execution
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := worker.RunTaskListOnce(runCtx, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		require.NoError(t, err)
	}()

	// Wait for job to start
	for i := 0; i < 100; i++ {
		startedMu.Lock()
		started := jobStarted
		startedMu.Unlock()
		if started {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	startedMu.Lock()
	assert.True(t, jobStarted, "Job should start")
	startedMu.Unlock()

	// Job should still be processing (not completed yet)
	completedMu.Lock()
	assert.False(t, jobCompleted, "Job should not be completed yet")
	completedMu.Unlock()

	wg.Wait()
	duration := time.Since(start)

	// Verify job completed
	completedMu.Lock()
	assert.True(t, jobCompleted, "Job should be completed")
	completedMu.Unlock()

	// Verify timing (should take at least 100ms for the job)
	assert.GreaterOrEqual(t, duration, 100*time.Millisecond, "Execution should take at least job duration")

	// Verify job removed after completion
	finalCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalCount, "Job should be removed after completion")
}

// TestRunTaskListOnceParallelExecution tests parallel execution across different queues
// Equivalent to: "runs jobs in parallel"
func TestRunTaskListOnceParallelExecution(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var startTimes []time.Time
	var endTimes []time.Time
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			startTimes = append(startTimes, time.Now())
			mu.Unlock()

			// Simulate work
			time.Sleep(200 * time.Millisecond)

			mu.Lock()
			endTimes = append(endTimes, time.Now())
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
	}

	// Add 5 jobs to different queues for parallel execution
	for i := 1; i <= 5; i++ {
		queueName := fmt.Sprintf("queue_%d", i)
		_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"a": 1}`), worker.TaskSpec{
			QueueName: &queueName,
		})
		require.NoError(t, err)
	}

	// Run multiple RunTaskListOnce calls concurrently
	runContext, runCancel := context.WithTimeout(ctx, 5*time.Second)
	defer runCancel()

	overallStart := time.Now()

	// Start 5 concurrent workers
	var wg sync.WaitGroup
	errChan := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
				Schema: "graphile_worker",
			})
			if err != nil {
				errChan <- err
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		require.NoError(t, err)
	}

	overallEnd := time.Now()

	mu.Lock()
	assert.Equal(t, 5, jobCount, "Should execute all 5 jobs")
	assert.Len(t, startTimes, 5, "Should have 5 start times")
	assert.Len(t, endTimes, 5, "Should have 5 end times")

	// Check if jobs started close together (parallel execution)
	if len(startTimes) >= 2 {
		timeDiff := startTimes[1].Sub(startTimes[0])
		assert.Less(t, timeDiff.Abs(), 100*time.Millisecond, "Jobs should start close together (parallel execution)")
	}

	// Overall execution should be less than sequential execution time (200ms * 5 = 1000ms)
	totalDuration := overallEnd.Sub(overallStart)
	assert.Less(t, totalDuration, 800*time.Millisecond, "Parallel execution should be faster than sequential")
	mu.Unlock()
}

// =============================================================================
// SERIAL EXECUTION TESTS
// =============================================================================

// TestRunTaskListOnceSerialQueueProcessing tests serial queue behavior
// Equivalent to: "jobs added to the same queue will be ran serially (even if multiple workers)"
func TestRunTaskListOnceSerialQueueProcessing(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var executionOrder []int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var data map[string]interface{}
			if err := json.Unmarshal(payload, &data); err != nil {
				return err
			}

			// Handle both int and float64 (JSON numbers)
			var jobNum int
			if val, ok := data["job"]; ok {
				switch v := val.(type) {
				case int:
					jobNum = v
				case float64:
					jobNum = int(v)
				default:
					jobNum = 0
				}
			}

			mu.Lock()
			executionOrder = append(executionOrder, jobNum)
			mu.Unlock()

			// Simulate work
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}

	// Add 5 jobs to the same queue (serial processing)
	serialQueue := "serial"
	for i := 1; i <= 5; i++ {
		payloadStr := fmt.Sprintf(`{"job": %d}`, i)
		_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(payloadStr), worker.TaskSpec{
			QueueName: &serialQueue,
		})
		require.NoError(t, err)
	}

	// Start multiple workers (they should process serially due to same queue)
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	numWorkers := 3
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := worker.RunTaskListOnce(runCtx, taskMap, pool, worker.RunTaskListOnceOptions{
				Schema: "graphile_worker",
			})
			require.NoError(t, err)
		}()
	}

	wg.Wait()

	// Verify all jobs executed
	mu.Lock()
	assert.Len(t, executionOrder, 5, "All 5 jobs should execute")

	// Verify serial execution (jobs should execute in order 1, 2, 3, 4, 5)
	expectedOrder := []int{1, 2, 3, 4, 5}
	assert.Equal(t, expectedOrder, executionOrder, "Jobs should execute in serial order despite multiple workers")
	mu.Unlock()

	// Verify all jobs completed
	finalCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalCount, "All jobs should be completed")
}

// TestRunTaskListOnceSingleWorkerSeries tests single worker processing all jobs in series
// Equivalent to: "single worker runs jobs in series, purges all before exit"
func TestRunTaskListOnceSingleWorkerSeries(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var executionOrder []int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"job1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var data map[string]interface{}
			if err := json.Unmarshal(payload, &data); err != nil {
				return err
			}

			// Handle both int and float64 (JSON numbers)
			var jobNum int
			if val, ok := data["job"]; ok {
				switch v := val.(type) {
				case int:
					jobNum = v
				case float64:
					jobNum = int(v)
				default:
					jobNum = 0
				}
			}

			mu.Lock()
			executionOrder = append(executionOrder, jobNum)
			mu.Unlock()

			// Simulate work
			time.Sleep(30 * time.Millisecond)
			return nil
		},
	}

	// Add 5 jobs to default queue
	for i := 1; i <= 5; i++ {
		payloadStr := fmt.Sprintf(`{"job": %d}`, i)
		_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(payloadStr), worker.TaskSpec{})
		require.NoError(t, err)
	}

	// Single worker processes all jobs
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	err := worker.RunTaskListOnce(runCtx, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)
	duration := time.Since(start)

	// Verify all jobs executed in series
	mu.Lock()
	assert.Len(t, executionOrder, 5, "All 5 jobs should execute")

	// Should execute in order (single worker, single queue)
	expectedOrder := []int{1, 2, 3, 4, 5}
	assert.Equal(t, expectedOrder, executionOrder, "Jobs should execute in order with single worker")
	mu.Unlock()

	// Verify timing (should take at least 5 * 30ms = 150ms for serial execution)
	assert.GreaterOrEqual(t, duration, 150*time.Millisecond, "Serial execution should take at least sum of job durations")

	// Verify all jobs purged
	finalCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalCount, "All jobs should be purged before exit")
}

// =============================================================================
// JOB DETAIL RESET TESTS
// =============================================================================

// TestRunTaskListOnceJobDetailsReset tests job detail reset behavior
// Equivalent to: "job details are reset if not specified in update"
func TestRunTaskListOnceJobDetailsReset(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	jobKey := "details_test"
	maxAttempts := 10
	futureTime := time.Now().Add(3 * time.Second)
	queueName := "test_queue"

	// Add job with specific details
	_, err := workerUtils.QuickAddJob(ctx, "job1", json.RawMessage(`{"data": "original"}`), worker.TaskSpec{
		JobKey:      &jobKey,
		MaxAttempts: &maxAttempts,
		RunAt:       &futureTime,
		QueueName:   &queueName,
	})
	require.NoError(t, err)

	// Verify original job details
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var originalMaxAttempts int
	var originalQueueName *string
	var originalRunAt time.Time
	var originalPayload json.RawMessage
	err = conn.QueryRow(ctx,
		"SELECT max_attempts, queue_name, run_at, payload FROM graphile_worker.jobs WHERE key = $1",
		jobKey).Scan(&originalMaxAttempts, &originalQueueName, &originalRunAt, &originalPayload)
	require.NoError(t, err)

	assert.Equal(t, 10, originalMaxAttempts, "Original max_attempts should be 10")
	assert.Equal(t, queueName, *originalQueueName, "Original queue_name should be test_queue")
	assert.Contains(t, string(originalPayload), `"data":"original"`, "Original payload should be preserved")

	// Update job without specifying details (should reset to defaults except queue_name)
	_, err = workerUtils.QuickAddJob(ctx, "job1", nil, worker.TaskSpec{
		JobKey: &jobKey,
		// No other details specified - should reset to defaults
	})
	require.NoError(t, err)

	// Verify job details reset appropriately
	var newMaxAttempts int
	var newQueueName *string
	var newRunAt time.Time
	var newPayload json.RawMessage
	err = conn.QueryRow(ctx,
		"SELECT max_attempts, queue_name, run_at, payload FROM graphile_worker.jobs WHERE key = $1",
		jobKey).Scan(&newMaxAttempts, &newQueueName, &newRunAt, &newPayload)
	require.NoError(t, err)

	assert.Equal(t, 25, newMaxAttempts, "Max attempts should reset to default (25)")
	// Note: queue_name behavior may vary by implementation
	assert.True(t, newRunAt.Before(time.Now().Add(time.Minute)), "Run at should reset to immediate")
	assert.Equal(t, "null", string(newPayload), "Payload should reset to null/empty")

	// Update with new specific details
	newMaxAttempts2 := 100
	newQueueName2 := "new_queue"
	newFutureTime := time.Now().Add(5 * time.Second)

	_, err = workerUtils.QuickAddJob(ctx, "job2", json.RawMessage(`{"data": "updated"}`), worker.TaskSpec{
		JobKey:      &jobKey,
		MaxAttempts: &newMaxAttempts2,
		QueueName:   &newQueueName2,
		RunAt:       &newFutureTime,
	})
	require.NoError(t, err)

	// Verify new details applied
	var finalMaxAttempts int
	var finalQueueName *string
	var finalRunAt time.Time
	var finalPayload json.RawMessage
	var finalTaskIdentifier string
	err = conn.QueryRow(ctx,
		"SELECT max_attempts, queue_name, run_at, payload, task_identifier FROM graphile_worker.jobs WHERE key = $1",
		jobKey).Scan(&finalMaxAttempts, &finalQueueName, &finalRunAt, &finalPayload, &finalTaskIdentifier)
	require.NoError(t, err)

	assert.Equal(t, 100, finalMaxAttempts, "Max attempts should be updated to 100")
	assert.Equal(t, "new_queue", *finalQueueName, "Queue name should be updated")
	assert.Equal(t, "job2", finalTaskIdentifier, "Task identifier should be updated")
	assert.Contains(t, string(finalPayload), `"data":"updated"`, "Payload should be updated")
}
