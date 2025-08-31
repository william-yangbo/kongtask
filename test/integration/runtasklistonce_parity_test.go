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

// TestRunTaskListOnceParity implements parity testing for RunTaskListOnce function
// based on main.runTaskListOnce.test.ts scenarios

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

// TestRunTaskListOnceErrorRetryScheduling tests error handling and retry scheduling
// Equivalent to: "schedules retry job on error with exponential backoff"
func TestRunTaskListOnceErrorRetryScheduling(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	failCount := 0
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"failing_task": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			failCount++
			currentFail := failCount
			mu.Unlock()

			return fmt.Errorf("simulated failure %d", currentFail)
		}, &jobCount, &mu),
	}

	// Add job with max attempts
	maxAttempts := 5
	_, err := workerUtils.QuickAddJob(ctx, "failing_task", json.RawMessage(`{"data": "retry_test"}`), worker.TaskSpec{
		MaxAttempts: &maxAttempts,
	})
	require.NoError(t, err)

	// Run task list once (first attempt - should fail)
	runContext1, runCancel1 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel1()

	err = worker.RunTaskListOnce(runContext1, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should process 1 job on first run")
	assert.Equal(t, 1, failCount, "Should have failed once")
	mu.Unlock()

	// Run task list once immediately again - should NOT process job because it's scheduled for future
	runContext2, runCancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel2()

	err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should NOT process job again immediately (it's scheduled for future)")
	assert.Equal(t, 1, failCount, "Should still have only failed once")
	mu.Unlock()

	// Verify job is scheduled for the future with exponential backoff (~exp(1) ~= 2.7 seconds)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var runAt time.Time
	var attempts int
	err = conn.QueryRow(ctx, "SELECT run_at, attempts FROM graphile_worker.jobs WHERE task_identifier = 'failing_task'").Scan(&runAt, &attempts)
	require.NoError(t, err)

	// Check that job is scheduled for future with exponential backoff
	assert.Equal(t, 1, attempts, "Job should have 1 attempt")
	expectedMinDelay := time.Now().Add(2 * time.Second) // ~exp(1) = 2.7s, but allow some margin
	expectedMaxDelay := time.Now().Add(5 * time.Second) // generous upper bound
	assert.True(t, runAt.After(expectedMinDelay), "Job should be scheduled for future with exponential backoff")
	assert.True(t, runAt.Before(expectedMaxDelay), "Job should not be scheduled too far in the future")
} // TestRunTaskListOnceJobRetryLogic tests retry logic and attempt counting
// Equivalent to: "respects maxAttempts setting"
func TestRunTaskListOnceJobRetryLogic(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	attemptCount := 0
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"always_fail": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			attemptCount++
			current := attemptCount
			mu.Unlock()
			return fmt.Errorf("always fails - attempt %d", current)
		}, &jobCount, &mu),
	}

	// Add job with limited max attempts
	maxAttempts := 2
	_, err := workerUtils.QuickAddJob(ctx, "always_fail", json.RawMessage(`{"data": "retry_limit_test"}`), worker.TaskSpec{
		MaxAttempts: &maxAttempts,
	})
	require.NoError(t, err)

	// Run task once - should fail and reschedule
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

	// Verify job is still in database with 1 attempt and scheduled for future
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var dbAttempts, maxAttemptsDB int
	var runAt time.Time
	err = conn.QueryRow(ctx, "SELECT attempts, max_attempts, run_at FROM graphile_worker.jobs WHERE task_identifier = 'always_fail'").Scan(&dbAttempts, &maxAttemptsDB, &runAt)
	require.NoError(t, err)

	assert.Equal(t, 1, dbAttempts, "Job should have 1 attempt in database")
	assert.Equal(t, 2, maxAttemptsDB, "Job should have max_attempts = 2")
	assert.True(t, runAt.After(time.Now()), "Job should be scheduled for future")

	// Manually set run_at to now so we can test the second attempt
	_, err = conn.Exec(ctx, "UPDATE graphile_worker.jobs SET run_at = now() WHERE task_identifier = 'always_fail'")
	require.NoError(t, err)

	// Run task again - should fail for the second and final time
	runContext2, runCancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel2()

	err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	secondAttemptCount := attemptCount
	secondJobCount := jobCount
	mu.Unlock()

	assert.Equal(t, 2, secondAttemptCount, "Should have attempted job twice total")
	assert.Equal(t, 2, secondJobCount, "Should have processed 2 jobs total")

	// Verify job is still in database but has reached max attempts (no longer eligible for execution)
	var jobCount3, dbAttempts3, maxAttemptsDB3 int
	err = conn.QueryRow(ctx, "SELECT COUNT(*), attempts, max_attempts FROM graphile_worker.jobs WHERE task_identifier = 'always_fail' GROUP BY attempts, max_attempts").Scan(&jobCount3, &dbAttempts3, &maxAttemptsDB3)
	require.NoError(t, err)

	assert.Equal(t, 1, jobCount3, "Job should still exist in database (not deleted)")
	assert.Equal(t, 2, dbAttempts3, "Job should have 2 attempts in database")
	assert.Equal(t, 2, maxAttemptsDB3, "Job should have max_attempts = 2")
	assert.Equal(t, dbAttempts3, maxAttemptsDB3, "Job attempts should equal max_attempts (exhausted)")

	// Running again should find no jobs
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

	assert.Equal(t, 2, finalAttemptCount, "Should still have only 2 attempts total")
	assert.Equal(t, 2, finalJobCount, "Should still have processed only 2 jobs total")
}

// TestRunTaskListOnceFutureScheduling tests future job scheduling
// Equivalent to: "schedules jobs for future execution"
func TestRunTaskListOnceFutureScheduling(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	executed := false
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"future_task": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executed = true
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
	}

	// Schedule job 1 second in the future
	futureTime := time.Now().Add(1 * time.Second)
	_, err := workerUtils.QuickAddJob(ctx, "future_task", json.RawMessage(`{"data": "future_test"}`), worker.TaskSpec{
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

	// Wait for future time and run again
	time.Sleep(1200 * time.Millisecond) // Wait for future time + buffer

	runContext2, runCancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel2()

	err = worker.RunTaskListOnce(runContext2, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should execute future job after scheduled time")
	assert.True(t, executed, "Future job should be executed after scheduled time")
	mu.Unlock()
}

// TestRunTaskListOnceJobKeyAPI tests job key functionality for updating pending jobs
// Equivalent to: "updates pending job when using same job key"
func TestRunTaskListOnceJobKeyAPI(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var executedPayloads []string
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"keyed_task": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			mu.Lock()
			executedPayloads = append(executedPayloads, string(payload))
			mu.Unlock()
			return nil
		}, &jobCount, &mu),
	}

	// Add first job with key
	jobKey := "unique_job_key"
	_, err := workerUtils.QuickAddJob(ctx, "keyed_task", json.RawMessage(`{"version": 1}`), worker.TaskSpec{
		JobKey: &jobKey,
	})
	require.NoError(t, err)

	// Add second job with same key (should update/replace first job)
	_, err = workerUtils.QuickAddJob(ctx, "keyed_task", json.RawMessage(`{"version": 2}`), worker.TaskSpec{
		JobKey: &jobKey,
	})
	require.NoError(t, err)

	// Run task list once
	runContext, runCancel := context.WithTimeout(ctx, 3*time.Second)
	defer runCancel()

	err = worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
		Schema: "graphile_worker",
	})
	require.NoError(t, err)

	// Verify that the job queue is now empty (job was processed)
	finalJobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 0, finalJobCount, "Job queue should be empty after processing")

	// Should only execute one job (the updated one)
	mu.Lock()
	assert.Equal(t, 1, jobCount, "Should only execute one job when using same key")
	assert.Len(t, executedPayloads, 1, "Should only have one executed payload")
	assert.Contains(t, executedPayloads[0], `"version":2`, "Should execute the updated job payload")
	mu.Unlock()
}

// TestRunTaskListOnceParallelExecution tests parallel execution across different queues
// Equivalent to: "runs jobs in parallel" - using multiple concurrent runTaskListOnce calls
func TestRunTaskListOnceParallelExecution(t *testing.T) {
	workerUtils, pool := setupRunTaskListOnceTest(t)
	ctx := context.Background()

	var startTimes []time.Time
	var endTimes []time.Time
	var jobCount int
	var mu sync.Mutex

	taskMap := map[string]worker.TaskHandler{
		"slow_task": countingTaskWrapper(func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
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

	// Add jobs to different queues (this is key for parallel execution)
	queueA := "queue_1"
	queueB := "queue_2"

	_, err := workerUtils.QuickAddJob(ctx, "slow_task", json.RawMessage(`{"queue": "a"}`), worker.TaskSpec{
		QueueName: &queueA,
	})
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "slow_task", json.RawMessage(`{"queue": "b"}`), worker.TaskSpec{
		QueueName: &queueB,
	})
	require.NoError(t, err)

	// Run multiple RunTaskListOnce calls concurrently (this is how parallel execution happens)
	runContext, runCancel := context.WithTimeout(ctx, 5*time.Second)
	defer runCancel()

	overallStart := time.Now()

	// Use errgroup for concurrent execution
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Start two concurrent RunTaskListOnce calls
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		if err != nil {
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		err := worker.RunTaskListOnce(runContext, taskMap, pool, worker.RunTaskListOnceOptions{
			Schema: "graphile_worker",
		})
		if err != nil {
			errChan <- err
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		require.NoError(t, err)
	}

	overallEnd := time.Now()

	mu.Lock()
	assert.Equal(t, 2, jobCount, "Should execute both jobs")
	assert.Len(t, startTimes, 2, "Should have 2 start times")
	assert.Len(t, endTimes, 2, "Should have 2 end times")

	// Check if jobs started close together (parallel execution)
	if len(startTimes) >= 2 {
		timeDiff := startTimes[1].Sub(startTimes[0])
		assert.Less(t, timeDiff.Abs(), 100*time.Millisecond, "Jobs should start close together (parallel execution)")
	}

	// Overall execution should be less than sequential execution time (200ms + 200ms = 400ms)
	totalDuration := overallEnd.Sub(overallStart)
	assert.Less(t, totalDuration, 350*time.Millisecond, "Parallel execution should be faster than sequential")
	mu.Unlock()
}
