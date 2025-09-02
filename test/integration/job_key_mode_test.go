// Package integration contains integration tests for kongtask JobKeyMode functionality
// This file tests the JobKeyMode feature introduced in graphile-worker commit e7ab91e
package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestJobKeyModeIntegration tests the JobKeyMode functionality with real database operations
// This test validates the graphile-worker commit e7ab91e implementation
func TestJobKeyModeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Use existing testutil to setup database (like other integration tests)
	_, pool := testutil.StartPostgres(t)

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Create WorkerUtils instance
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Test data
	taskIdentifier := "job_key_mode_test"
	jobKey := "integration_test_key_" + time.Now().Format("20060102150405")

	t.Run("JobKeyMode Replace (Debouncing)", func(t *testing.T) {
		payload1 := map[string]interface{}{"step": 1, "data": "first"}
		payload2 := map[string]interface{}{"step": 2, "data": "second"}

		runAt1 := time.Now().Add(1 * time.Hour)
		runAt2 := time.Now().Add(2 * time.Hour)

		// Add first job
		jobID1, err := utils.QuickAddJob(ctx, taskIdentifier, payload1, worker.TaskSpec{
			JobKey:     &jobKey,
			JobKeyMode: stringPtr(worker.JobKeyModeReplace),
			RunAt:      &runAt1,
		})
		require.NoError(t, err)

		// Verify first job exists
		job1 := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job1, "First job should exist")
		assert.Equal(t, 0, job1.Revision)
		assert.Contains(t, string(job1.Payload), `"step":1`)

		// Add second job with replace mode - should update the first
		jobID2, err := utils.QuickAddJob(ctx, taskIdentifier, payload2, worker.TaskSpec{
			JobKey:     &jobKey,
			JobKeyMode: stringPtr(worker.JobKeyModeReplace),
			RunAt:      &runAt2,
		})
		require.NoError(t, err)

		// Verify job was replaced (same ID returned)
		assert.Equal(t, jobID1, jobID2, "Should return same job ID for replaced job")

		// Verify job was updated
		job2 := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job2, "Updated job should exist")
		assert.Equal(t, job1.ID, job2.ID, "Should be same job ID")
		assert.Equal(t, 1, job2.Revision, "Revision should be incremented")
		assert.Contains(t, string(job2.Payload), `"step":2`)

		// Verify run_at was updated
		assert.WithinDuration(t, runAt2, job2.RunAt, time.Second)

		// Clean up
		clearJobByKey(t, pool, jobKey)
	})

	t.Run("JobKeyMode PreserveRunAt (Throttling)", func(t *testing.T) {
		payload1 := map[string]interface{}{"step": 1, "data": "first"}
		payload2 := map[string]interface{}{"step": 2, "data": "updated"}

		runAt1 := time.Now().Add(1 * time.Hour)
		runAt2 := time.Now().Add(3 * time.Hour)

		// Add first job (default replace mode)
		_, err := utils.QuickAddJob(ctx, taskIdentifier, payload1, worker.TaskSpec{
			JobKey: &jobKey,
			RunAt:  &runAt1,
		})
		require.NoError(t, err)

		// Get original job
		job1 := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job1)
		originalRunAt := job1.RunAt

		// Add second job with preserve_run_at mode
		_, err = utils.QuickAddJob(ctx, taskIdentifier, payload2, worker.TaskSpec{
			JobKey:     &jobKey,
			JobKeyMode: stringPtr(worker.JobKeyModePreserveRunAt),
			RunAt:      &runAt2,
		})
		require.NoError(t, err)

		// Verify job was updated but run_at preserved
		job2 := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job2)
		assert.Equal(t, job1.ID, job2.ID)
		assert.Equal(t, 1, job2.Revision)
		assert.Contains(t, string(job2.Payload), `"step":2`)

		// Most important: run_at should be preserved
		assert.WithinDuration(t, originalRunAt, job2.RunAt, time.Second)

		// Clean up
		clearJobByKey(t, pool, jobKey)
	})

	t.Run("JobKeyMode UnsafeDedupe (Dangerous)", func(t *testing.T) {
		payload1 := map[string]interface{}{"step": 1, "data": "original"}
		payload2 := map[string]interface{}{"step": 2, "data": "ignored"}

		runAt1 := time.Now().Add(1 * time.Hour)
		runAt2 := time.Now().Add(4 * time.Hour)

		// Add first job
		_, err := utils.QuickAddJob(ctx, taskIdentifier, payload1, worker.TaskSpec{
			JobKey: &jobKey,
			RunAt:  &runAt1,
		})
		require.NoError(t, err)

		// Get original job
		job1 := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job1)

		// Add second job with unsafe_dedupe - should only bump revision
		_, err = utils.QuickAddJob(ctx, taskIdentifier, payload2, worker.TaskSpec{
			JobKey:     &jobKey,
			JobKeyMode: stringPtr(worker.JobKeyModeUnsafeDedupe),
			RunAt:      &runAt2,
		})
		require.NoError(t, err)

		// Verify only revision changed
		job2 := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job2)
		assert.Equal(t, job1.ID, job2.ID)
		assert.Equal(t, 1, job2.Revision, "Revision should be bumped")

		// Payload and run_at should be unchanged
		assert.Equal(t, string(job1.Payload), string(job2.Payload))
		assert.WithinDuration(t, job1.RunAt, job2.RunAt, time.Millisecond)

		// Clean up
		clearJobByKey(t, pool, jobKey)
	})
}

// TestJobKeyModeConvenienceFunctions tests the convenience helper functions
func TestJobKeyModeConvenienceFunctions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	_, pool := testutil.StartPostgres(t)

	// Reset and migrate database
	testutil.Reset(t, pool, "graphile_worker")
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	taskIdentifier := "convenience_test"
	payload := map[string]interface{}{"data": "test"}

	t.Run("AddJobWithReplace", func(t *testing.T) {
		jobKey := "replace_" + time.Now().Format("20060102150405")

		err := utils.AddJobWithReplace(ctx, taskIdentifier, payload, jobKey)
		require.NoError(t, err)

		job := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job)
		assert.Contains(t, string(job.Payload), `"data":"test"`)

		clearJobByKey(t, pool, jobKey)
	})

	t.Run("AddJobWithPreserveRunAt", func(t *testing.T) {
		jobKey := "preserve_" + time.Now().Format("20060102150405")

		err := utils.AddJobWithPreserveRunAt(ctx, taskIdentifier, payload, jobKey)
		require.NoError(t, err)

		job := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job)
		assert.Contains(t, string(job.Payload), `"data":"test"`)

		clearJobByKey(t, pool, jobKey)
	})

	t.Run("AddJobWithUnsafeDedupe", func(t *testing.T) {
		jobKey := "dedupe_" + time.Now().Format("20060102150405")

		err := utils.AddJobWithUnsafeDedupe(ctx, taskIdentifier, payload, jobKey)
		require.NoError(t, err)

		job := getJobByKey(t, pool, jobKey)
		require.NotNil(t, job)
		assert.Contains(t, string(job.Payload), `"data":"test"`)

		clearJobByKey(t, pool, jobKey)
	})
}

// Helper types and functions

type Job struct {
	ID       int64           `db:"id"`
	Revision int             `db:"revision"`
	Payload  json.RawMessage `db:"payload"`
	RunAt    time.Time       `db:"run_at"`
	Key      *string         `db:"key"`
}

func getJobByKey(t *testing.T, pool *pgxpool.Pool, jobKey string) *Job {
	ctx := context.Background()

	var job Job
	query := `SELECT id, revision, payload, run_at, key FROM graphile_worker.jobs WHERE key = $1`

	err := pool.QueryRow(ctx, query, jobKey).Scan(
		&job.ID,
		&job.Revision,
		&job.Payload,
		&job.RunAt,
		&job.Key,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		t.Fatalf("Failed to get job by key: %v", err)
	}

	return &job
}

func clearJobByKey(t *testing.T, pool *pgxpool.Pool, jobKey string) {
	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM graphile_worker.jobs WHERE key = $1", jobKey)
	if err != nil {
		t.Fatalf("Failed to clear job by key: %v", err)
	}
}
