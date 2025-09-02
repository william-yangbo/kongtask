package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

const testSchema = "graphile_worker"

func TestForbiddenFlagsAPI(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()

	testutil.Reset(t, pool, testSchema)

	// Schedule a job with flags
	utils := worker.NewWorkerUtils(pool, testSchema)

	jobID, err := utils.QuickAddJob(context.Background(), "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{
		Flags: []string{"a", "b"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, jobID)

	err = utils.Release()
	require.NoError(t, err)

	// Assert that it has an entry in jobs table with correct flags
	conn, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer conn.Release()

	rows, err := conn.Query(context.Background(), "SELECT * FROM "+testSchema+".jobs")
	require.NoError(t, err)
	defer rows.Close()

	jobCount := 0
	for rows.Next() {
		var job testutil.Job
		var queueName, lastError, key, lockedBy *string
		var flagsRaw json.RawMessage // Use json.RawMessage for raw JSON
		var lockedAt *time.Time
		var id int

		err := rows.Scan(
			&id, &queueName, &job.TaskIdentifier, &job.Payload,
			&job.Priority, &job.RunAt, &job.Attempts, &job.MaxAttempts,
			&lastError, &job.CreatedAt, &job.UpdatedAt,
			&key, &lockedAt, &lockedBy, &job.Revision, &flagsRaw,
		)
		require.NoError(t, err)

		// Check flags - should be {"a": true, "b": true}
		if flagsRaw != nil {
			assert.JSONEq(t, `{"a": true, "b": true}`, string(flagsRaw))
		}
		jobCount++
	}
	assert.Equal(t, 1, jobCount)
}

func TestGetJobSkipsForbiddenFlags(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()

	testutil.Reset(t, pool, testSchema)

	utils := worker.NewWorkerUtils(pool, testSchema)

	// Schedule multiple jobs with different flags
	_, err := utils.QuickAddJob(context.Background(), "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{
		Flags: []string{"a", "b"},
	})
	require.NoError(t, err)

	_, err = utils.QuickAddJob(context.Background(), "job2", map[string]interface{}{"a": 2}, worker.TaskSpec{
		Flags: []string{"b", "c"},
	})
	require.NoError(t, err)

	jobID3, err := utils.QuickAddJob(context.Background(), "job3", map[string]interface{}{"a": 3}, worker.TaskSpec{
		Flags: []string{"d"},
	})
	require.NoError(t, err)

	err = utils.Release()
	require.NoError(t, err)

	// Create a worker with forbidden flags "a" and "b"
	w := worker.NewWorker(pool, testSchema,
		worker.WithForbiddenFlags([]string{"a", "b"}),
	)

	// Should get job3 (only one without forbidden flags)
	job, err := w.GetJob(context.Background())
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "job3", job.TaskIdentifier)
	assert.Equal(t, jobID3, job.ID)

	// Try to get another job - should get nothing since job1 and job2 have forbidden flags
	job2, err := w.GetJob(context.Background())
	require.NoError(t, err)
	assert.Nil(t, job2)

	// Verify that jobs with forbidden flags are still in the database
	conn, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer conn.Release()

	var count int
	err = conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+testSchema+".jobs WHERE attempts = 0").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "Jobs with forbidden flags should remain untouched")
}

func TestGetJobRunsAllJobsWithoutForbiddenFlags(t *testing.T) {
	testCases := []struct {
		name           string
		forbiddenFlags []string
	}{
		{"unknown flag", []string{"z"}},
		{"empty array", []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, pool := testutil.StartPostgres(t)
			defer pool.Close()

			testutil.Reset(t, pool, testSchema)

			utils := worker.NewWorkerUtils(pool, testSchema)

			// Schedule jobs with different flags
			_, err := utils.QuickAddJob(context.Background(), "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{
				Flags: []string{"a", "b"},
			})
			require.NoError(t, err)

			_, err = utils.QuickAddJob(context.Background(), "job2", map[string]interface{}{"a": 2}, worker.TaskSpec{
				Flags: []string{"b", "d"},
			})
			require.NoError(t, err)

			err = utils.Release()
			require.NoError(t, err)

			// Create a worker with forbidden flags that don't match our jobs
			w := worker.NewWorker(pool, testSchema,
				worker.WithForbiddenFlags(tc.forbiddenFlags),
			)

			// Both jobs should be available since forbidden flags don't match
			job1, err := w.GetJob(context.Background())
			require.NoError(t, err)
			assert.NotNil(t, job1, "First job should be available")

			job2, err := w.GetJob(context.Background())
			require.NoError(t, err)
			assert.NotNil(t, job2, "Second job should also be available")

			// No more jobs should be available
			job3, err := w.GetJob(context.Background())
			require.NoError(t, err)
			assert.Nil(t, job3, "No more jobs should be available")
		})
	}
}

func TestForbiddenFlagsFunctionSupport(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	defer pool.Close()

	testutil.Reset(t, pool, testSchema)

	utils := worker.NewWorkerUtils(pool, testSchema)

	// Schedule jobs with different flags
	_, err := utils.QuickAddJob(context.Background(), "job1", map[string]interface{}{"a": 1}, worker.TaskSpec{
		Flags: []string{"a", "b"},
	})
	require.NoError(t, err)

	jobIDForbidden, err := utils.QuickAddJob(context.Background(), "job2", map[string]interface{}{"a": 2}, worker.TaskSpec{
		Flags: []string{"c", "forbidden"},
	})
	require.NoError(t, err)

	err = utils.Release()
	require.NoError(t, err)

	// Create a worker with forbidden flags function that returns "forbidden"
	forbiddenFlagsFn := func() ([]string, error) {
		return []string{"forbidden"}, nil
	}

	w := worker.NewWorker(pool, testSchema,
		worker.WithForbiddenFlagsFn(forbiddenFlagsFn),
	)

	// Should get job1 (without forbidden flag)
	job, err := w.GetJob(context.Background())
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "job1", job.TaskIdentifier)

	// Try to get another job - should get nothing because job2 has forbidden flag
	job2, err := w.GetJob(context.Background())
	require.NoError(t, err)
	assert.Nil(t, job2, "Job with forbidden flag should not be returned")

	// Verify that job with forbidden flag is still in the database
	conn, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer conn.Release()

	var count int
	err = conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+testSchema+".jobs WHERE id = $1 AND attempts = 0", jobIDForbidden).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Job with forbidden flag should remain untouched")
}
