package integration

import (
	"context"
	"encoding/json"
	"testing"

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
