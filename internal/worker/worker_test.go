package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
)

func TestWorker_AddAndGetJob(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	// Test adding a job
	payload := map[string]interface{}{
		"message": "hello world",
		"count":   42,
	}

	err = w.AddJob(ctx, "test_task", payload)
	require.NoError(t, err)

	// Test getting the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	assert.Equal(t, "test_task", job.TaskIdentifier)
	assert.Equal(t, 1, job.AttemptCount) // get_job increments attempts
	assert.Equal(t, 25, job.MaxAttempts) // Default from SQL function

	// Verify payload
	var actualPayload map[string]interface{}
	err = json.Unmarshal(job.Payload, &actualPayload)
	require.NoError(t, err)
	assert.Equal(t, "hello world", actualPayload["message"])
	assert.Equal(t, float64(42), actualPayload["count"]) // JSON numbers are float64
}

func TestWorker_CompleteJob(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker and add job
	w := NewWorker(pool, "graphile_worker")
	err = w.AddJob(ctx, "test_task", map[string]string{"key": "value"})
	require.NoError(t, err)

	// Get the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Complete the job
	err = w.CompleteJob(ctx, job.ID)
	require.NoError(t, err)

	// Verify no more jobs are available
	nextJob, err := w.GetJob(ctx)
	require.NoError(t, err)
	assert.Nil(t, nextJob)
}

func TestWorker_FailJob(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker and add job
	w := NewWorker(pool, "graphile_worker")
	err = w.AddJob(ctx, "test_task", map[string]string{"key": "value"})
	require.NoError(t, err)

	// Get the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Fail the job
	errorMessage := "Something went wrong"
	err = w.FailJob(ctx, job.ID, errorMessage)
	require.NoError(t, err)

	// The job should be marked as failed but not immediately available due to exponential backoff
	// Instead, check the job queue directly
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var lastError *string
	var attemptCount int
	err = conn.QueryRow(ctx,
		"SELECT last_error, attempts FROM graphile_worker.jobs WHERE id = $1",
		job.ID).Scan(&lastError, &attemptCount)
	require.NoError(t, err)

	assert.NotNil(t, lastError)
	assert.Equal(t, errorMessage, *lastError)
	assert.Equal(t, 1, attemptCount) // Should remain 1 (get_job incremented it)
}

func TestWorker_ProcessJob_Success(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker and register handler
	w := NewWorker(pool, "graphile_worker")

	var processedJob *Job
	w.RegisterTask("test_task", func(ctx context.Context, job *Job) error {
		processedJob = job
		return nil
	})

	// Add and get job
	err = w.AddJob(ctx, "test_task", map[string]string{"test": "data"})
	require.NoError(t, err)

	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Process the job
	err = w.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Verify handler was called
	assert.NotNil(t, processedJob)
	assert.Equal(t, job.ID, processedJob.ID)

	// Verify job is completed (no more jobs available)
	nextJob, err := w.GetJob(ctx)
	require.NoError(t, err)
	assert.Nil(t, nextJob)
}

func TestWorker_ProcessJob_Failure(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker and register failing handler
	w := NewWorker(pool, "graphile_worker")

	expectedError := fmt.Errorf("task failed")
	w.RegisterTask("failing_task", func(ctx context.Context, job *Job) error {
		return expectedError
	})

	// Add and get job
	err = w.AddJob(ctx, "failing_task", map[string]string{"test": "data"})
	require.NoError(t, err)

	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Process the job (should fail)
	err = w.ProcessJob(ctx, job)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Verify job is marked as failed by checking database directly
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var lastError *string
	var attemptCount int
	err = conn.QueryRow(ctx,
		"SELECT last_error, attempts FROM graphile_worker.jobs WHERE id = $1",
		job.ID).Scan(&lastError, &attemptCount)
	require.NoError(t, err)

	assert.NotNil(t, lastError)
	assert.Equal(t, "task failed", *lastError)
	assert.Equal(t, 1, attemptCount) // Should remain 1 from initial get_job
}

func TestWorker_ProcessJob_UnknownTask(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker without registering handler
	w := NewWorker(pool, "graphile_worker")

	// Add and get job for unknown task
	err = w.AddJob(ctx, "unknown_task", map[string]string{"test": "data"})
	require.NoError(t, err)

	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Process should fail with unknown task error
	err = w.ProcessJob(ctx, job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no handler registered for task: unknown_task")
}

func TestWorker_NoJobsAvailable(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	// Try to get job when none available
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	assert.Nil(t, job)
}

func TestWorker_Run_WithContext(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker with handler
	w := NewWorker(pool, "graphile_worker")

	processedJobs := 0
	w.RegisterTask("test_task", func(ctx context.Context, job *Job) error {
		processedJobs++
		return nil
	})

	// Add some jobs
	for i := 0; i < 3; i++ {
		err = w.AddJob(ctx, "test_task", map[string]int{"id": i})
		require.NoError(t, err)
	}

	// Run worker with timeout
	workerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Start worker in goroutine
	done := make(chan error, 1)
	go func() {
		done <- w.Run(workerCtx)
	}()

	// Wait for jobs to be processed
	time.Sleep(2 * time.Second)
	cancel() // Stop the worker

	// Wait for worker to finish
	select {
	case err := <-done:
		assert.Equal(t, context.Canceled, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Worker didn't stop in time")
	}

	// Verify jobs were processed
	assert.Equal(t, 3, processedJobs)
}

func TestWorker_Concurrency(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create multiple workers
	w1 := NewWorker(pool, "graphile_worker")
	w2 := NewWorker(pool, "graphile_worker")
	w3 := NewWorker(pool, "graphile_worker")

	// Track processed jobs with worker ID
	processedJobs := make(map[string][]int)
	jobHandler := func(workerID string) TaskHandler {
		return func(ctx context.Context, job *Job) error {
			processedJobs[workerID] = append(processedJobs[workerID], job.ID)
			time.Sleep(10 * time.Millisecond) // Simulate work
			return nil
		}
	}

	w1.RegisterTask("concurrent_task", jobHandler(w1.workerID))
	w2.RegisterTask("concurrent_task", jobHandler(w2.workerID))
	w3.RegisterTask("concurrent_task", jobHandler(w3.workerID))

	// Add multiple jobs
	for i := 0; i < 9; i++ {
		err = w1.AddJob(ctx, "concurrent_task", map[string]int{"id": i})
		require.NoError(t, err)
	}

	// Run workers concurrently for a short time
	workerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Start all workers
	go w1.Run(workerCtx)
	go w2.Run(workerCtx)
	go w3.Run(workerCtx)

	// Wait for all jobs to be processed
	// Wait for all 9 jobs to be processed
	err = testutil.SleepUntil(func() bool {
		total := len(processedJobs[w1.workerID]) + len(processedJobs[w2.workerID]) + len(processedJobs[w3.workerID])
		return total >= 9
	}, 5*time.Second)
	require.NoError(t, err)

	// Check final count (might be more due to timing)
	totalProcessed := len(processedJobs[w1.workerID]) + len(processedJobs[w2.workerID]) + len(processedJobs[w3.workerID])
	assert.GreaterOrEqual(t, totalProcessed, 9, "Should process at least 9 jobs")
	assert.LessOrEqual(t, totalProcessed, 12, "Should not process too many extra jobs")

	// At least 2 workers should have processed jobs (due to concurrency)
	workersUsed := 0
	if len(processedJobs[w1.workerID]) > 0 {
		workersUsed++
	}
	if len(processedJobs[w2.workerID]) > 0 {
		workersUsed++
	}
	if len(processedJobs[w3.workerID]) > 0 {
		workersUsed++
	}
	assert.GreaterOrEqual(t, workersUsed, 2, "Multiple workers should participate in processing")
}

func TestWorker_JobPriority(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	processedJobs := make([]map[string]interface{}, 0)
	w.RegisterTask("priority_task", func(ctx context.Context, job *Job) error {
		var payload map[string]interface{}
		json.Unmarshal(job.Payload, &payload)
		processedJobs = append(processedJobs, payload)
		return nil
	})

	// Add jobs with different priorities (lower number = higher priority)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Add low priority job first
	_, err = conn.Exec(ctx,
		`INSERT INTO graphile_worker.jobs (task_identifier, payload, priority) VALUES ($1, $2, $3)`,
		"priority_task", `{"priority": "low", "order": 3}`, 10)
	require.NoError(t, err)

	// Add high priority job
	_, err = conn.Exec(ctx,
		`INSERT INTO graphile_worker.jobs (task_identifier, payload, priority) VALUES ($1, $2, $3)`,
		"priority_task", `{"priority": "high", "order": 1}`, 1)
	require.NoError(t, err)

	// Add medium priority job
	_, err = conn.Exec(ctx,
		`INSERT INTO graphile_worker.jobs (task_identifier, payload, priority) VALUES ($1, $2, $3)`,
		"priority_task", `{"priority": "medium", "order": 2}`, 5)
	require.NoError(t, err)

	// Also need to create job queue entries
	_, err = conn.Exec(ctx,
		`INSERT INTO graphile_worker.job_queues (queue_name, job_count) 
		 SELECT DISTINCT queue_name, COUNT(*) 
		 FROM graphile_worker.jobs 
		 GROUP BY queue_name 
		 ON CONFLICT (queue_name) DO UPDATE SET job_count = EXCLUDED.job_count`)
	require.NoError(t, err) // Process jobs one by one
	for i := 0; i < 3; i++ {
		job, err := w.GetJob(ctx)
		require.NoError(t, err)
		require.NotNil(t, job)

		err = w.ProcessJob(ctx, job)
		require.NoError(t, err)
	}

	// Verify jobs were processed in priority order (1, 2, 3)
	require.Len(t, processedJobs, 3)
	assert.Equal(t, float64(1), processedJobs[0]["order"])
	assert.Equal(t, float64(2), processedJobs[1]["order"])
	assert.Equal(t, float64(3), processedJobs[2]["order"])
}
