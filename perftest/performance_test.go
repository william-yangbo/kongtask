package perftest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/internal/worker"
)

// TestBasicPerformance tests basic job processing
func TestBasicPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing basic performance...")

	w := worker.NewWorker(pool, "graphile_worker")

	var processedJobs int64
	w.RegisterTask("performance_test", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		atomic.AddInt64(&processedJobs, 1)
		return nil
	})

	// Add and process 100 jobs
	start := time.Now()
	for i := 0; i < 100; i++ {
		err = w.AddJob(ctx, "performance_test", map[string]interface{}{
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

// TestBulkJobsPerformance tests processing thousands of jobs
func TestBulkJobsPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Load and execute init.sql to create 20,000 test jobs
	sqlBytes, err := os.ReadFile("init.sql")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, string(sqlBytes))
	require.NoError(t, err)

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
} // TestLatencyPerformance measures job processing latency
func TestLatencyPerformance(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	// Initialize database schema
	migrator := migrate.NewMigrator(pool, "")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	t.Log("⚡ Testing latency performance...")

	var totalLatency time.Duration
	var jobCount int64
	var mutex sync.Mutex

	w := worker.NewWorker(pool, "graphile_worker")
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

	// Add 50 test jobs
	for i := 0; i < 50; i++ {
		err = w.AddJob(ctx, "latency_test", map[string]interface{}{
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
