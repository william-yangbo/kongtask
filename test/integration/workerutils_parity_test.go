package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestWorkerUtilsAddJob corresponds to workerUtils.addJob.test.ts "runs a job added through the worker utils"
func TestWorkerUtilsAddJob(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()
	ctx := context.Background()

	// Reset database (matching TypeScript reset function)
	testutil.Reset(t, pool, "graphile_worker")

	// Schedule a job (matching TypeScript utils.addJob call)
	workerUtils := worker.NewWorkerUtils(pool, "graphile_worker")

	_, err := workerUtils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{})
	require.NoError(t, err)

	// Assert that it has an entry in jobs (matching TypeScript verification)
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 1, jobCount, "Should have exactly 1 job")
}

// TestWorkerUtilsJobKeyAPI corresponds to workerUtils.addJob.test.ts "supports the jobKey API"
// This test specifically verifies the revision tracking functionality from commit 60da79a
func TestWorkerUtilsJobKeyAPI(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()
	ctx := context.Background()

	// Reset database (matching TypeScript reset function)
	testutil.Reset(t, pool, "graphile_worker")

	// Schedule jobs with same jobKey (matching TypeScript test exactly)
	workerUtils := worker.NewWorkerUtils(pool, "graphile_worker")

	jobKey := "UNIQUE"

	// Add multiple jobs with same key (matching TypeScript sequence)
	_, err := workerUtils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{JobKey: &jobKey})
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 2}, worker.TaskSpec{JobKey: &jobKey})
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 3}, worker.TaskSpec{JobKey: &jobKey})
	require.NoError(t, err)

	_, err = workerUtils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 4}, worker.TaskSpec{JobKey: &jobKey})
	require.NoError(t, err)

	// Assert that it has only one entry in jobs (matching TypeScript verification)
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 1, jobCount, "Should have exactly 1 job (job key should prevent duplicates)")

	// Verify the payload and revision (matching TypeScript assertions)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var payload json.RawMessage
	var revision *int
	err = conn.QueryRow(ctx,
		"SELECT payload, revision FROM graphile_worker.jobs WHERE key = $1",
		jobKey,
	).Scan(&payload, &revision)
	require.NoError(t, err)

	// Verify payload matches latest addition (matching TypeScript: jobs[0].payload.a).toBe(4))
	var payloadMap map[string]interface{}
	err = json.Unmarshal(payload, &payloadMap)
	require.NoError(t, err)
	assert.Equal(t, float64(4), payloadMap["a"], "Payload should reflect latest update (a: 4)")

	// Verify revision count (matching TypeScript: jobs[0].revision).toBe(3))
	require.NotNil(t, revision, "Revision should not be nil")
	assert.Equal(t, 3, *revision, "Revision should be 3 (indicating 3 updates after initial)")
}

// TestWorkerUtilsQuickAddJob corresponds to workerUtils.addJob.test.ts "runs a job added through the addJob shortcut function"
func TestWorkerUtilsQuickAddJob(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()
	ctx := context.Background()

	// Reset database (matching TypeScript reset function)
	testutil.Reset(t, pool, "graphile_worker")

	// Schedule a job using QuickAddJob (matching TypeScript quickAddJob call)
	workerUtils := worker.NewWorkerUtils(pool, "graphile_worker")

	_, err := workerUtils.QuickAddJob(ctx, "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{})
	require.NoError(t, err)

	// Assert that it has an entry in jobs (matching TypeScript verification)
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 1, jobCount, "Should have exactly 1 job")
}

// TestWorkerUtilsAddJobRespectsUseNodeTime corresponds to workerUtils.addJob.test.ts "adding job respects useNodeTime"
// This test now includes full time mocking integration with the TimeProvider
func TestWorkerUtilsAddJobRespectsUseNodeTime(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()
	ctx := context.Background()

	// Reset database
	testutil.Reset(t, pool, "graphile_worker")

	// Set up fake timers (matching TypeScript: await setTime(REFERENCE_TIMESTAMP))
	timer := testutil.GetGlobalFakeTimer()
	referenceTime := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	timer.SetTime(referenceTime)

	// Advance fake time by 1 hour (matching TypeScript: const timeOfAddJob = REFERENCE_TIMESTAMP + 1 * HOUR)
	timeOfAddJob := referenceTime.Add(time.Hour)
	timer.SetTime(timeOfAddJob)

	// Create time provider for testing
	timeProvider := NewTestTimeProvider(timer)

	// Test with useNodeTime=true and time provider
	useNodeTimeTrue := true
	schema := "graphile_worker"
	options := &worker.SharedOptions{
		UseNodeTime:  &useNodeTimeTrue,
		Schema:       &schema,
		TimeProvider: timeProvider,
	}

	addJob := worker.MakeAddJobWithOptions(options, func(ctx context.Context, callback func(tx pgx.Tx) error) error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return err
		}
		defer conn.Release()

		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		if err := callback(tx); err != nil {
			return err
		}

		return tx.Commit(ctx)
	})

	// Add job without explicit runAt (this should trigger useNodeTime logic with mocked time)
	err := addJob(ctx, "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{})
	require.NoError(t, err)

	// Assert that job was created (matching TypeScript verification)
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	assert.Equal(t, 1, jobCount, "Should have exactly 1 job")

	// Verify run_at is within a couple of seconds of timeOfAddJob (matching TypeScript assertions)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var runAt time.Time
	err = conn.QueryRow(ctx, "SELECT run_at FROM graphile_worker.jobs WHERE task_identifier = $1", "job1").Scan(&runAt)
	require.NoError(t, err)

	// Assert the run_at is within a couple of seconds of timeOfAddJob (matching TypeScript logic)
	// even though PostgreSQL has a NOW() that's many months later
	timeDiff := runAt.Sub(timeOfAddJob)
	assert.True(t, timeDiff >= -2*time.Second, "run_at should be within 2 seconds of mock time (got %v)", timeDiff)
	assert.True(t, timeDiff <= 2*time.Second, "run_at should be within 2 seconds of mock time (got %v)", timeDiff)

	t.Logf("Mock time: %v", timeOfAddJob)
	t.Logf("Job run_at: %v", runAt)
	t.Logf("Time difference: %v", timeDiff)
	t.Logf("✅ Full time mocking integration working: useNodeTime respects TimeProvider!")
}
