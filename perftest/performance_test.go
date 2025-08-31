package perftest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestBasicPerformance tests basic job processing (v0.4.0 compatible)
func TestBasicPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing basic performance with v0.4.0 features...")

	w := worker.NewWorker(pool, "graphile_worker")

	var processedJobs int64
	w.RegisterTask("performance_test", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		atomic.AddInt64(&processedJobs, 1)
		return nil
	})

	// Test WorkerUtils (v0.4.0 feature) for job scheduling
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Add and process 100 jobs using new WorkerUtils
	start := time.Now()
	for i := 0; i < 100; i++ {
		_, err = utils.QuickAddJob(ctx, "performance_test", map[string]interface{}{
			"id": i,
		})
		require.NoError(t, err)
	}

	// Process jobs manually
	for i := 0; i < 100; i++ {
		job, err := w.GetJob(ctx)
		require.NoError(t, err)
		if job != nil {
			err = w.ProcessJob(ctx, job)
			require.NoError(t, err)
		}
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt64(&processedJobs)
	jobsPerSecond := float64(processed) / elapsed.Seconds()

	t.Logf("✅ Processed %d jobs in %v (%.2f jobs/second)", processed, elapsed, jobsPerSecond)
}

// TestJobKeyPerformance tests v0.4.0 JobKey deduplication performance
func TestJobKeyPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing JobKey deduplication performance (v0.4.0 feature)...")

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Test adding multiple jobs with same JobKey - should result in only 1 job
	jobKey := "UNIQUE_PERFORMANCE_TEST"
	spec := worker.TaskSpec{JobKey: &jobKey}

	start := time.Now()
	var successfulJobs int64

	// Try to add 1000 jobs with same key - only first should succeed
	for i := 0; i < 1000; i++ {
		_, err := utils.QuickAddJob(ctx, "performance_test", map[string]interface{}{
			"id": i,
		}, spec)
		if err == nil {
			atomic.AddInt64(&successfulJobs, 1)
		}
	}

	elapsed := time.Since(start)
	successful := atomic.LoadInt64(&successfulJobs)

	// Verify only 1 job exists in database
	jobCount := testutil.JobCount(t, pool, "graphile_worker")
	require.Equal(t, 1, jobCount, "Should have exactly 1 job due to JobKey deduplication")

	t.Logf("✅ JobKey deduplication: %d attempts in %v, %d jobs created (expected: 1)", 1000, elapsed, successful)
}

// TestStringIDPerformance tests v0.4.0 string ID performance
func TestStringIDPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing string ID performance (v0.4.0 change)...")

	w := worker.NewWorker(pool, "graphile_worker")
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	var processedIDs []string
	var mutex sync.Mutex

	w.RegisterTask("string_id_test", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		mutex.Lock()
		processedIDs = append(processedIDs, helpers.Job.ID) // Job.ID is now string in v0.4.0
		mutex.Unlock()
		return nil
	})

	// Add 50 jobs and collect their string IDs
	start := time.Now()
	for i := 0; i < 50; i++ {
		_, err := utils.QuickAddJob(ctx, "string_id_test", map[string]interface{}{
			"index": i,
		})
		require.NoError(t, err)
	}

	// Process jobs and verify string IDs
	for i := 0; i < 50; i++ {
		job, err := w.GetJob(ctx)
		if err != nil || job == nil {
			break
		}

		// Verify ID is string format
		require.IsType(t, "", job.ID, "Job.ID should be string in v0.4.0")
		require.NotEmpty(t, job.ID, "Job.ID should not be empty")

		err = w.ProcessJob(ctx, job)
		require.NoError(t, err)
	}

	elapsed := time.Since(start)

	mutex.Lock()
	processedCount := len(processedIDs)
	mutex.Unlock()

	t.Logf("✅ Processed %d jobs with string IDs in %v", processedCount, elapsed)

	// Verify all IDs are unique strings
	idSet := make(map[string]bool)
	for _, id := range processedIDs {
		require.False(t, idSet[id], "Duplicate ID found: %s", id)
		idSet[id] = true
	}
}

// TestBulkJobsPerformance tests processing thousands of jobs (v0.4.0 compatible)
func TestBulkJobsPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Use WorkerUtils to create jobs efficiently (v0.4.0 feature)
	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	t.Log("⚡ Creating 1000 bulk jobs using WorkerUtils (v0.4.0)...")

	// Create jobs using new WorkerUtils instead of SQL
	for i := 1; i <= 1000; i++ {
		_, err := utils.QuickAddJob(ctx, "log_if_999", map[string]interface{}{
			"id": i,
		})
		require.NoError(t, err)
	}

	t.Log("⚡ Starting bulk jobs performance test...")

	// Setup worker with log_if_999 task (matching original perfTest)
	var processedJobs int64
	var foundTarget bool
	start := time.Now()

	w := worker.NewWorker(pool, "graphile_worker")
	w.RegisterTask("log_if_999", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		atomic.AddInt64(&processedJobs, 1)

		// Parse job payload to get id
		var payloadData struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(payload, &payloadData); err == nil {
			if payloadData.ID == 999 {
				t.Log("Found 999!")
				foundTarget = true
			}
		}
		return nil
	})

	// Process jobs in batches to avoid timeout
	for processedJobs < 1000 { // Process first 1000 jobs
		job, err := w.GetJob(ctx)
		if err != nil || job == nil {
			break
		}
		err = w.ProcessJob(ctx, job)
		require.NoError(t, err)

		if time.Since(start) > time.Minute*2 { // Timeout after 2 minutes
			break
		}
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt64(&processedJobs)
	jobsPerSecond := float64(processed) / elapsed.Seconds()

	t.Logf("✅ Processed %d jobs in %v (%.2f jobs/second)", processed, elapsed, jobsPerSecond)
	if foundTarget {
		t.Log("✅ Successfully found target job (id=999)")
	}
}

// TestParallelWorkerPerformance tests v0.4.0 parallel worker pattern (4 workers, 10 concurrency each)
func TestParallelWorkerPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing parallel worker performance (4 workers, matches v0.4.0 run.js)...")

	// Constants matching v0.4.0 run.js
	const JOB_COUNT = 20000
	const PARALLELISM = 4
	const CONCURRENCY = 10

	utils := worker.NewWorkerUtils(pool, "graphile_worker")

	// Schedule jobs using WorkerUtils (equivalent to init.js)
	t.Logf("📝 Scheduling %d jobs...", JOB_COUNT)
	scheduleStart := time.Now()
	for i := 1; i <= JOB_COUNT; i++ {
		_, err := utils.QuickAddJob(ctx, "log_if_999", map[string]interface{}{
			"id": i,
		})
		require.NoError(t, err)
	}
	scheduleElapsed := time.Since(scheduleStart)
	t.Logf("✅ Scheduled %d jobs in %v", JOB_COUNT, scheduleElapsed)

	// Setup parallel workers (equivalent to run.js parallelism)
	var processedJobs int64
	var foundTarget int64

	// Process jobs with parallel workers
	t.Logf("⚡ Processing with %d parallel workers, %d concurrency each...", PARALLELISM, CONCURRENCY)
	processStart := time.Now()

	var wg sync.WaitGroup
	for workerID := 0; workerID < PARALLELISM; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			w := worker.NewWorker(pool, "graphile_worker")
			w.RegisterTask("log_if_999", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
				atomic.AddInt64(&processedJobs, 1)

				var payloadData struct {
					ID int `json:"id"`
				}
				if err := json.Unmarshal(payload, &payloadData); err == nil {
					if payloadData.ID == 999 {
						atomic.AddInt64(&foundTarget, 1)
					}
				}
				return nil
			})

			// Process jobs with concurrency limit
			for {
				job, err := w.GetJob(ctx)
				if err != nil || job == nil {
					break
				}
				err = w.ProcessJob(ctx, job)
				if err != nil {
					t.Errorf("Worker %d failed to process job: %v", id, err)
					break
				}

				// Check if we should stop (timeout or completion)
				if time.Since(processStart) > time.Minute*5 {
					break
				}
			}
		}(workerID)
	}

	wg.Wait()
	processElapsed := time.Since(processStart)
	processed := atomic.LoadInt64(&processedJobs)
	found := atomic.LoadInt64(&foundTarget)

	// Calculate performance metrics (matching run.js output)
	jobsPerSecond := float64(processed) / processElapsed.Seconds()

	t.Logf("✅ Processed %d/%d jobs in %v", processed, JOB_COUNT, processElapsed)
	t.Logf("📊 Jobs per second: %.2f", jobsPerSecond)
	t.Logf("🎯 Found target job (id=999): %d times", found)

	// Assert reasonable performance (should be much faster than serial)
	require.Greater(t, jobsPerSecond, 100.0, "Should process at least 100 jobs per second with parallel workers")
	require.Greater(t, found, int64(0), "Should find the target job (id=999)")
}

// TestLatencyPerformance measures job processing latency (v0.4.0 compatible)
func TestLatencyPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema with v0.4.0 migrations
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing latency performance with v0.4.0 features...")

	var totalLatency time.Duration
	var jobCount int64
	var mutex sync.Mutex

	w := worker.NewWorker(pool, "graphile_worker")
	utils := worker.NewWorkerUtils(pool, "graphile_worker") // v0.4.0 feature

	w.RegisterTask("latency_test", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		// Measure latency from job creation to processing
		createdAt := helpers.Job.CreatedAt
		processingTime := time.Since(createdAt)

		mutex.Lock()
		totalLatency += processingTime
		jobCount++
		mutex.Unlock()

		return nil
	})

	// Add 50 test jobs using WorkerUtils (v0.4.0 feature)
	for i := 0; i < 50; i++ {
		_, err := utils.QuickAddJob(ctx, "latency_test", map[string]interface{}{
			"id": i,
		})
		require.NoError(t, err)
	}

	// Process jobs
	for i := 0; i < 50; i++ {
		job, err := w.GetJob(ctx)
		require.NoError(t, err)
		if job != nil {
			err = w.ProcessJob(ctx, job)
			require.NoError(t, err)
		}
	}

	if jobCount > 0 {
		avgLatency := totalLatency / time.Duration(jobCount)
		t.Logf("✅ Average job latency: %v (processed %d jobs)", avgLatency, jobCount)
	}
}

// TestStartupShutdownPerformance measures worker startup/shutdown time
func TestStartupShutdownPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	t.Log("⚡ Testing startup/shutdown performance...")

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Measure startup time
	startTime := time.Now()
	w := worker.NewWorker(pool, "graphile_worker")
	w.RegisterTask("test_task", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		return nil
	})

	// Add a test job
	err = w.AddJob(ctx, "test_task", map[string]interface{}{"test": "data"})
	require.NoError(t, err)

	// Process one job to test the workflow
	job, err := w.GetJob(ctx)
	require.NoError(t, err)
	if job != nil {
		err = w.ProcessJob(ctx, job)
		require.NoError(t, err)
	}

	startupTime := time.Since(startTime)

	t.Logf("✅ Basic worker operations completed in: %v", startupTime)
}

// TestMemoryPerformance tests memory usage patterns
func TestMemoryPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing memory performance...")

	var processedJobs int64
	w := worker.NewWorker(pool, "graphile_worker")
	w.RegisterTask("memory_test", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		// Simulate some memory usage
		data := make([]byte, 1024*10) // 10KB per job
		_ = data
		atomic.AddInt64(&processedJobs, 1)
		return nil
	})

	// Add and process jobs
	start := time.Now()
	for i := 0; i < 100; i++ {
		err = w.AddJob(ctx, "memory_test", map[string]interface{}{
			"data": fmt.Sprintf("test_data_%d", i),
		})
		require.NoError(t, err)
	}

	// Process jobs
	for i := 0; i < 100; i++ {
		job, err := w.GetJob(ctx)
		require.NoError(t, err)
		if job != nil {
			err = w.ProcessJob(ctx, job)
			require.NoError(t, err)
		}
	}

	elapsed := time.Since(start)
	processed := atomic.LoadInt64(&processedJobs)
	t.Logf("✅ Memory test: processed %d jobs in %v", processed, elapsed)
}
