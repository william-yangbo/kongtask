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

// TestJobKeyModeSequentialOperations tests the JobKeyMode functionality with sequential operations
// This test mirrors the TypeScript test "supports the jobKey API with jobKeyMode" from workerUtils.addJob.test.ts
func TestJobKeyModeSequentialOperations(t *testing.T) {
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

	// Create WorkerUtils instance
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Test data - using fixed future dates like TypeScript test
	taskIdentifier := "job1"
	jobKey := "UNIQUE"
	runAt1 := time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC)
	runAt2 := time.Date(2201, 1, 1, 0, 0, 0, 0, time.UTC)
	runAt3 := time.Date(2202, 1, 1, 0, 0, 0, 0, time.UTC)
	runAt4 := time.Date(2203, 1, 1, 0, 0, 0, 0, time.UTC)

	// 1. Job first added in replace mode
	t.Log("Step 1: Adding job with replace mode")
	jobID1, err := utils.QuickAddJob(ctx, taskIdentifier, map[string]interface{}{"a": 1}, worker.TaskSpec{
		JobKey:     &jobKey,
		RunAt:      &runAt1,
		JobKeyMode: stringPtr(worker.JobKeyModeReplace),
	})
	require.NoError(t, err)

	job1 := getJobByKey(t, pool, jobKey)
	require.NotNil(t, job1)
	assert.Equal(t, 0, job1.Revision, "Initial job should have revision 0")
	assert.Contains(t, string(job1.Payload), `"a":1`, "Initial job should have payload {a:1}")
	assert.WithinDuration(t, runAt1, job1.RunAt, time.Second, "Initial job should have runAt1")

	// 2. Now updated, but preserve run_at
	t.Log("Step 2: Updating job with preserve_run_at mode")
	jobID2, err := utils.QuickAddJob(ctx, taskIdentifier, map[string]interface{}{"a": 2}, worker.TaskSpec{
		JobKey:     &jobKey,
		RunAt:      &runAt2,
		JobKeyMode: stringPtr(worker.JobKeyModePreserveRunAt),
	})
	require.NoError(t, err)
	assert.Equal(t, jobID1, jobID2, "Should return same job ID")

	job2 := getJobByKey(t, pool, jobKey)
	require.NotNil(t, job2)
	assert.Equal(t, job1.ID, job2.ID, "Should be same job")
	assert.Equal(t, 1, job2.Revision, "Revision should increment to 1")
	assert.Contains(t, string(job2.Payload), `"a":2`, "Payload should be updated to {a:2}")
	assert.WithinDuration(t, runAt1, job2.RunAt, time.Second, "run_at should be preserved (still runAt1)")

	// 3. unsafe_dedupe should take no action other than to bump the revision number
	t.Log("Step 3: Calling with unsafe_dedupe mode")
	jobID3, err := utils.QuickAddJob(ctx, taskIdentifier, map[string]interface{}{"a": 3}, worker.TaskSpec{
		JobKey:     &jobKey,
		RunAt:      &runAt3,
		JobKeyMode: stringPtr(worker.JobKeyModeUnsafeDedupe),
	})
	require.NoError(t, err)
	assert.Equal(t, jobID1, jobID3, "Should return same job ID")

	job3 := getJobByKey(t, pool, jobKey)
	require.NotNil(t, job3)
	assert.Equal(t, job1.ID, job3.ID, "Should be same job")
	assert.Equal(t, 2, job3.Revision, "Revision should increment to 2")
	assert.Equal(t, string(job2.Payload), string(job3.Payload), "Payload should remain unchanged (still {a:2})")
	assert.WithinDuration(t, runAt1, job3.RunAt, time.Second, "run_at should remain unchanged (still runAt1)")

	// 4. Replace the job one final time
	t.Log("Step 4: Final replace operation")
	jobID4, err := utils.QuickAddJob(ctx, taskIdentifier, map[string]interface{}{"a": 4}, worker.TaskSpec{
		JobKey:     &jobKey,
		RunAt:      &runAt4,
		JobKeyMode: stringPtr(worker.JobKeyModeReplace),
	})
	require.NoError(t, err)
	assert.Equal(t, jobID1, jobID4, "Should return same job ID")

	job4 := getJobByKey(t, pool, jobKey)
	require.NotNil(t, job4)
	assert.Equal(t, job1.ID, job4.ID, "Should be same job")
	assert.Equal(t, 3, job4.Revision, "Revision should increment to 3")
	assert.Contains(t, string(job4.Payload), `"a":4`, "Payload should be updated to {a:4}")
	assert.WithinDuration(t, runAt4, job4.RunAt, time.Second, "run_at should be updated to runAt4")

	// Final verification - should have exactly one job with final state
	t.Log("Final verification: Check database state")
	allJobs := getAllJobs(t, pool)
	assert.Len(t, allJobs, 1, "Should have exactly one job in database")

	finalJob := allJobs[0]
	assert.Equal(t, 3, finalJob.Revision, "Final job should have revision 3")
	assert.Contains(t, string(finalJob.Payload), `"a":4`, "Final job should have payload {a:4}")
	assert.WithinDuration(t, runAt4, finalJob.RunAt, time.Second, "Final job should have runAt4")

	t.Log("✅ Sequential JobKeyMode operations test completed successfully")
}

// Helper function to get all jobs from database
func getAllJobs(t *testing.T, pool *pgxpool.Pool) []Job {
	ctx := context.Background()

	query := `SELECT id, revision, payload, run_at, key FROM graphile_worker.jobs ORDER BY id`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		t.Fatalf("Failed to query jobs: %v", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		err := rows.Scan(
			&job.ID,
			&job.Revision,
			&job.Payload,
			&job.RunAt,
			&job.Key,
		)
		if err != nil {
			t.Fatalf("Failed to scan job: %v", err)
		}
		jobs = append(jobs, job)
	}

	return jobs
}
