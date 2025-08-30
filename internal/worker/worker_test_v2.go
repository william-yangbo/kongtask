package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
	assert.Equal(t, float64(42), actualPayload["count"]) // JSON unmarshals numbers as float64
}

func TestWorker_CompleteJob(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	// Add a job
	err = w.AddJob(ctx, "test_task", map[string]string{"test": "data"})
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

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	// Add a job
	err = w.AddJob(ctx, "test_task", map[string]string{"test": "data"})
	require.NoError(t, err)

	// Get the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Fail the job
	failureMessage := "Something went wrong"
	err = w.FailJob(ctx, job.ID, failureMessage)
	require.NoError(t, err)

	// The job should be rescheduled (available again after some time)
	// For this test, we'll just verify the job is no longer immediately available
	nextJob, err := w.GetJob(ctx)
	require.NoError(t, err)
	assert.Nil(t, nextJob) // Job is rescheduled for later
}

func TestWorker_ProcessJob_Success(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	var processedPayload json.RawMessage
	var processedHelpers *Helpers
	w.RegisterTask("test_task", func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error {
		processedPayload = payload
		processedHelpers = helpers
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

	// Verify the handler was called correctly
	assert.NotNil(t, processedPayload)
	assert.NotNil(t, processedHelpers)
	assert.Equal(t, job.Payload, processedPayload)

	// Verify job was completed (no longer available)
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

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	expectedError := fmt.Errorf("task failed")
	w.RegisterTask("failing_task", func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error {
		return expectedError
	})

	// Add and get job
	err = w.AddJob(ctx, "failing_task", map[string]string{"test": "data"})
	require.NoError(t, err)

	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Process the job - should fail
	err = w.ProcessJob(ctx, job)
	require.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Job should be rescheduled (not immediately available)
	nextJob, err := w.GetJob(ctx)
	require.NoError(t, err)
	assert.Nil(t, nextJob)
}

func TestWorker_ProcessJob_UnknownTask(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	// Add job for unknown task
	err = w.AddJob(ctx, "unknown_task", map[string]string{"test": "data"})
	require.NoError(t, err)

	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Process the job - should fail with unknown task error
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

	// Try to get job when none are available
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

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	processedJobs := 0
	w.RegisterTask("test_task", func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error {
		processedJobs++
		return nil
	})

	// Add some jobs
	for i := 0; i < 3; i++ {
		err = w.AddJob(ctx, "test_task", map[string]int{"job": i})
		require.NoError(t, err)
	}

	// Create context with timeout
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Run worker
	err = w.Run(runCtx)
	assert.Equal(t, context.DeadlineExceeded, err)

	// Should have processed all jobs
	assert.Equal(t, 3, processedJobs)
}

func TestWorker_Concurrency(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Add multiple jobs
	numJobs := 10
	for i := 0; i < numJobs; i++ {
		w := NewWorker(pool, "graphile_worker")
		err = w.AddJob(ctx, "concurrent_task", map[string]int{"job": i})
		require.NoError(t, err)
	}

	// Create multiple workers
	numWorkers := 3
	var wg sync.WaitGroup
	processedJobs := make(map[string][]int)
	var mu sync.Mutex

	createWorkerTask := func(workerID string) TaskHandler {
		return func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error {
			var data map[string]int
			json.Unmarshal(payload, &data)
			mu.Lock()
			processedJobs[workerID] = append(processedJobs[workerID], data["job"])
			mu.Unlock()
			time.Sleep(10 * time.Millisecond) // Simulate work
			return nil
		}
	}

	workers := make([]*Worker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		workers[i] = NewWorker(pool, "graphile_worker")
		workers[i].RegisterTask("concurrent_task", createWorkerTask(workerID))
	}

	// Start workers
	workerCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	for i, worker := range workers {
		wg.Add(1)
		go func(w *Worker, id int) {
			defer wg.Done()
			w.Run(workerCtx)
		}(worker, i)
	}

	wg.Wait()

	// Verify all jobs were processed
	totalProcessed := 0
	for _, jobs := range processedJobs {
		totalProcessed += len(jobs)
	}
	assert.Equal(t, numJobs, totalProcessed)
}

func TestWorker_RunOnce(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Setup database schema
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create worker
	w := NewWorker(pool, "graphile_worker")

	processedJobs := 0
	w.RegisterTask("test_task", func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error {
		processedJobs++
		return nil
	})

	// Add some jobs
	for i := 0; i < 3; i++ {
		err = w.AddJob(ctx, "test_task", map[string]int{"job": i})
		require.NoError(t, err)
	}

	// Run worker once
	err = w.RunOnce(ctx)
	require.NoError(t, err)

	// Should have processed all jobs
	assert.Equal(t, 3, processedJobs)

	// No more jobs should be available
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	assert.Nil(t, job)
}
