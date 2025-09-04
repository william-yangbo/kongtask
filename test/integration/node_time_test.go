package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestUseNodeTimeWorksForRegularJobs corresponds to nodeTime.test.ts "useNodeTime works for regular jobs"
func TestUseNodeTimeWorksForRegularJobs(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)

	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Set up fake timers
		setTime := testutil.SetupFakeTimers()
		defer testutil.GetGlobalFakeTimer().Reset()

		// Set time to reference timestamp (January 1st, 2021, 00:00:00 UTC)
		setTime(REFERENCE_TIMESTAMP_MS)

		// Create task handler
		job1Called := false
		job1Handler := func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			job1Called = true
			t.Logf("job1 executed with payload: %s", string(payload))
			return nil
		}

		// Schedule a job "in the future" according to Go (but in the past according to Postgres)
		runAt := time.Unix((REFERENCE_TIMESTAMP_MS+3*int64(testutil.HOUR/time.Millisecond))/1000, 0).UTC() // 03:00:00.000

		utils := worker.NewWorkerUtils(pool, TEST_SCHEMA)
		_, err := utils.QuickAddJob(context.Background(), "job1", map[string]interface{}{"a": "wrong"}, worker.TaskSpec{
			RunAt:  &runAt,
			JobKey: &jobKeyAbc,
		})
		require.NoError(t, err)

		// Create worker with useNodeTime = true and fake time provider
		timer := testutil.GetGlobalFakeTimer()
		timeProvider := NewTestTimeProvider(timer)
		w := worker.NewWorker(pool, TEST_SCHEMA,
			worker.WithUseNodeTime(true),
			worker.WithTimeProvider(timeProvider),
		)
		w.RegisterTask("job1", job1Handler)

		// Running the task should not execute the job because it's "in the future" according to Go time
		// Note: Go time thinks we're in Jan 2021, but PostgreSQL knows we're not
		err = w.RunOnce(context.Background())
		require.NoError(t, err)
		require.False(t, job1Called, "Job should not have been called because it's in the future")

		// Now set our timestamp to be just after runAt and try again
		setTime(REFERENCE_TIMESTAMP_MS + 3*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond)) // 03:00:00.001

		// Reset the flag
		job1Called = false

		// This time the job should run (when using useNodeTime with a controlled time source)
		err = w.RunOnce(context.Background())
		require.NoError(t, err)

		// The job should have run this time
		require.True(t, job1Called, "Job should have been called after time advanced")
	})
}

// TestValidateJobWouldRunWithoutUseNodeTime corresponds to nodeTime.test.ts
// "validate the job would have run if not for useNodeTime"
func TestValidateJobWouldRunWithoutUseNodeTime(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)

	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Set up fake timers
		setTime := testutil.SetupFakeTimers()
		defer testutil.GetGlobalFakeTimer().Reset()

		// Set time to reference timestamp
		setTime(REFERENCE_TIMESTAMP_MS)

		// Create task handler
		job1Called := false
		job1Handler := func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			job1Called = true
			t.Logf("job1 executed with payload: %s", string(payload))
			return nil
		}

		// Schedule a job "in the future" according to Go (but in the past according to Postgres)
		runAt := time.Unix((REFERENCE_TIMESTAMP_MS+3*int64(testutil.HOUR/time.Millisecond))/1000, 0).UTC() // 03:00:00.000

		utils := worker.NewWorkerUtils(pool, TEST_SCHEMA)
		_, err := utils.QuickAddJob(context.Background(), "job1", map[string]interface{}{"a": "wrong"}, worker.TaskSpec{
			RunAt:  &runAt,
			JobKey: &jobKeyAbc,
		})
		require.NoError(t, err)

		// Create worker with useNodeTime = false (default behavior)
		w := worker.NewWorker(pool, TEST_SCHEMA, worker.WithUseNodeTime(false))
		w.RegisterTask("job1", job1Handler)

		// Run one task iteration
		err = w.RunOnce(context.Background())
		require.NoError(t, err)

		// The job SHOULD have run because, according to PostgreSQL, it's in the past
		require.True(t, job1Called, "Job should have been called when useNodeTime is false")
	})
}

// TestUseNodeTimeWithTimeAdvancement corresponds to nodeTime.test.ts concept
// "useNodeTime works with time advancement" - testing that jobs scheduled for future run when time advances
func TestUseNodeTimeWithTimeAdvancement(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)

	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Set up fake timers
		timer := testutil.GetGlobalFakeTimer()
		timer.SetTime(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

		// Create task handler
		jobCalled := false
		jobHandler := func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			jobCalled = true
			t.Logf("job executed with payload: %s", string(payload))
			return nil
		}

		// Schedule a job for 2 hours in the future
		runAt := time.Date(2021, 1, 1, 2, 0, 0, 0, time.UTC)

		utils := worker.NewWorkerUtils(pool, TEST_SCHEMA)
		_, err := utils.QuickAddJob(context.Background(), "futureJob", map[string]interface{}{"scheduled": "future"}, worker.TaskSpec{
			RunAt: &runAt,
		})
		require.NoError(t, err)

		// Create worker with useNodeTime = true and fake time provider
		timeProvider := NewTestTimeProvider(timer)
		w := worker.NewWorker(pool, TEST_SCHEMA,
			worker.WithUseNodeTime(true),
			worker.WithTimeProvider(timeProvider),
		)
		w.RegisterTask("futureJob", jobHandler)

		// Run one task iteration - should not execute because it's in the future
		err = w.RunOnce(context.Background())
		require.NoError(t, err)
		require.False(t, jobCalled, "Job should not have been called because it's in the future")

		// Advance time to exactly when the job should run
		timer.SetTime(runAt)

		// Reset flag
		jobCalled = false

		// Run one task iteration - should execute now
		err = w.RunOnce(context.Background())
		require.NoError(t, err)
		require.True(t, jobCalled, "Job should have been called after time advanced to scheduled time")
	})
}

// Helper constants and variables
var jobKeyAbc = "abc"
