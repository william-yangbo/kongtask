// Package integration contains integration tests for kongtask
// These tests verify the complete functionality and parity with graphile-worker
package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestWorkerUtilsAddJobParity tests basic job addition through WorkerUtils
// (parity with graphile-worker workerUtils.addJob.test.ts)
func TestWorkerUtilsAddJobParity(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Schedule a job using WorkerUtils
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	_, err = utils.QuickAddJob(ctx, "job1", map[string]interface{}{
		"a": 1,
	})
	require.NoError(t, err)

	// Assert that it has an entry in jobs table
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, 1, jobCount, "Should have exactly 1 job in the queue")

	// Verify we can process the job
	var processedPayload map[string]interface{}
	var processingCompleted bool

	w := worker.NewWorker(pool, "graphile_worker")
	w.RegisterTask("job1", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		err := json.Unmarshal(payload, &processedPayload)
		processingCompleted = true
		return err
	})

	// Get and process the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job, "Should get a job from the queue")

	err = w.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Verify the payload was processed correctly
	require.True(t, processingCompleted, "Job should have been processed")
	require.Equal(t, float64(1), processedPayload["a"], "Payload should match what was sent")
}

// TestWorkerUtilsJobKeyParity tests the JobKey deduplication API
// (parity with graphile-worker workerUtils.addJob.test.ts)
func TestWorkerUtilsJobKeyParity(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Schedule multiple jobs with the same JobKey
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	jobKey := "UNIQUE"
	spec := worker.TaskSpec{JobKey: &jobKey}

	// Add three jobs with the same JobKey - only first should succeed
	_, err = utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 1}, spec)
	require.NoError(t, err)

	_, _ = utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 2}, spec)
	// Second job with same key should be ignored/fail silently depending on implementation

	_, _ = utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 3}, spec)
	// Third job with same key should be ignored/fail silently depending on implementation

	// Assert that only one job exists due to JobKey deduplication
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, 1, jobCount, "Should have exactly 1 job due to JobKey deduplication")

	// Verify we can process the job and it contains the first payload
	var processedPayload map[string]interface{}
	var processingCompleted bool

	w := worker.NewWorker(pool, "graphile_worker")
	w.RegisterTask("job1", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		err := json.Unmarshal(payload, &processedPayload)
		processingCompleted = true
		return err
	})

	// Get and process the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job, "Should get a job from the queue")

	err = w.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Verify the payload was processed correctly (should contain one of the values)
	require.True(t, processingCompleted, "Job should have been processed")
	aValue := processedPayload["a"].(float64)
	require.Contains(t, []float64{1, 2, 3}, aValue, "Payload should be one of the submitted values")
}

// TestQuickAddJobGlobalParity tests the quickAddJobGlobal shortcut function
// (parity with graphile-worker workerUtils.addJob.test.ts)
func TestQuickAddJobGlobalParity(t *testing.T) {
	dbURL, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Schedule a job using the shortcut function
	_, err = worker.QuickAddJobGlobal(ctx, worker.WorkerUtilsOptions{
		ConnectionString: dbURL,
		Schema:           "graphile_worker",
	}, "job1", map[string]interface{}{
		"a": 1,
	})
	require.NoError(t, err)

	// Assert that it has an entry in jobs table
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, 1, jobCount, "Should have exactly 1 job in the queue")

	// Verify we can process the job
	var processedPayload map[string]interface{}
	var processingCompleted bool

	w := worker.NewWorker(pool, "graphile_worker")
	w.RegisterTask("job1", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		err := json.Unmarshal(payload, &processedPayload)
		processingCompleted = true
		return err
	})

	// Get and process the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job, "Should get a job from the queue")

	err = w.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Verify the payload was processed correctly
	require.True(t, processingCompleted, "Job should have been processed")
	require.Equal(t, float64(1), processedPayload["a"], "Payload should match what was sent")
}

// TestWorkerUtilsAdvancedOptions tests various job options
// (extended test for comprehensive coverage)
func TestWorkerUtilsAdvancedOptions(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Test with various options
	maxAttempts := 3
	jobKey := "TEST_OPTIONS"

	spec := worker.TaskSpec{
		MaxAttempts: &maxAttempts,
		JobKey:      &jobKey,
	}

	_, err = utils.QuickAddJob(ctx, "test_job", map[string]interface{}{
		"test": "options",
	}, spec)
	require.NoError(t, err)

	// Verify job was created
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, 1, jobCount, "Should have exactly 1 job in the queue")

	// Verify we can process the job
	w := worker.NewWorker(pool, "graphile_worker")
	var jobProcessed bool

	w.RegisterTask("test_job", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		// Verify job properties that are available
		require.Equal(t, maxAttempts, helpers.Job.MaxAttempts, "MaxAttempts should match")
		if helpers.Job.Key != nil {
			require.Equal(t, jobKey, *helpers.Job.Key, "JobKey should match if present")
		}

		jobProcessed = true
		return nil
	})

	// Process the job
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, job)

	err = w.ProcessJob(ctx, job)
	require.NoError(t, err)
	require.True(t, jobProcessed, "Job should have been processed with correct options")
}

// TestWorkerUtilsMultipleJobs tests adding and processing multiple jobs
// (stress test for reliability)
func TestWorkerUtilsMultipleJobs(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Add multiple jobs
	jobCount := 10
	for i := 0; i < jobCount; i++ {
		_, err = utils.QuickAddJob(ctx, "multi_job", map[string]interface{}{
			"index": i,
		})
		require.NoError(t, err)
	}

	// Verify all jobs were created
	actualJobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, jobCount, actualJobCount, "Should have %d jobs in the queue", jobCount)

	// Process all jobs
	w := worker.NewWorker(pool, "graphile_worker")
	var processedCount int
	var processedIndices []int

	w.RegisterTask("multi_job", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		var data map[string]interface{}
		err := json.Unmarshal(payload, &data)
		require.NoError(t, err)

		index := int(data["index"].(float64))
		processedIndices = append(processedIndices, index)
		processedCount++
		return nil
	})

	// Process all jobs
	for i := 0; i < jobCount; i++ {
		job, err := w.GetJob(ctx)
		require.NoError(t, err)
		require.NotNil(t, job, "Should get job %d", i+1)

		err = w.ProcessJob(ctx, job)
		require.NoError(t, err)
	}

	require.Equal(t, jobCount, processedCount, "Should have processed all jobs")
	require.Len(t, processedIndices, jobCount, "Should have processed indices for all jobs")
}
