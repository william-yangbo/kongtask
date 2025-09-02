// Package integration contains integration tests for kongtask
// This file tests the admin functions introduced in commit 27dee4d
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// Helper function to create a selection of jobs for testing admin functions
func makeSelectionOfJobs(t *testing.T, utils *worker.WorkerUtils, pool *pgxpool.Pool) (failedJob, regularJob1, lockedJob, regularJob2, untouchedJob worker.Job) {
	ctx := context.Background()
	future := time.Now().Add(60 * time.Minute)

	// Create jobs with different states
	failedJobID, err := utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	regularJob1ID, err := utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 2}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	lockedJobID, err := utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 3}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	regularJob2ID, err := utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 4}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	untouchedJobID, err := utils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 5}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	// Lock one job and fail another
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Lock a job by setting locked_by and locked_at
	_, err = conn.Exec(ctx,
		"UPDATE graphile_worker.jobs SET locked_by = 'test', locked_at = now() WHERE id = $1",
		lockedJobID)
	require.NoError(t, err)

	// Fail a job by setting attempts = max_attempts
	_, err = conn.Exec(ctx,
		"UPDATE graphile_worker.jobs SET attempts = max_attempts, last_error = 'Failed forever' WHERE id = $1",
		failedJobID)
	require.NoError(t, err)

	// Retrieve the jobs to return
	failedJob = *mustGetJobByID(t, pool, failedJobID)
	regularJob1 = *mustGetJobByID(t, pool, regularJob1ID)
	lockedJob = *mustGetJobByID(t, pool, lockedJobID)
	regularJob2 = *mustGetJobByID(t, pool, regularJob2ID)
	untouchedJob = *mustGetJobByID(t, pool, untouchedJobID)

	return
}

// Helper function to get a job by ID
func mustGetJobByID(t *testing.T, pool *pgxpool.Pool, jobID string) *worker.Job {
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var job worker.Job
	var queueName, lastError, key, lockedBy *string
	var lockedAt *time.Time
	var id int

	err = conn.QueryRow(ctx,
		"SELECT id, queue_name, task_identifier, payload, priority, run_at, attempts, max_attempts, last_error, created_at, updated_at, key, revision, locked_at, locked_by FROM graphile_worker.jobs WHERE id = $1",
		jobID).Scan(
		&id, &queueName, &job.TaskIdentifier, &job.Payload,
		&job.Priority, &job.RunAt, &job.AttemptCount, &job.MaxAttempts,
		&lastError, &job.CreatedAt, &job.UpdatedAt,
		&key, &job.Revision, &lockedAt, &lockedBy,
	)
	require.NoError(t, err)

	job.ID = jobID
	job.QueueName = queueName
	job.LastError = lastError
	job.Key = key
	job.LockedAt = lockedAt
	job.LockedBy = lockedBy

	return &job
}

// TestCompleteJobs tests the completeJobs admin function (commit 27dee4d)
// Matches TypeScript test: completes the jobs, leaves others unaffected
func TestCompleteJobs(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	failedJob, regularJob1, lockedJob, regularJob2, untouchedJob := makeSelectionOfJobs(t, utils, pool)

	// Complete some jobs (like TypeScript version)
	jobs := []string{failedJob.ID, regularJob1.ID, lockedJob.ID, regularJob2.ID}
	completedJobs, err := utils.CompleteJobs(ctx, jobs)
	require.NoError(t, err)

	// Should complete failed, regularJob1, and regularJob2 (but not locked job)
	assert.Len(t, completedJobs, 3, "Should complete 3 jobs (excluding locked job)")

	// Extract and sort completed job IDs for verification (matching TypeScript logic)
	completedIDs := make([]string, len(completedJobs))
	for i, job := range completedJobs {
		completedIDs[i] = job.ID
	}

	// Verify expected jobs were completed (matching TypeScript expectations)
	expectedCompletedIDs := []string{failedJob.ID, regularJob1.ID, regularJob2.ID}
	assert.ElementsMatch(t, completedIDs, expectedCompletedIDs, "Should complete exactly these jobs")

	// Verify remaining jobs in database (matching TypeScript verification)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	rows, err := conn.Query(ctx, "SELECT id FROM graphile_worker.jobs ORDER BY id ASC")
	require.NoError(t, err)
	defer rows.Close()

	var remainingIDs []string
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		require.NoError(t, err)
		remainingIDs = append(remainingIDs, id)
	}

	// Should have exactly 2 jobs remaining: locked job and untouched job
	assert.Len(t, remainingIDs, 2, "Should have 2 jobs remaining (locked + untouched)")
	expectedRemainingIDs := []string{lockedJob.ID, untouchedJob.ID}
	assert.ElementsMatch(t, remainingIDs, expectedRemainingIDs, "Should have locked and untouched jobs remaining")
}

// TestPermanentlyFailJobs tests the permanentlyFailJobs admin function (commit 27dee4d)
func TestPermanentlyFailJobs(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	failedJob, regularJob1, lockedJob, regularJob2, untouchedJob := makeSelectionOfJobs(t, utils, pool)

	// Permanently fail some jobs
	jobs := []string{failedJob.ID, regularJob1.ID, lockedJob.ID, regularJob2.ID}
	reason := "TESTING!"
	failedJobs, err := utils.PermanentlyFailJobs(ctx, jobs, reason)
	require.NoError(t, err)

	// Should fail failed, regularJob1, and regularJob2 (but not locked job)
	assert.Len(t, failedJobs, 3, "Should fail 3 jobs (excluding locked job)")

	// Verify specific jobs were failed (exact ID matching like TypeScript test)
	expectedFailedIDs := []string{failedJob.ID, regularJob1.ID, regularJob2.ID}
	actualFailedIDs := make([]string, len(failedJobs))
	for i, job := range failedJobs {
		actualFailedIDs[i] = job.ID
	}
	assert.ElementsMatch(t, expectedFailedIDs, actualFailedIDs, "Should fail exactly the expected jobs")

	for _, job := range failedJobs {
		assert.NotNil(t, job.LastError)
		assert.Equal(t, reason, *job.LastError)
		assert.Equal(t, job.MaxAttempts, job.AttemptCount, "Attempts should equal max_attempts")
		assert.Greater(t, job.AttemptCount, 0, "Attempt count should be greater than 0")
	}

	// Verify all jobs still exist but are failed
	totalCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 5, totalCount, "Should still have all 5 jobs")

	// Verify locked job is unmodified (like TypeScript test)
	lockedJobAfter := mustGetJobByID(t, pool, lockedJob.ID)
	assert.Equal(t, lockedJob.AttemptCount, lockedJobAfter.AttemptCount)
	assert.Equal(t, lockedJob.LastError, lockedJobAfter.LastError)

	// Verify untouched job is unmodified
	untouchedJobAfter := mustGetJobByID(t, pool, untouchedJob.ID)
	assert.Equal(t, untouchedJob.AttemptCount, untouchedJobAfter.AttemptCount)
	assert.Equal(t, untouchedJob.LastError, untouchedJobAfter.LastError)
}

// TestRescheduleJobs tests the rescheduleJobs admin function (commit 27dee4d)
func TestRescheduleJobs(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	failedJob, regularJob1, lockedJob, regularJob2, untouchedJob := makeSelectionOfJobs(t, utils, pool)

	// Reschedule some jobs
	jobs := []string{failedJob.ID, regularJob1.ID, lockedJob.ID, regularJob2.ID}
	nowish := time.Now().Add(5 * time.Minute)
	newPriority := 5
	newAttempts := 1

	rescheduledJobs, err := utils.RescheduleJobs(ctx, jobs, worker.RescheduleOptions{
		RunAt:    &nowish,
		Priority: &newPriority,
		Attempts: &newAttempts,
	})
	require.NoError(t, err)

	// Should reschedule failed, regularJob1, and regularJob2 (but not locked job)
	assert.Len(t, rescheduledJobs, 3, "Should reschedule 3 jobs (excluding locked job)")

	for _, job := range rescheduledJobs {
		assert.Equal(t, newAttempts, job.AttemptCount)
		assert.Equal(t, newPriority, job.Priority)
		assert.WithinDuration(t, nowish, job.RunAt, time.Second, "Run time should be close to specified time")

		// Failed job should retain its error message
		if job.ID == failedJob.ID {
			assert.NotNil(t, job.LastError)
			assert.Equal(t, "Failed forever", *job.LastError)
		} else {
			// Regular jobs should have null error
			assert.Nil(t, job.LastError)
		}
	}

	// Verify all jobs still exist
	totalCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 5, totalCount, "Should still have all 5 jobs")

	// Verify untouched job is unmodified
	untouchedJobAfter := mustGetJobByID(t, pool, untouchedJob.ID)
	assert.Equal(t, untouchedJob.Priority, untouchedJobAfter.Priority)
	assert.Equal(t, untouchedJob.AttemptCount, untouchedJobAfter.AttemptCount)
}

// TestPrioritySupport tests that job priority is properly supported (commit 27dee4d)
func TestPrioritySupport(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Create jobs with different priorities
	// Note: Lower numerical value = higher priority (executed first)
	higherPriority := 1 // Will be executed first
	lowerPriority := 10 // Will be executed second

	higherPriorityJobID, err := utils.QuickAddJob(ctx, "test_job", map[string]interface{}{"priority": "higher"},
		worker.TaskSpec{Priority: &higherPriority})
	require.NoError(t, err)

	lowerPriorityJobID, err := utils.QuickAddJob(ctx, "test_job", map[string]interface{}{"priority": "lower"},
		worker.TaskSpec{Priority: &lowerPriority})
	require.NoError(t, err)

	// Verify priority is stored correctly
	higherPriorityJob := mustGetJobByID(t, pool, higherPriorityJobID)
	lowerPriorityJob := mustGetJobByID(t, pool, lowerPriorityJobID)

	assert.Equal(t, higherPriority, higherPriorityJob.Priority)
	assert.Equal(t, lowerPriority, lowerPriorityJob.Priority)

	// Default priority should be 0
	defaultJobID, err := utils.QuickAddJob(ctx, "test_job", map[string]interface{}{"priority": "default"})
	require.NoError(t, err)

	defaultJob := mustGetJobByID(t, pool, defaultJobID)
	assert.Equal(t, 0, defaultJob.Priority)
}
